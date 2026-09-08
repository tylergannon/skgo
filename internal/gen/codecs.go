package gen

import (
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/tylergannon/polytype/devalue/codegen"
	"github.com/tylergannon/polytype/grammar"
	"github.com/tylergannon/polytype/typegrammar"
)

// devalueCodecsFile is the file polytype's codecs land in. There is one per
// app, in the generated bindings package, because that is the one package that
// can already name every type on the wire: it imports every package declaring
// a remote function, and a codec written anywhere else would either have to be
// duplicated per route or written into the directory of the type it encodes,
// which is not the app's to write into for a type from a dependency (#14).
const devalueCodecsFile = "skgo_devalue_gen.go"

// codecRoot is one type polytype is asked to emit an encoder and a strict
// decoder for.
//
// Roots are deduplicated by the type's own identity, so two functions that
// take the same argument type share one codec. The name is positional —
// polytype names an anonymous root `Root<i>` by its index in the slice it is
// given — which is why the slice's order is fixed here and never derived from
// a map.
type codecRoot struct {
	key string
	typ types.Type
	// loadDir is the package polytype loads to resolve the type. It is the
	// package that *names* the type in a signature, not the one that declares
	// it: polytype resolves a named root through the loaded package's own
	// dependency graph, and a package whose function takes the type
	// necessarily imports the package declaring it.
	loadDir string
	pos     token.Position
	name    string
}

// codecs is the app's root set: the ordered slice polytype is given, and the
// index that dedupes it.
type codecSet struct {
	roots []*codecRoot
	byKey map[string]*codecRoot
}

// root registers t and returns the name of the generated codec pair for it:
// `Encode<name>` and `Decode<name>`.
func (c *codecSet) root(t types.Type, loadDir string, pos token.Position) string {
	if c.byKey == nil {
		c.byKey = map[string]*codecRoot{}
	}
	key := types.TypeString(t, nil)
	if existing, ok := c.byKey[key]; ok {
		return existing.name
	}
	entry := &codecRoot{
		key:     key,
		typ:     t,
		loadDir: loadDir,
		pos:     pos,
		name:    fmt.Sprintf("Root%d", len(c.roots)),
	}
	c.roots = append(c.roots, entry)
	c.byKey[key] = entry
	return entry.name
}

// planCodecs decides, for every remote function, which codecs answer its
// argument and its result, and registers the roots polytype has to emit.
//
// Three shapes have no generated codec, and each for a reason that is kit's or
// polytype's rather than a shortcut:
//
//   - A form's argument. Kit posts a form as `application/x-sveltekit-formdata`,
//     whose POJO can carry an uploaded File. A File is not a JSON value and
//     polytype describes none, so the submission is assigned onto the
//     handler's own argument type by skgo.DecodeForm instead.
//   - A result that can reach a type the app transports. It has to reach
//     devalue as itself so the `transport` hook's reducer still recognises it;
//     see skgo.Call.Transported.
//   - A function declared without an argument. There is no argument type, and
//     kit's own validator for such a function refuses every argument.
func (a *app) planCodecs() error {
	for _, fn := range a.remotes {
		if fn.in != nil && fn.kind != kindForm {
			fn.inCodec = a.codecSet.root(fn.in, fn.goPkg.loadDir, fn.pos)
		}
		if !a.containsTransported(fn.out) {
			fn.outCodec = a.codecSet.root(fn.out, fn.goPkg.loadDir, fn.pos)
		}
	}
	// The app's transported types are roots too, which is what makes polytype
	// emit `EncodeMoney`/`DecodeMoney` — the two halves of the `transport`
	// hook's Go side.
	for _, entry := range a.transportKeyOrder() {
		a.codecSet.root(entry.named, entry.goPkg.loadDir, entry.pos)
	}
	return nil
}

