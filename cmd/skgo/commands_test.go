package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/commands"
)

// The command table in internal/commands describes each command's flags as
// data, because the usage and the documentation render from it. This test
// reads the flag definitions out of this package's source and requires the
// table to match them: same names, same defaults, same usage sentences, same
// order.
func TestCommandTableMatchesFlagSets(t *testing.T) {
	owners := map[string]string{
		"buildCommand":       "build",
		"devCommand":         "dev",
		"environmentCommand": "env",
		"showAdvice":         "advice",
		"checkProject":       "check",
		"generate":           "generate",
		"kitPatchCommand":    "kit-patch",
		"nativeCommand":      "native build",
		"updateCommand":      "update",
		"newCommand":         "new",
		"addCommand":         "add",
		"buildinfoCommand":   "buildinfo",
	}
	defined := map[string][]commands.Flag{}
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			command, owned := owners[fn.Name.Name]
			if !owned {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				if flag, ok := flagDefinition(call); ok {
					defined[command] = append(defined[command], flag)
				}
				return true
			})
		}
	}
	for _, command := range commands.All() {
		got := defined[command.Name]
		if len(got) != len(command.Flags) {
			t.Errorf("%s: source defines %d flags, table lists %d\nsource: %+v\ntable:  %+v", command.Name, len(got), len(command.Flags), got, command.Flags)
			continue
		}
		for i := range got {
			if got[i] != command.Flags[i] {
				t.Errorf("%s flag %d: source %+v, table %+v", command.Name, i, got[i], command.Flags[i])
			}
		}
		delete(defined, command.Name)
	}
	for name := range defined {
		t.Errorf("source defines flags for %q, which the table does not list", name)
	}
}

// flagDefinition recognises fs.String("name", "default", "usage") and its
// Bool, Int, Var, Func and *Var relatives.
func flagDefinition(call *ast.CallExpr) (commands.Flag, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return commands.Flag{}, false
	}
	method := sel.Sel.Name
	var nameArg, defaultArg, usageArg ast.Expr
	switch method {
	case "String", "Bool", "Int", "Duration":
		if len(call.Args) != 3 {
			return commands.Flag{}, false
		}
		nameArg, defaultArg, usageArg = call.Args[0], call.Args[1], call.Args[2]
	case "StringVar", "BoolVar", "IntVar", "DurationVar":
		if len(call.Args) != 4 {
			return commands.Flag{}, false
		}
		nameArg, defaultArg, usageArg = call.Args[1], call.Args[2], call.Args[3]
	case "Var", "Func":
		if len(call.Args) != 3 {
			return commands.Flag{}, false
		}
		if method == "Var" {
			nameArg, usageArg = call.Args[1], call.Args[2]
		} else {
			nameArg, usageArg = call.Args[0], call.Args[1]
		}
	default:
		return commands.Flag{}, false
	}
	name, ok := literal(nameArg)
	if !ok {
		return commands.Flag{}, false
	}
	usage, ok := literal(usageArg)
	if !ok {
		return commands.Flag{}, false
	}
	def := ""
	if defaultArg != nil {
		def, ok = literal(defaultArg)
		if !ok {
			return commands.Flag{}, false
		}
	}
	return commands.Flag{Name: name, Default: def, Usage: usage}, true
}

func literal(e ast.Expr) (string, bool) {
	switch v := e.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			s, err := strconv.Unquote(v.Value)
			return s, err == nil
		}
		return v.Value, true
	case *ast.Ident:
		if v.Name == "true" || v.Name == "false" {
			return v.Name, true
		}
	}
	return "", false
}

func TestUsageListsEveryCommand(t *testing.T) {
	for _, command := range commands.All() {
		if !strings.Contains(usage, "\tskgo "+command.Name) {
			t.Errorf("usage does not list %s:\n%s", command.Name, usage)
		}
	}
	if strings.Contains(usage, "_plugin-probe") {
		t.Error("usage lists the internal probe")
	}
}

// The documentation runs each command's help at build time and fails on a
// non-zero exit, so every listed command, and skgo itself, must print help
// for -h and --help and succeed. One shared binary serves every case; each
// runs in an empty directory with an empty home so no application or
// installed plugin is found.
func TestEveryCommandHelpExitsZero(t *testing.T) {
	bin := skgoBinary(t)
	invocations := [][]string{nil}
	for _, command := range commands.All() {
		invocations = append(invocations, strings.Fields(command.Name))
	}
	for _, words := range invocations {
		for _, help := range []string{"-h", "--help"} {
			args := append(append([]string{}, words...), help)
			t.Run(strings.Join(args, " "), func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				cmd := exec.CommandContext(t.Context(), bin, args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "HOME="+dir, "SKGO_PLUGIN_DIRS=")
				var output bytes.Buffer
				cmd.Stdout, cmd.Stderr = &output, &output
				if err := cmd.Run(); err != nil {
					t.Fatalf("skgo %s: %v\n%s", strings.Join(args, " "), err, output.String())
				}
				if strings.TrimSpace(output.String()) == "" {
					t.Fatalf("skgo %s printed nothing", strings.Join(args, " "))
				}
			})
		}
	}
}
