//go:build skgo_dev_integration

package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Browser orchestration and source edits stay in Go. Only the actual Kit client
// assertions live in the existing Gherkin suite.
func TestRealSessionRouteEditDuringPendingLoad(t *testing.T) {
	realSession(t, false, false, false, "pending-browser")
}

func setupPendingBrowser(t *testing.T, app string, write func(string, string)) {
	t.Helper()
	entered, released := make(chan struct{}), make(chan struct{})
	var enterOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(released) }) }
	control := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hold":
			enterOnce.Do(func() { close(entered) })
			select {
			case <-released:
				io.WriteString(w, "literal-go-released-113")
			case <-r.Context().Done():
			}
		case "/entered":
			select {
			case <-entered:
				io.WriteString(w, "literal-go-waiting-113")
			default:
				http.Error(w, "load has not entered Go I/O", http.StatusConflict)
			}
		case "/edit":
			select {
			case <-entered:
			default:
				http.Error(w, "edit must happen during Go I/O", http.StatusConflict)
				return
			}
			select {
			case <-released:
				http.Error(w, "load already released", http.StatusConflict)
				return
			default:
			}
			files := map[string]string{
				"web/src/routes/pending/+layout.svelte":       `<script>let {children}=$props();</script><section data-testid="edited-layout"><h2>literal-new-layout-113</h2>{@render children()}</section>`,
				"web/src/routes/pending/changed/+page.svelte": `<script>let count=$state(0);</script><h1>literal-new-route-113</h1><button data-testid="edit-counter" onclick={()=>count++}>Clicked {count}</button>`,
			}
			for name, body := range files {
				path := filepath.Join(app, name)
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
				if err := os.WriteFile(path, []byte(body), 0644); err != nil {
					http.Error(w, err.Error(), 500)
					return
				}
			}
			io.WriteString(w, "literal-structure-edited-113")
		case "/release":
			release()
			io.WriteString(w, "literal-load-released-113")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(control.Close)
	t.Cleanup(release)
	t.Setenv("SKGO_E2E_PENDING_EDIT", control.URL)
	write("web/src/routes/pending/+page.svelte", `<script>let {data}=$props();</script><h1>literal-held-page-113</h1><p data-testid="go-receipt">{data.receipt}</p>`)
	write("web/src/routes/pending/page.server.go", fmt.Sprintf(`package pending
import("io";"net/http";"github.com/tylergannon/skgo")
type PageData struct{Receipt string `+"`json:\"receipt\"`"+`}
func load(event PageRequestEvent)(PageData,error){
 request,err:=http.NewRequestWithContext(event.Request().Context(),"GET",%q,nil);if err!=nil{return PageData{},err}
 response,err:=http.DefaultClient.Do(request);if err!=nil{return PageData{},err};defer response.Body.Close()
 receipt,err:=io.ReadAll(response.Body);return PageData{Receipt:string(receipt)},err
}
var _=skgo.Load(load)
`, control.URL+"/hold"))
}

func exercisePendingBrowser(t *testing.T, app, origin string, log *sessionLog) {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, filepath.Join(root, "example/web/node_modules/.bin/vp"), "run", "--no-cache", "test", "--grep", "route structure changes during pending Go I/O", "--project", "chromium", "--workers", "1")
	command.Dir = filepath.Join(root, "example/e2e")
	command.Env = append(os.Environ(), "BASE_URL="+origin, "SKGO_E2E_MODE=dev")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pending-edit browser: %v\n%s\n%s", err, output, log.String())
	}
	if !strings.Contains(string(output), "Running 1 test using 1 worker") || !strings.Contains(string(output), "1 passed") || strings.Contains(string(output), "skipped") {
		t.Fatalf("browser scenario did not run exactly once:\n%s", output)
	}
	t.Logf("%s", output)
}
