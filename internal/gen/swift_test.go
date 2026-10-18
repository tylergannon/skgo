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
	root := t.TempDir()
	a, gp := emitterPackage(t, root, "package data\n")
	a.cfg.SwiftOut = filepath.Join(root, "Generated.swift")
	a.cfg.SwiftRemotes = []string{"src/lib/todos.remote.ts#read", "src/lib/todos.remote.ts#save"}
	a.remotes = []*remoteFn{
		{name: "read", module: "src/lib/todos.remote.ts", handler: "remote_read", kind: kindQuery, goPkg: gp, out: types.Typ[types.Int64]},
		{name: "save", module: "src/lib/todos.remote.ts", handler: "remote_save", kind: kindCommand, goPkg: gp, in: types.Typ[types.Int64], out: types.Typ[types.Int64]},
	}
	if err := a.planCodecs(); err != nil {
		t.Fatal(err)
	}
	if err := a.writeSwiftClient(); err != nil {
		t.Fatal(err)
	}
	if err := a.writeAppBindings(); err != nil {
		t.Fatal(err)
	}
	source := a.cfg.generation.files[a.cfg.SwiftOut]
	for _, want := range []string{"public func read() async throws -> Int64", "public func save(_ argument: Int64, updates: [QueryUpdate] = []) async throws -> Int64", `core.call("worolc/read", kind: .query, argument: nil)`, `core.call("worolc/save", kind: .command, argument: argument, updates: updates)`} {
		if !strings.Contains(string(source), want) {
			t.Fatalf("Swift output lacks %q:\n%s", want, source)
		}
	}
	bindings := a.cfg.generation.files[filepath.Join(a.cfg.Out, generatedGoFile)]
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
		{"missing", []string{"src/lib/todos.remote.ts#absent"}, kindQuery, types.Typ[types.String], "unknown Swift remote"},
		{"duplicate", []string{"src/lib/todos.remote.ts#read", "src/lib/todos.remote.ts#read"}, kindQuery, types.Typ[types.String], "duplicate Swift selection"},
		{"live", []string{"src/lib/todos.remote.ts#read"}, kindLive, types.Typ[types.String], "supports ordinary queries and commands"},
		{"transport", []string{"src/lib/todos.remote.ts#read"}, kindQuery, named, "custom transport unsupported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "Generated.swift")
			a := &app{cfg: Config{SwiftOut: path, SwiftRemotes: tc.selections}, remotes: []*remoteFn{{name: "read", module: "src/lib/todos.remote.ts", kind: tc.kind, out: tc.out}}, transported: []*transportedType{{key: "Money", named: named}}}
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
