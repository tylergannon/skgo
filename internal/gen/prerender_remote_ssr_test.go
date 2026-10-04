package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestCompiledProductionNoArgumentNullReachesTheArtifactLookup builds the
// production Goja bundle from an isolated authored remote. Kit's production
// wrapper must disable the no-argument validator before the invocation reaches
// Go; the missing null-key artifact then answers 500 without running the body.
func TestCompiledProductionNoArgumentNullReachesTheArtifactLookup(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app, err := copyExample(root, filepath.Join(t.TempDir(), "example"))
	if err != nil {
		t.Fatal(err)
	}
	ui := filepath.Join(app, "ui")
	if err := os.Rename(filepath.Join(app, "web"), ui); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "internal", "skgo", "config.go"), "--web ../../web", "--web ../../ui"); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "go.mod"), "ignore ./web/node_modules", "ignore ./ui/node_modules"); err != nil {
		t.Fatal(err)
	}
	if err := replaceOnce(filepath.Join(app, "cmd", "main.go"), "github.com/tylergannon/skgo/example/web", "github.com/tylergannon/skgo/example/ui"); err != nil {
		t.Fatal(err)
	}
	linkExampleFrontendDependencies(t, root, app)
	write := func(name, content string) {
		t.Helper()
		file := filepath.Join(ui, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("src/routes/prerender-contract/fixture.remote.go", `package prerendercontract

import (
	"context"
	"sync/atomic"

	"github.com/tylergannon/skgo"
)

var calls atomic.Int32

func noargValue(context.Context) (string, error) {
	calls.Add(1)
	return "live no-argument body ran", nil
}

func Calls() int32 { return calls.Load() }

var _ = skgo.Prerender(noargValue)
`)
	write("src/routes/prerender-contract/+page.svelte", `<script>import { noargValue } from './fixture.remote';const value=await noargValue(null);</script><h1>Prerender contract</h1><p>{value}</p>`)
	write("src/routes/prerender-contract/+error.svelte", `<script>let { error, status } = $props();</script><h1>{status}</h1><p>{error.message}</p>`)
	if output, err := runGoGenerate(app); err != nil {
		t.Fatalf("go generate ./...: %v\n%s", err, output)
	}
	cmd := exec.Command(filepath.Join(ui, "node_modules", ".bin", "vp"), "build")
	cmd.Dir = ui
	cmd.Env = append(os.Environ(), "GOWORK=off", "ORIGIN=http://127.0.0.1:8080")
	output, err := cmd.CombinedOutput()
	t.Logf("vp build exit: %d\n%s", exitCode(err), output)
	if err != nil {
		t.Fatalf("vp build: %v", err)
	}
	fixture := filepath.Join(app, "cmd", "prerender-fixture")
	if err := os.MkdirAll(fixture, 0o755); err != nil {
		t.Fatal(err)
	}
	fixtureTest := `package prerenderfixture_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	prerendercontract "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5yhezlsmvxgizlsfvrw63tuojqwg5a"
	ui "github.com/tylergannon/skgo/example/ui"
)

func TestProductionNoArgumentNullUsesArtifactMiss(t *testing.T) {
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil { t.Fatal(err) }
	handler, mode, err := example.NewHandler(dist, "", "http://127.0.0.1:8080")
	if err != nil { t.Fatal(err) }
	if mode != "prod" { t.Fatalf("mode = %q", mode) }
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/prerender-contract", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want opaque 500 (a 400 means Kit validated before artifact lookup): %s", recorder.Code, recorder.Body.String())
	}
	if got := prerendercontract.Calls(); got != 0 {
		t.Fatalf("the Go body ran %d times for the missing null-key artifact", got)
	}
	if strings.Contains(recorder.Body.String(), "live no-argument body ran") {
		t.Fatal("the missing artifact fell back to the Go body")
	}
}
`
	if err := os.WriteFile(filepath.Join(fixture, "ssr_test.go"), []byte(fixtureTest), 0o644); err != nil {
		t.Fatal(err)
	}
	goTest := exec.Command("go", "test", "./cmd/prerender-fixture", "-run", "^TestProductionNoArgumentNullUsesArtifactMiss$", "-count=1", "-v")
	goTest.Dir = app
	goTest.Env = append(os.Environ(), "GOWORK=off")
	testOutput, err := goTest.CombinedOutput()
	t.Logf("fixture Go SSR test exit: %d\n%s", exitCode(err), testOutput)
	if err != nil {
		t.Fatalf("compiled production SSR fixture: %v", err)
	}
}
