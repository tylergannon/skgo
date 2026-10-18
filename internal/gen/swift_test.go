package gen

import (
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tg "github.com/tylergannon/polytype/typegrammar"
)

func TestNativeSwiftGeneration(t *testing.T) {
	t.Parallel()
	root, cfg := foreignFixture(t, `package data
import("context";"github.com/tylergannon/skgo";"example.com/app/generated/params")
func read(context.Context)(int64,error){return 42,nil}
func save(context.Context,params.RequestEvent,int64)(int64,error){return 42,nil}
var _=skgo.Query(read)
var _=skgo.Command(save)
`, nil)
	cfg.SwiftOut = filepath.Join(root, "app", "Generated.swift")
	cfg.SwiftRemotes = []string{"src/data/data.remote.ts#read", "src/data/data.remote.ts#save"}
	if err := Run(cfg); err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(cfg.SwiftOut)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"func read(", "func save(", "Int64"} {
		if !strings.Contains(string(source), want) {
			t.Fatalf("Swift output lacks %q:\n%s", want, source)
		}
	}
	bindings := readFixtureFile(t, root, "app/generated/skgo_gen.go")
	for _, want := range []string{`skgoNativeCheck0(in, "", 0)`, `skgoNativeCheck0(out, "", 0)`, "9007199254740991", "Skgo_save(event.Context(), event, in)"} {
		if !strings.Contains(bindings, want) {
			t.Fatalf("native binding lacks %q:\n%s", want, bindings)
		}
	}
}

func TestNativeNumericGuardEmitter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		kind tg.ScalarKind
		typ  types.Type
		want string
	}{
		{tg.Int64, types.Typ[types.Int64], "int64(value) < -9007199254740991 || int64(value) > 9007199254740991"},
		{tg.Uint64, types.Typ[types.Uint64], "uint64(value) > 9007199254740991"},
		{tg.Float64, types.Typ[types.Float64], "math.IsNaN(float64(value)) || math.IsInf(float64(value), 0)"},
	} {
		guard := nativeGuards{names: map[string]string{}, imports: fileImports{byPkg: map[*types.Package]string{}, used: map[string]bool{}}}
		guard.check(&tg.Scalar{Kind: tc.kind}, tc.typ)
		if guard.err != nil || !strings.Contains(guard.out.String(), tc.want) {
			t.Fatalf("numeric guard: %v\n%s", guard.err, guard.out.String())
		}
	}
}

func TestNativeSelectionAdmission(t *testing.T) {
	t.Parallel()
	pkg := types.NewPackage("example.com/domain", "domain")
	named := types.NewNamed(types.NewTypeName(token.NoPos, pkg, "Money", nil), types.Typ[types.Int], nil)
	for _, tc := range []struct {
		name       string
		selections []string
		kind       remoteKind
		out        types.Type
		want       string
	}{
		{"missing", []string{"src/data.remote.ts#absent"}, kindQuery, types.Typ[types.String], "unknown Swift remote"},
		{"duplicate", []string{"src/data.remote.ts#read", "src/data.remote.ts#read"}, kindQuery, types.Typ[types.String], "duplicate Swift selection"},
		{"live", []string{"src/data.remote.ts#read"}, kindLive, types.Typ[types.String], "supports ordinary queries and commands"},
		{"transport", []string{"src/data.remote.ts#read"}, kindQuery, named, "custom transport unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Generated.swift")
			a := &app{cfg: Config{SwiftOut: path, SwiftRemotes: tc.selections}, remotes: []*remoteFn{{name: "read", module: "src/data.remote.ts", kind: tc.kind, out: tc.out}}, transported: []*transportedType{{key: "Money", named: named}}}
			if err := a.writeSwiftClient(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "Authored.swift")
	const authored = "struct Authored {}\n"
	writeSharedFixture(t, filepath.Dir(path), filepath.Base(path), authored)
	if err := checkSwiftOutput(path); err == nil || !strings.Contains(err.Error(), "authored") {
		t.Fatalf("authored output admitted: %v", err)
	}
	source, err := os.ReadFile(path)
	if err != nil || string(source) != authored {
		t.Fatalf("authored output changed: %q %v", source, err)
	}
}
