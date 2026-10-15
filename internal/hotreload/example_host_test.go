//go:build skgo_example_host

package hotreload

import (
	"context"
	"github.com/tylergannon/skgo/internal/gen"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This opt-in integration runs the actual example constructor against a real
// frontend development snapshot produced by the adapter. The selected source
// tree is private; application generation never modifies the authored example.
func TestExampleDevelopmentHost(t *testing.T) {
	dist := os.Getenv("SKGO_EXAMPLE_HOST_DIST")
	if dist == "" {
		t.Fatal("supply SKGO_EXAMPLE_HOST_DIST from the running adapter")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	root, err := filepath.Abs("../../example")
	if err != nil {
		t.Fatal(err)
	}
	p, err := gen.OpenProject(gen.Config{Web: filepath.Join(root, "web"), Out: filepath.Join(root, "internal/skgo"), LocalsPackage: "github.com/tylergannon/skgo/example/internal/app", LocalsType: "Locals", HookPackage: "github.com/tylergannon/skgo/example/internal/serverhooks", CacheDir: "off"})
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	b, err := p.Prepare(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Cancel()
	defer b.Inputs.Close()
	h, err := BuildHost(ctx, HostOptions{Inputs: b.Inputs, WorkDir: t.TempDir(), Origin: "http://localhost", DistDir: dist, Output: os.Stderr})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	for _, check := range []struct{ path, body string }{{"/items/42", "Widget 42"}, {"/error/unexpected", "Something went wrong on our end."}} {
		r, e := http.Get(h.URL + check.path)
		if e != nil {
			t.Fatal(e)
		}
		body, e := io.ReadAll(r.Body)
		r.Body.Close()
		if e != nil || !strings.Contains(string(body), check.body) {
			t.Fatalf("%s status%d: %s; err%v", check.path, r.StatusCode, body, e)
		}
	}
}
