//go:build skgo_dev_integration

package devserver

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Kit's development server copies a handler's content-length onto the public
// response. The example app traces server requests, so the fixture does too.
func TestRealSessionEndpointContentLength(t *testing.T) {
	realSession(t, false, false, false, "content-length")
}

const sizedLine = "literal-sized-415\n"

func setupContentLength(t *testing.T, app string, write func(string, string)) {
	t.Helper()
	write("web/vite.config.ts", strings.Replace(sessionConfig("", "_app", "https://initial.test"), "experimental:", "tracing:{server:true},experimental:", 1))
	// Larger than net/http's unflushed buffer, so the length is explicit.
	write("web/src/routes/sized/server.go", fmt.Sprintf(`package sized
import("net/http";"strings";"testing/fstest";"github.com/tylergannon/skgo")
var files=fstest.MapFS{"asset.txt":{Data:[]byte(strings.Repeat(%q,4000))}}
func get(w http.ResponseWriter,r *http.Request){http.ServeFileFS(w,r,files,"asset.txt")}
var _=skgo.GET(get)
`, sizedLine))
	write("web/src/routes/streamed/server.go", fmt.Sprintf(`package streamed
import("io";"net/http";"os";"time";"github.com/tylergannon/skgo")
func get(w http.ResponseWriter,r *http.Request){
 io.WriteString(w,"literal-stream-first\n");http.NewResponseController(w).Flush()
 for deadline:=time.Now().Add(20*time.Second);time.Now().Before(deadline);time.Sleep(10*time.Millisecond){if _,err:=os.Stat(%q);err==nil{break}}
 io.WriteString(w,"literal-stream-second\n")
}
var _=skgo.GET(get)
`, filepath.Join(app, "stream.release")))
}

func exerciseContentLength(t *testing.T, app, origin string, log *sessionLog) {
	t.Helper()
	client := &http.Client{Timeout: 30 * time.Second}
	sized := strings.Repeat(sizedLine, 4000)
	for _, tc := range []struct{ path, body string }{
		// net/http measures a small unflushed body the handler left unsized.
		{"/orders", "literal-4101:8"},
		{"/sized", sized},
	} {
		res, err := client.Get(origin + tc.path)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil || res.StatusCode != 200 || string(body) != tc.body {
			t.Fatalf("%s status=%d len=%d: %v\n%s", tc.path, res.StatusCode, len(body), err, log.String())
		}
		if got := res.Header.Get("Content-Length"); got != strconv.Itoa(len(tc.body)) || len(res.TransferEncoding) != 0 {
			t.Errorf("%s Content-Length=%q Transfer-Encoding=%v, want %d", tc.path, got, res.TransferEncoding, len(tc.body))
		}
		for name := range res.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-skgo-") && !strings.EqualFold(name, "X-Skgo-Mode") {
				t.Errorf("%s leaked private header %s", tc.path, name)
			}
		}
	}

	// A flushed body without a declared length still reaches the visitor
	// before its handler finishes.
	res, err := client.Get(origin + "/streamed")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.Header.Get("Content-Length") != "" || len(res.TransferEncoding) == 0 {
		t.Errorf("stream Content-Length=%q Transfer-Encoding=%v", res.Header.Get("Content-Length"), res.TransferEncoding)
	}
	reader := bufio.NewReader(res.Body)
	first := make(chan string, 1)
	go func() { line, _ := reader.ReadString('\n'); first <- line }()
	select {
	case line := <-first:
		if line != "literal-stream-first\n" {
			t.Fatalf("first stream line %q\n%s", line, log.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("stream held its first line until the handler finished\n%s", log.String())
	}
	if err := os.WriteFile(filepath.Join(app, "stream.release"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(reader)
	if err != nil || string(rest) != "literal-stream-second\n" {
		t.Fatalf("stream rest %q: %v", rest, err)
	}
}
