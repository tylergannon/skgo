package snapshotprobe

import (
	"os"
	"strings"
	"testing"

	. "golang.org/x/tools/gopls/internal/test/integration"
)

func TestMain(m *testing.M) { os.Exit(Main(m)) }

// Exercise the complete upstream snapshot machinery, not typerefs alone.
func TestImportedConstantInvalidation(t *testing.T) {
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
	for _, explicit := range []bool{false, true} {
		name := "inferred"
		fixture := strings.Replace(files, "const Explicit int =", "const Explicit =", 1)
		if explicit {
			name, fixture = "explicit", files
		}
		t.Run(name, func(t *testing.T) {
			WithOptions(Modes(Default)).Run(t, fixture, func(t *testing.T, env *Env) {
				env.OpenFile("client/client.go")
				env.OpenFile("storage/storage.go")
				env.AfterChange(NoDiagnostics())
				env.RegexpReplace("storage/storage.go", "Code = (1)", "2")
				env.AfterChange(Diagnostics(env.AtRegexp("client/client.go", `api.New\(\)`)))
				env.RegexpReplace("storage/storage.go", "Code = (2)", "1")
				env.AfterChange(NoDiagnostics())
			})
		})
	}
}
