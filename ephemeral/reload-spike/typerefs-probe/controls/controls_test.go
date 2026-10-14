// These controls characterize the upstream defect at the pinned version.
// The stale-diagnostic control should fail when the defect is repaired.
package controls

import (
	. "golang.org/x/tools/gopls/internal/test/integration"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

const files = `
-- go.mod --
module example.test

go 1.27.0
-- storage/storage.go --
package storage
const Code = 1
-- model/model.go --
package model
import "example.test/storage"
const Explicit int = storage.Code
type Block [Explicit]byte
-- api/api.go --
package api
import "example.test/model"
func New() model.Block { return model.Block{} }
-- client/client.go --
package client
import "example.test/api"
var Value [1]byte = api.New()
`

func TestReviewerExplicitStaleThenImporterTouch(t *testing.T) {
	WithOptions(Modes(Default)).Run(t, files, func(t *testing.T, env *Env) {
		env.OpenFile("client/client.go")
		env.OpenFile("storage/storage.go")
		env.AfterChange(NoDiagnostics())
		env.RegexpReplace("storage/storage.go", "Code = (1)", "2")
		env.AfterChange(NoDiagnostics())
		time.Sleep(250 * time.Millisecond)
		env.AfterChange(NoDiagnostics())
		env.SaveBufferWithoutActions("storage/storage.go")
		env.AfterChange(NoDiagnostics())
		_, err := env.Sandbox.RunGoCommand(env.Ctx, "", "test", []string{"./client"}, nil, false)
		if err == nil || !strings.Contains(err.Error(), "cannot use api.New()") {
			t.Fatalf("independent compiler: err=%v", err)
		}
		t.Logf("compiler independently rejects saved Code2: %v", err)
		env.OpenFile("api/api.go")
		env.AfterChange(NoDiagnostics())
		env.RegexpReplace("api/api.go", "package api", "package api\n// changed importer source")
		env.AfterChange(Diagnostics(env.AtRegexp("client/client.go", `api.New\(\)`)))
		t.Log("comment-only importer edit causes the previously missing client diagnostic")
	})
}

func TestFreshCodeTwoReportsError(t *testing.T) {
	WithOptions(Modes(Default)).Run(t, strings.Replace(files, "Code = 1", "Code = 2", 1), func(t *testing.T, env *Env) {
		env.OpenFile("client/client.go")
		env.AfterChange(Diagnostics(env.AtRegexp("client/client.go", `api.New\(\)`)))
	})
}
