//go:build skgo_dev_integration

package devserver

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A route's Go file embeds the file beside it from a directory Go cannot
// spell, and an edit to that file reaches the running session.
func TestRealSessionRouteEmbed(t *testing.T) { realSession(t, false, false, false, "embed") }

const embedSessionDir = "web/src/routes/embedded/[name]"

func setupRouteEmbed(write func(string, string)) {
	write(embedSessionDir+"/server.go", `package embedded
import("embed";"net/http";"github.com/tylergannon/skgo")
//go:embed *.txt
var texts embed.FS
func get(w http.ResponseWriter,r *http.Request){http.ServeFileFS(w,r,texts,skgo.EventFrom(r.Context()).Param("name"))}
var _=skgo.GET(get)
`)
	write(embedSessionDir+"/greeting.txt", "literal-embed-172:one\n")
}

func exerciseRouteEmbed(t *testing.T, app, origin string, log *sessionLog) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	wait := func(want string) {
		t.Helper()
		var status int
		var body []byte
		var contentType string
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			res, err := client.Get(origin + "/embedded/greeting.txt")
			if err == nil {
				body, _ = io.ReadAll(res.Body)
				res.Body.Close()
				status, contentType = res.StatusCode, res.Header.Get("Content-Type")
				if status == 200 && string(body) == want && contentType == "text/plain; charset=utf-8" {
					return
				}
			}
			time.Sleep(80 * time.Millisecond)
		}
		t.Fatalf("embedded route answered status=%d type=%q body=%q, want 200 %q\n%s", status, contentType, body, want, log.String())
	}
	wait("literal-embed-172:one\n")
	if err := os.WriteFile(filepath.Join(app, embedSessionDir, "greeting.txt"), []byte("literal-embed-172:two\n"), 0644); err != nil {
		t.Fatal(err)
	}
	wait("literal-embed-172:two\n")
}
