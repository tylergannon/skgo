package gen

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// The contract tests in clients_test.go, stalegen_test.go and check_test.go
// all ask the generator the same kind of question: the developer changed their
// Go and ran `go generate ./...`; what came out? Each used to copy the example
// and run the generator for itself, several times over, at several seconds a
// pass. The changes they plant do not overlap, so they are planted together in
// one evolved copy of the example, generated once, and each test asserts on
// that one result. The follow-up generations that need a different starting
// tree — a tree whose generated files went stale, and the untouched example
// regenerated over its own committed output — each run once on their own
// copy, concurrently with the first.

// evolvedEdit is one change a contract test plants in the example's source.
// old must occur exactly once, so a fixture that drifts fails here rather
// than silently planting nothing.
type evolvedEdit struct {
	file, old, new string
}

const (
	optionalSource = "web/src/routes/optional/optional.remote.go"
	streamSource   = "web/src/routes/stream/page.server.go"
)

// deferredPayload is a named Deferred payload whose fields do not collide,
// carrying a nullable member, reached by value, through a slice and through an
// array. TestNamedDeferredPayloadSerializedNames plants the colliding version
// in its own sandbox; this is its valid counterpart.
const deferredPayload = "type Payload struct {\n\tFirst string `json:\"same\"`\n\tSecond string `json:\"second\"`\n\tMaybe polytype.Nullable[string] `json:\"maybe\"`\n}\n\n"

var deferredRepresentations = []struct{ name, field string }{
	{"named", "\n\tPayload skgo.Deferred[Payload] `json:\"payload\"`"},
	{"slice", "\n\tPayloads skgo.Deferred[[]Payload] `json:\"payloads\"`"},
	{"array", "\n\tPayloadPair skgo.Deferred[[2]Payload] `json:\"payloadPair\"`"},
}

func evolvedEdits() []evolvedEdit {
	fields := ""
	for _, r := range deferredRepresentations {
		fields += r.field
	}
	return []evolvedEdit{
		// TestFormClientInputContractEvolution: a Form input field changes type.
		{optionalSource, "Enabled polytype.Optional[bool]   `json:\"enabled,omitzero\"`", "Enabled polytype.Optional[string] `json:\"enabled,omitzero\"`"},
		{optionalSource, "present: %t", "present: %q"},
		// TestFormClientResultContractEvolution: a Form result field changes type.
		{optionalSource, "Operations int    `json:\"operations\"`", "Operations string `json:\"operations\"`"},
		{optionalSource, "result.Operations = operations.byName[in.Name]", "result.Operations = fmt.Sprint(operations.byName[in.Name])"},
		// TestFormClientGenerationRecoversAndTracksContract: the Form result
		// gains a field.
		{optionalSource, "type Result struct {", "type Result struct {\n\tExtra string `json:\"extra\"`"},
		// TestFormClientGenerationRecoversAndTracksContract: a second Form.
		// TestCheckAcceptsNullableWithGeneratorProjection: a Form whose input
		// carries a nullable field. It is a third Form beside submit rather
		// than a field on submit's Input, because a Nullable input takes a
		// Form out of the Go client, whose contract the tests above assert on
		// submit.
		{optionalSource, "var _ = skgo.Form(submit)\n", "var _ = skgo.Form(submit)\n\n" +
			"func submitAgain(ctx context.Context, in Input) (Result, error) { return submit(ctx, in) }\n" +
			"var _ = skgo.Form(submitAgain)\n\n" +
			"type NullableInput struct {\n\tName string `json:\"name\"`\n\tNickname polytype.Nullable[string] `json:\"nickname\"`\n}\n\n" +
			"func submitNullable(ctx context.Context, in NullableInput) (Result, error) { return submit(ctx, Input{Name: in.Name}) }\n" +
			"var _ = skgo.Form(submitNullable)\n"},
		// TestNamedDeferredPayloadSerializedNames: named, slice and array
		// Deferred payloads of a type with a nullable member.
		{streamSource, "// Panel is one section of the page.", deferredPayload + "// Panel is one section of the page."},
		{streamSource, "type PageData struct {", "type PageData struct {" + fields},
		{streamSource, "\"github.com/tylergannon/skgo\"", "\"github.com/tylergannon/skgo\"\n\t\"github.com/tylergannon/polytype\""},
	}
}

// clientFiles are the generated Go client, relative to the app.
var clientFiles = []string{
	"internal/skgo/client/skgo_client_gen.go",
	"internal/skgo/client/skgo_client_devalue_gen.go",
}

// generation is one `go generate ./...` and what it left behind.
type generation struct {
	dir     string
	out     string
	err     error
	clients [][]byte
	// build is `go build ./...` over the bindings package afterwards, when
	// the generation's claim includes that the tree it left compiles.
	buildOut string
	buildErr error
	// staleFiles is how many skgo_remotes_gen.go files were corrupted first.
	staleFiles int
}

