package gen

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedTransportDecodedTypes(t *testing.T) {
	const hooks = `package hooks
import "github.com/tylergannon/skgo"
type RunSnapshot struct { JSON string ` + "`json:\"json\"`" + ` }
var _ = skgo.Transported[RunSnapshot]("RunSnapshot")
`
	const view = `package runview
import hooks "example.com/app/web/src"
type Data struct { Snapshot hooks.RunSnapshot ` + "`json:\"snapshot\"`" + ` }
`
	const remote = `package data
import("context";"example.com/app/runview";"github.com/tylergannon/skgo")
func viewRun(context.Context)(runview.Data,error){return runview.Data{},nil}
var _ = skgo.Query(viewRun)
`
	const load = `package runid
import("example.com/app/runview";"github.com/tylergannon/skgo")
func load(PageRequestEvent)(runview.Data,error){return runview.Data{},nil}
var _=skgo.Load(load)
`
	_, cfg := foreignFixture(t, remote, map[string]string{
		"app/web/src/hooks.go":                           hooks,
		"app/runview/detail.go":                          view,
		"app/web/src/routes/runs/[runID]/page.server.go": load,
	})
	p, err := OpenProject(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()
	for i := 0; i < 3; i++ {
		b := prepareRetained(t, p)
		if err := b.Commit(nil); err != nil {
			t.Fatal(err)
		}
		data := readFixtureFile(t, cfg.Web, "src/data/data.remote.ts")
		if !strings.Contains(data, "import type { RunSnapshot } from '../hooks'") && !strings.Contains(data, `import type { RunSnapshot } from "../hooks"`) {
			t.Fatalf("iteration %d lost decoded type:\n%s", i, data)
		}
		page := readFixtureFile(t, cfg.Web, "src/routes/runs/[runID]/+page.server.ts")
		if !strings.Contains(page, "import type { RunSnapshot } from '../../../hooks'") && !strings.Contains(page, `import type { RunSnapshot } from "../../../hooks"`) {
			t.Fatalf("iteration %d load lost decoded type:\n%s", i, page)
		}
		if i == 0 {
			files, err := findSourceFiles(cfg.Web)
			if err != nil {
				t.Fatal(err)
			}
			metadata, err := captureKitRouteMetadata(cfg, files)
			if err != nil {
				t.Fatal(err)
			}
			if _, changed, err := p.ObserveRoutes(metadata); err != nil || !changed {
				t.Fatalf("routes: %t %v", changed, err)
			}
		}
		if i == 1 {
			observeRetainedWrite(t, p, filepath.Join(cfg.Web, "src/hooks.go"), hooks+"\nconst revision = 2\n")
		}
	}
}
