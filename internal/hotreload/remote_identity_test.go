package hotreload

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

type remoteIdentityImporter struct{ sdk *types.Package }

func (i remoteIdentityImporter) Import(path string) (*types.Package, error) { return i.sdk, nil }

func TestNativeLowerPreservesRequestedRemoteIdentity(t *testing.T) {
	for _, name := range []string{"RefreshNoArg", "RefreshRequestedNoArg", "Requested", "IgnoreRequested", "IgnoreRequestedNoArg", "ReconnectRequested", "ReconnectRequestedNoArg"} {
		t.Run(name, func(t *testing.T) {
			// SDK calls use a function value as a registration identity, independent
			// of the ABI/body signature the real generic API accepts.
			sdk := types.NewPackage("github.com/tylergannon/skgo", "skgo")
			anyType := types.NewInterfaceType(nil, nil).Complete()
			parameters := types.NewTuple(types.NewVar(token.NoPos, sdk, "ctx", anyType), types.NewVar(token.NoPos, sdk, "fn", anyType))
			sdk.Scope().Insert(types.NewFunc(token.NoPos, sdk, name, types.NewSignatureType(nil, nil, nil, parameters, nil, false)))
			sdk.MarkComplete()
			source := `package fixture
import "github.com/tylergannon/skgo"
func watchCount(){}
func signIn(){skgo.` + name + `(nil,watchCount)}
`
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "fixture.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
			checker := types.Config{Importer: remoteIdentityImporter{sdk}}
			pkg, err := checker.Check("example.com/fixture", fset, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			lowered, err := Lower(&packages.Package{Name: "fixture", PkgPath: pkg.Path(), Fset: fset, Syntax: []*ast.File{file}, Types: pkg, TypesInfo: info}, map[string][]byte{"fixture.go": []byte(source)})
			if err != nil {
				t.Fatal(err)
			}
			for _, view := range []map[string][]byte{lowered.Host, lowered.Candidate} {
				found := false
				for _, body := range view {
					if strings.Contains(string(body), "skgo."+name+"(nil, watchCount)") {
						found = true
					}
					if strings.Contains(string(body), "skgo."+name+"(nil, _skgo_body_watchCount)") {
						t.Fatalf("%s was given a revision body identity instead of its registered host function", name)
					}
				}
				if !found {
					t.Fatalf("%s stable identity call was lost", name)
				}
			}
		})
	}
}
