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

// A route's Go file embeds the files beside it from a directory Go cannot
// spell, and a change to either reaches the running session. One file is named
// by itself and the others by a pattern in a subdirectory: the session must see
// an edit to the first and a new file matching the second.
func TestRealSessionRouteEmbed(t *testing.T) { realSession(t, false, false, false, "embed") }

const embedSessionDir = "web/src/routes/embedded/[name]"

func setupRouteEmbed(write func(string, string)) {
	write(embedSessionDir+"/server.go", `package embedded
import("embed";"io";"net/http";"github.com/tylergannon/skgo")
//go:embed greeting.txt
var greeting string
//go:embed notes/*.txt
var notes embed.FS
func get(w http.ResponseWriter,r *http.Request){
name:=skgo.EventFrom(r.Context()).Param("name")
if name=="greeting.txt"{w.Header().Set("Content-Type","text/plain; charset=utf-8");io.WriteString(w,greeting);return}
http.ServeFileFS(w,r,notes,"notes/"+name)}
var _=skgo.GET(get)
`)
	write(embedSessionDir+"/greeting.txt", "literal-embed-172:one\n")
	write(embedSessionDir+"/notes/note.txt", "literal-note-172:one\n")
}

func exerciseRouteEmbed(t *testing.T, app, origin string, log *sessionLog) {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	wait := func(file, want string) {
		t.Helper()
		var status int
		var body []byte
		var contentType string
		deadline := time.Now().Add(45 * time.Second)
		for time.Now().Before(deadline) {
			res, err := client.Get(origin + "/embedded/" + file)
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
		t.Fatalf("embedded %s answered status=%d type=%q body=%q, want 200 %q\n%s", file, status, contentType, body, want, log.String())
	}
	edit := func(rel, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(app, embedSessionDir, rel), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	wait("greeting.txt", "literal-embed-172:one\n")
	wait("note.txt", "literal-note-172:one\n")
	// One change at a time: either rebuild would pick up both. An edit to a
	// file named by itself, then a new file matching the pattern.
	edit("greeting.txt", "literal-embed-172:two\n")
	wait("greeting.txt", "literal-embed-172:two\n")
	edit("notes/added.txt", "literal-note-172:added\n")
	wait("added.txt", "literal-note-172:added\n")
}
