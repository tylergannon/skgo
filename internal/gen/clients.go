package gen

import (
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"strings"

	"github.com/tylergannon/polytype/devalue/codegen"
	"github.com/tylergannon/polytype/grammar"
	"github.com/tylergannon/polytype/typegrammar"
	"github.com/tylergannon/skgo/internal/kithash"
)

// clientForm is intentionally a flat scalar Form. Other Forms still receive
// browser/server bindings, but no Go client until their wire types are covered.
func clientForm(fn *remoteFn) bool {
	if fn.kind != kindForm || fn.in == nil {
		return false
	}
	in := types.Unalias(fn.in)
	if named, ok := in.(*types.Named); ok {
		in = named.Underlying()
	}
	fields, ok := in.(*types.Struct)
	if !ok {
		return false
	}
	for i := 0; i < fields.NumFields(); i++ {
		if !fields.Field(i).Exported() || !clientScalar(fields.Field(i).Type(), true) {
			return false
		}
	}
	out := types.Unalias(fn.out)
	if named, ok := out.(*types.Named); ok {
		out = named.Underlying()
	}
	if result, ok := out.(*types.Struct); ok {
		for i := 0; i < result.NumFields(); i++ {
			if !result.Field(i).Exported() || !clientScalar(result.Field(i).Type(), true) {
				return false
			}
		}
		return true
	}
	return clientScalar(fn.out, false)
}

func clientScalar(t types.Type, optional bool) bool {
	t = types.Unalias(t)
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		if optional && obj != nil && obj.Pkg() != nil && obj.Pkg().Path() == "github.com/tylergannon/polytype" && obj.Name() == "Optional" && named.TypeArgs() != nil && named.TypeArgs().Len() == 1 {
			return clientScalar(named.TypeArgs().At(0), false)
		}
		t = named.Underlying()
	}
	basic, ok := t.(*types.Basic)
	if !ok {
		return false
	}
	return basic.Info()&(types.IsString|types.IsBoolean|types.IsInteger|types.IsFloat) != 0
}

func (a *app) writeFormClients() error {
	dir := filepath.Join(a.cfg.Out, "client")
	var forms []*remoteFn
	for _, fn := range a.remotes {
		if clientForm(fn) && !a.containsTransported(fn.out) {
			forms = append(forms, fn)
		}
	}
	if len(forms) == 0 {
		// Clear previously generated clients when declarations cease to qualify.
		for _, name := range []string{"skgo_client_gen.go", "skgo_client_devalue_gen.go"} {
			if err := os.Remove(filepath.Join(dir, name)); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		return nil
	}
	var b strings.Builder
	b.WriteString(goHeader + "package client\n\nimport (\n\t\"context\"\n\t\"github.com/tylergannon/skgo\"\n")
	for _, gp := range a.pkgs {
		used := false
		for _, fn := range forms {
			if fn.goPkg == gp {
				used = true
				break
			}
		}
		if used {
			fmt.Fprintf(&b, "\t%s %q\n", gp.alias, gp.pkg.PkgPath)
		}
	}
	b.WriteString(")\n\n// Client uses the same enhanced Form endpoint as the browser.\n")
	b.WriteString("type Client struct { skgo.FormClient }\n")
	var defs typegrammarDefs
	nodes := make([]typegrammar.Type, len(forms))
	for i, fn := range forms {
		loaded, err := grammar.Load(fn.goPkg.loadDir)
		if err != nil {
			return fmt.Errorf("skgo: loading Form client result: %w", err)
		}
		lowered, roots, err := loaded.Lower([]grammar.Root{{Type: fn.out, Position: fn.pos}})
		if err != nil {
			return fmt.Errorf("skgo: Form client %s result: %w", fn.name, err)
		}
		defs.add(lowered)
		nodes[i] = roots[0]
		method := strings.TrimPrefix(fn.handler, "remote_")
		method = strings.ToUpper(method[:1]) + method[1:]
		fmt.Fprintf(&b, "\n// %s submits %s#%s once.\n", method, fn.module, fn.name)
		fmt.Fprintf(&b, "func (c Client) %s(ctx context.Context, in %s.SkgoArg_%s) (%s.SkgoOut_%s, error) {\n", method, fn.goPkg.alias, fn.name, fn.goPkg.alias, fn.name)
		fmt.Fprintf(&b, "\treturn skgo.SubmitForm(ctx, c.FormClient, %q, in, DecodeRoot%d)\n}\n", kithash.Kit(fn.module)+"/"+fn.name, i)
	}
	codecs, err := codegen.Generate(defs.definitions, nodes, codegen.Options{PackageName: "client", ImportPath: a.bindingsImportPath() + "/client"})
	if err != nil {
		return fmt.Errorf("skgo: generating Form client codecs: %w", err)
	}
	if err := a.writeGo(filepath.Join(dir, "skgo_client_gen.go"), b.String()); err != nil {
		return err
	}
	return a.writeGo(filepath.Join(dir, "skgo_client_devalue_gen.go"), string(codecs))
}