type evolvedApp struct {
	setupErr error
	// first is the generation over the evolved source, starting with the Go
	// client deleted. Its tree is shared: tests read it and add their own
	// consumer packages outside internal/skgo, and nothing writes to it again.
	first generation
	// check is Check over the evolved source after first.
	check func() error
	// stale generates a copy of the evolved source whose Go client files hold
	// garbage and whose every skgo_remotes_gen.go calls a symbol skgo does not
	// export (#91). It needs nothing from first, so it runs beside it.
	stale func() generation
}

var evolved = sync.OnceValue(func() *evolvedApp {
	// The one test that asks for regenerated also asks for this; start it
	// now rather than after this returns.
	go regenerated()
	e := &evolvedApp{}
	app, err := sharedSandbox("evolved")
	if err == nil {
		err = applyEdits(app, evolvedEdits())
	}
	staleDir := filepath.Join(packageTemp, "evolved-stale")
	if err == nil {
		err = copySandboxTree(app, staleDir, nil)
	}
	if err == nil {
		for _, f := range clientFiles {
			if err = os.Remove(filepath.Join(app, filepath.FromSlash(f))); err != nil {
				break
			}
		}
	}
	if err != nil {
		e.setupErr = err
		return e
	}
	e.stale = start(func() generation {
		g := generation{dir: staleDir}
		stale, err := findGeneratedRemoteFilesIn(staleDir)
		if err == nil && len(stale) == 0 {
			err = fmt.Errorf("no skgo_remotes_gen.go found under %s", staleDir)
		}
		for _, f := range stale {
			if err == nil {
				err = corruptGeneratedFileAt(f)
			}
		}
		for _, f := range clientFiles {
			if err == nil {
				err = os.WriteFile(filepath.Join(staleDir, filepath.FromSlash(f)), []byte("stale generated client\n"), 0o644)
			}
		}
		if err != nil {
			g.err = fmt.Errorf("preparing the stale tree: %w", err)
			return g
		}
		g = generate(staleDir)
		g.staleFiles = len(stale)
		if g.err == nil {
			g.buildOut, g.buildErr = runGoBuild(filepath.Join(staleDir, "internal", "skgo"))
		}
		return g
	})

	e.first = generate(app)
	if e.first.err != nil {
		// Every test asserting on this fixture fails on it in requireEvolved.
		return e
	}
	e.check = start(func() error {
		defer tlog("evolved check")()
		return Check(Config{Web: filepath.Join(app, "web"), Out: filepath.Join(app, "internal", "skgo")})
	})
	e.first.buildOut, e.first.buildErr = runGoBuild(filepath.Join(app, "internal", "skgo"))
	return e
})

// regenerated is `go generate ./...` over an untouched copy of the example,
// whose generated files are this generator's own earlier output: the example
// commits them, and TestNothingGeneratedWasWrittenByHand in the example module
// holds the tree to exactly what generation writes. Regenerating it is the
// repeat-generation claim with the expectation fixed in advance, and it needs
// nothing from the evolved app, so it runs beside it.
var regenerated = sync.OnceValue(func() generation {
	app, err := sharedSandbox("regenerated")
	if err != nil {
		return generation{err: err}
	}
	return generate(app)
})

// requireEvolved returns the evolved app, failing the test if the fixture
// could not be set up or its first generation failed.
func requireEvolved(t *testing.T) *evolvedApp {
	t.Helper()
	e := evolved()
	if e.setupErr != nil {
		t.Fatalf("setting up the evolved example: %v", e.setupErr)
	}
	if e.first.err != nil {
		t.Fatalf("go generate ./... over the evolved example: %v\n%s", e.first.err, e.first.out)
	}
	return e
}

// start runs fn in the background now and returns a func that waits for it.
func start[T any](fn func() T) func() T {
	var result T
	done := make(chan struct{})
	go func() {
		defer close(done)
		result = fn()
	}()
	return func() T {
		<-done
		return result
	}
}

// generate runs `go generate ./...` in app and reads back the Go client.
func generate(app string) generation {
	g := generation{dir: app}
	g.out, g.err = runGoGenerate(app)
	if g.err != nil {
		return g
	}
	for _, f := range clientFiles {
		raw, err := os.ReadFile(filepath.Join(app, filepath.FromSlash(f)))
		if err != nil {
			g.err = err
			return g
		}
		g.clients = append(g.clients, raw)
	}
	return g
}

func applyEdits(app string, edits []evolvedEdit) error {
	for _, e := range edits {
		path := filepath.Join(app, filepath.FromSlash(e.file))
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if n := strings.Count(string(raw), e.old); n != 1 {
			return fmt.Errorf("fixture %s must contain exactly one %q, has %d; update the test", e.file, e.old, n)
		}
		if err := os.WriteFile(path, []byte(strings.Replace(string(raw), e.old, e.new, 1)), 0o644); err != nil {
			return err
		}
	}
	return nil
}
