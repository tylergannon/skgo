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

// Kit's form input admits no null: an empty control is left out, not sent as
// null. A Nullable anywhere in a form's argument is refused at its field.
func TestAFormArgumentCannotBeNullable(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, fields, field string
		kind                remoteKind
	}{
		{"direct", "Nickname polytype.Nullable[string]", "Nickname", kindForm},
		{"nested", "Contacts []Contact", "Phone", kindForm},
		{"inside optional", "Primary polytype.Optional[Contact]", "Phone", kindForm},
		{"optional instead", "Nickname polytype.Optional[string]", "", kindForm},
		{"query argument", "Nickname polytype.Nullable[string]", "", kindQuery},
	} {
		t.Run(tc.name, func(t *testing.T) {
			imports := declarationImports{}
			imports[polytypePkg] = declarationPackage(t, polytypePkg, "polytype.go", "package polytype\ntype Nullable[T any] struct{ Valid bool; Value T }\ntype Optional[T any] struct{ Present bool; Value T }\n", imports).Types
			p := declarationPackage(t, "fixture", "wire.go", "package fixture\nimport \"github.com/tylergannon/polytype\"\ntype Contact struct{\nPhone polytype.Nullable[string]\n}\ntype Input struct{\n"+tc.fields+"\n}\n", imports)
			a := &app{remotes: []*remoteFn{{name: "save", kind: tc.kind, in: p.Types.Scope().Lookup("Input").Type(), goPkg: &goPackage{pkg: p}}}}
			err := a.checkFormNulls()
			if tc.field == "" {
				if err != nil {
					t.Fatalf("refused an argument kit admits: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "wire.go:") || !strings.Contains(err.Error(), "field "+tc.field+" ") || !strings.Contains(err.Error(), "polytype.Optional") {
				t.Fatalf("missing positioned refusal of %s: %v", tc.field, err)
			}
		})
	}
}