// generateCodecs emits the Go encoders and strict decoders for every root.
//
// The codecs are generated rather than written by hand because they are the
// whole contract with the browser: whatever a decoder admits is what a Go
// function can be called with, and kit's client is the only thing on the other
// end. polytype already knows how to project a Go type into the devalue value
// model and how to reject a shape the type does not admit, with a JSON-pointer
// path in the diagnostic; a second answer here would be a worse one.
func (a *app) generateCodecs() error {
	if len(a.codecSet.roots) == 0 {
		return nil
	}

	// Group by the package polytype loads, since a root is resolved through a
	// loaded package's dependency graph.
	byDir := map[string][]int{}
	var dirs []string
	for i, entry := range a.codecSet.roots {
		if _, seen := byDir[entry.loadDir]; !seen {
			dirs = append(dirs, entry.loadDir)
		}
		byDir[entry.loadDir] = append(byDir[entry.loadDir], i)
	}
	sort.Strings(dirs)

	var defs typegrammarDefs
	nodes := make([]typegrammar.Type, len(a.codecSet.roots))
	for _, dir := range dirs {
		loaded, err := grammar.Load(dir)
		if err != nil {
			return fmt.Errorf("skgo: loading %s to generate its codecs: %w", dir, err)
		}
		indexes := byDir[dir]
		roots := make([]grammar.Root, 0, len(indexes))
		for _, i := range indexes {
			roots = append(roots, grammar.Root{Type: a.codecSet.roots[i].typ, Position: a.codecSet.roots[i].pos})
		}
		lowered, lowedNodes, err := loaded.Lower(roots)
		if err != nil {
			return fmt.Errorf("skgo: polytype cannot generate a codec for a type in %s: %w", dir, err)
		}
		defs.add(lowered)
		for n, i := range indexes {
			nodes[i] = lowedNodes[n]
		}
	}

	if err := checkRootNames(defs.definitions, len(nodes)); err != nil {
		return err
	}

	out, err := codegen.Generate(defs.definitions, nodes, codegen.Options{
		PackageName: a.cfg.Package,
		ImportPath:  a.bindingsImportPath(),
	})
	if err != nil {
		return fmt.Errorf("skgo: generating devalue codecs: %w", err)
	}
	return a.writeGo(filepath.Join(a.cfg.Out, devalueCodecsFile), string(out))
}

// rootName is what polytype calls the codec for the i-th anonymous root.
var rootName = regexp.MustCompile(`^Root[0-9]+$`)

// checkRootNames refuses an app whose own type would take the name polytype
// gives one of the roots.
//
// polytype names a definition after its Go type and an anonymous root after
// its position, and it moves a root out of the way when the two collide. skgo
// spells the root names itself — they are what the generated closures call —
// so a Go type called `Root3` would leave a closure calling a codec for
// something else. It is a silly name to have and a worse one to debug, so it
// is refused where it can still be renamed.
func checkRootNames(defs typegrammar.Definitions, count int) error {
	for _, def := range defs {
		name := def.Name.Name
		if name == "" {
			continue
		}
		exported := strings.ToUpper(name[:1]) + name[1:]
		if rootName.MatchString(exported) {
			return fmt.Errorf("skgo: %s.%s takes the name skgo gives a generated codec; rename it", def.Name.PackagePath, name)
		}
	}
	_ = count
	return nil
}

// typegrammarDefs accumulates the lowered definitions of every package that
// declares a type on the wire, into the single graph codegen is given.
// Definitions are keyed by their resolved name, so a type two packages both
// reach is added once.
type typegrammarDefs struct {
	definitions typegrammar.Definitions
	seen        map[typegrammar.Name]bool
}

func (d *typegrammarDefs) add(defs typegrammar.Definitions) {
	if d.seen == nil {
		d.seen = map[typegrammar.Name]bool{}
	}
	for _, def := range defs {
		if d.seen[def.Name] {
			continue
		}
		d.seen[def.Name] = true
		d.definitions = append(d.definitions, def)
	}
}

// bindingsImportPath is the import path of the generated bindings package,
// which codegen needs so that a type declared there is spelled unqualified.
func (a *app) bindingsImportPath() string {
	rel, err := filepath.Rel(a.hostDir, a.cfg.Out)
	if err != nil || rel == "." {
		return a.hostModule
	}
	return a.hostModule + "/" + filepath.ToSlash(rel)
}
