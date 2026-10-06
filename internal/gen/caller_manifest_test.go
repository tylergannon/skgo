package gen

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Build a changed frontend over unchanged generated Go. The route is outside
// the caller the application would select, and has no Go load or hook.
func TestFrontendRebuildWithoutCallerGeneration(t *testing.T) {
	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	app := sandboxExample(t)
	web := filepath.Join(app, "web")
	dependencies := filepath.Join(web, "node_modules")
	if err := os.RemoveAll(dependencies); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "example", "web", "node_modules"), dependencies); err != nil {
		t.Fatal(err)
	}
	writeSharedFixture(t, app, "web/src/routes/manifest-drift/[newkey]/+page.svelte", "<p>New frontend caller</p>")
	command := exec.Command(filepath.Join(dependencies, ".bin", "vp"), "build")
	command.Dir = web
	command.Env = append(os.Environ(), "ORIGIN=http://127.0.0.1:8080")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("frontend rebuild: %v\n%s", err, output)
	}
	writeSharedFixture(t, app, "internal/skgo/manifest_test.go", `package skgo_test
import (
 "io/fs"
 "strings"
 "testing"
 "github.com/tylergannon/skgo"
 generated "github.com/tylergannon/skgo/example/internal/skgo"
 "github.com/tylergannon/skgo/example/web"
)
func TestRebuiltCallerSnapshotRejected(t *testing.T){
 dist,err:=fs.Sub(web.Build,"build");if err!=nil{t.Fatal(err)}
 m,err:=skgo.ReadManifest(dist);if err!=nil{t.Fatal(err)}
 found:=false;for _,route:=range m.Routes{if route.ID=="/manifest-drift/[newkey]"{found=true;if len(route.Params)!=1||route.Params[0].Name!="newkey"{t.Fatalf("new route metadata: %+v",route)}}};if !found{t.Fatal("real frontend build did not include new caller")}
 _,err=skgo.NewRemotes(m.RemoteConfig("http://127.0.0.1:8080"),generated.Remotes()...)
 if err==nil||!strings.Contains(err.Error(),"caller manifest drift")||!strings.Contains(err.Error(),"/manifest-drift/[newkey]")||!strings.Contains(err.Error(),"no generated Params constructor"){t.Fatalf("stale Go accepted rebuilt frontend: %v",err)}
}
`)
	command = exec.Command("go", "test", "-count=1", "-v", "-run", "^TestRebuiltCallerSnapshotRejected$", "./internal/skgo")
	command.Dir = app
	output, err := command.CombinedOutput()
	if err != nil || strings.Contains(string(output), "SKIP") || !strings.Contains(string(output), "--- PASS: TestRebuiltCallerSnapshotRejected") {
		t.Fatalf("rebuilt snapshot startup contract: %v\n%s", err, output)
	}
}
