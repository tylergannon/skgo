package devrender_test

import (
	"bytes"
	"encoding/binary"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

func awaitCallerRoute(t *testing.T, present bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		m, err := skgo.ReadDevManifest(dist(), devServer)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, route := range m.Routes {
			found = found || route.ID == "/manifest-drift/[newkey]"
		}
		if found == present {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("live Kit graph: new caller present=%t, want %t", found, present)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Edit the real Vite tree without regenerating/rebuilding Go. Drift outside
// /contact must stop all its entry paths until Go catches up.
func TestLiveCallerManifestAheadOfGo(t *testing.T) {
	dir := filepath.Join(webRoot, "src", "routes", "manifest-drift", "[newkey]")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(filepath.Dir(dir)); err != nil {
			t.Error(err)
		}
		awaitCallerRoute(t, false)
	}()
	if err := os.WriteFile(filepath.Join(dir, "+page.svelte"), []byte("<p>New dev caller</p>"), 0644); err != nil {
		t.Fatal(err)
	}
	awaitCallerRoute(t, true)
	if _, _, err := example.NewHandler(dist(), devServer, origin); err == nil || !strings.Contains(err.Error(), "caller manifest drift") || !strings.Contains(err.Error(), "/manifest-drift/[newkey]") {
		t.Fatalf("startup accepted Vite ahead of Go: %v", err)
	}
	id := ""
	for _, fn := range generated.Remotes() {
		if fn.Name() == "sendMessage" {
			id = fn.ID()
		}
	}
	if id == "" {
		t.Fatal("missing generated contact form")
	}
	fields := url.Values{"from/" + id: {"Drift fixture"}, "email/" + id: {"drift@example.test"}, "body/" + id: {"Literal drift payload."}}
	header := `[[1,5],{"from":2,"email":3,"body":4},"Drift fixture","drift@example.test","Literal drift payload.",{}]`
	enhanced := make([]byte, 7+len(header))
	binary.LittleEndian.PutUint32(enhanced[1:5], uint32(len(header)))
	copy(enhanced[7:], header)
	for _, entry := range []struct {
		name, path, method, media string
		body                      []byte
	}{
		{"document", "/contact", "GET", "", nil},
		{"data", "/contact/__data.json", "GET", "", nil},
		{"native", "/contact?" + url.Values{"/remote": {id}}.Encode(), "POST", "application/x-www-form-urlencoded", []byte(fields.Encode())},
		{"keyed", "/contact?" + url.Values{"/remote": {id + `/"k1"`}}.Encode(), "POST", "application/x-www-form-urlencoded", []byte(fields.Encode())},
		{"enhanced", "/_app/remote/" + id, "POST", "application/x-sveltekit-formdata", enhanced},
	} {
		t.Run(entry.name, func(t *testing.T) {
			req := httptest.NewRequest(entry.method, entry.path, bytes.NewReader(entry.body))
			req.Header.Set("Origin", origin)
			req.Header.Set("Content-Type", entry.media)
			req.Header.Set("Accept", "text/html")
			req.Header.Set("x-sveltekit-pathname", "/contact")
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if !strings.Contains(rec.Body.String(), "caller manifest drift") || !strings.Contains(rec.Body.String(), "/manifest-drift/[newkey]") {
				t.Fatalf("%s: %d %s", entry.path, rec.Code, rec.Body.String())
			}
		})
	}
}
