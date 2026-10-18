package gen

import (
	"go/token"
	"strings"
	"testing"
)

func TestCheckWireAdviceUsesAuthoredDeclarations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, field, consequence, repair string
		load                             bool
	}{
		{"nested unsupported", "Detail []map[string]string", "cannot cross the wire", "Use a named struct", false},
		{"duplicate name", "A string `json:\"same\"`; Alias string `json:\"same\"`", "duplicate serialized name", "distinct json name", false},
		{"deferred remote", "Later skgo.Deferred[string]", "Deferred", "server load", false},
		{"file result", "Upload skgo.File", "returns a skgo.File", "download URL", false},
		{"deferred file result", "Upload skgo.Deferred[skgo.File]", "returns a skgo.File", "download URL", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := wireDeclaration(t, "package fixture\nimport \"github.com/tylergannon/skgo\"\nvar _ skgo.File\ntype Output struct{\n"+tc.field+"\n}\n")
			typ := p.Types.Scope().Lookup("Output").Type()
			gp := &goPackage{pkg: p}
			pos := token.Position{Filename: "wire.go", Line: 4}
			a := &app{}
			if tc.load {
				a.loads = []*loadFn{{name: "load", out: typ, goPkg: gp, pos: pos}}
			} else {
				a.remotes = []*remoteFn{{name: "query", out: typ, goPkg: gp, pos: pos}}
			}
			err := a.checkFileUsage()
			if err == nil {
				err = a.checkWireFields()
			}
			if err == nil || !strings.Contains(err.Error(), tc.consequence) || !strings.Contains(err.Error(), tc.repair) || !strings.Contains(err.Error(), "wire.go:") {
				t.Fatalf("missing authored consequence/repair: %v", err)
			}
		})
	}
}
