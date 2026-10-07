package gen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

const optionalLinkImport = "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a"

// The evolved example (evolution_test.go) changed Input.Enabled from
// Optional[bool] to Optional[string].
func testFormClientInputContractEvolution(t *testing.T) {
	e := requireEvolved(t)
	assertGeneratedContract(t, e, "input", `"enabled"?: string;`, `"enabled"?: boolean;`,
		`Enabled polytype.Optional[string]`, `Enabled polytype.Optional[bool]`)

	consumer := `package contractconsumer
import (
	"context"
	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo/example/internal/skgo/client"
	optional "` + optionalLinkImport + `"
)
func use(c client.Client) {
	_, _ = c.Submit(context.Background(), optional.Input{Name: "Ada", Enabled: polytype.Optional[%s]{Present: true, Value: %s}})
}
`
	assertConsumerCompiles(t, e.first.dir, "input", fmt.Sprintf(consumer, "string", `"yes"`))
}

// The evolved example (evolution_test.go) changed Result.Operations from int
// to string.
func testFormClientResultContractEvolution(t *testing.T) {
	e := requireEvolved(t)
	assertGeneratedContract(t, e, "result", `"operations": string;`, `"operations": number;`,
		`Operations string`, `Operations int`)
	codec := e.first.clients[0]
	if !regexp.MustCompile(`dvString\(raw\d+, at\+"/operations"\)`).Match(codec) ||
		regexp.MustCompile(`dvInteger\(raw\d+, at\+"/operations"`).Match(codec) {
		t.Fatal("generated Go client did not decode operations as the fixture's new string type")
	}

	consumer := `package contractconsumer
import (
	"context"
	"github.com/tylergannon/skgo/example/internal/skgo/client"
	optional "` + optionalLinkImport + `"
)
func use(c client.Client) {
	result, _ := c.Submit(context.Background(), optional.Input{Name: "Ada"})
	var _ %s = result.Operations
}
`
	assertConsumerCompiles(t, e.first.dir, "result", fmt.Sprintf(consumer, "string"))
}

func readGenerated(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertGeneratedContract(t *testing.T, e *evolvedApp, caseName, browserWant, browserOld, linkWant, linkOld string) {
	t.Helper()
	app := e.first.dir
	base := filepath.Join(app, "web", "src", "routes", "optional")
	types := readGenerated(t, filepath.Join(base, "types.ts"))
	if !bytes.Contains(types, []byte(browserWant)) || bytes.Contains(types, []byte(browserOld)) {
		t.Fatalf("browser %s type did not follow fixture contract:\n%s", caseName, types)
	}
	stub := readGenerated(t, filepath.Join(base, "optional.remote.ts"))
	if !bytes.Contains(stub, []byte(`export const submit = form('unchecked', (_arg: Input): Result => unimplemented());`)) {
		t.Fatalf("browser Form does not use the projected input and result types:\n%s", stub)
	}
	link := readGenerated(t, filepath.Join(app, "internal", "skgo", "links", "onzggl3sn52xizltf5xxa5djn5xgc3a", "optional.remote.go"))
	if !bytes.Contains(link, []byte(linkWant)) || bytes.Contains(link, []byte(linkOld)) {
		t.Fatalf("Go route binding did not follow fixture %s contract:\n%s", caseName, link)
	}
	server := readGenerated(t, filepath.Join(app, "internal", "skgo", "skgo_gen.go"))
	submit := bytes.SplitN(server, []byte("func remote_submit("), 2)
	if len(submit) != 2 {
		t.Fatal("server Form binding is missing")
	}
	submitHandler := bytes.SplitN(submit[1], []byte("\nfunc "), 2)[0]
	if !bytes.Contains(submitHandler, []byte("skgo.DecodeForm(call.Arg, &in)")) ||
		!bytes.Contains(submitHandler, []byte("Skgo_submit(event.Context(), event, in)")) ||
		!bytes.Contains(submitHandler, []byte("EncodeRoot")) {
		t.Fatalf("server Form binding does not decode, call, and encode the changed %s contract", caseName)
	}
	client := e.first.clients[0]
	if !bytes.Contains(client, []byte("func (c Client) Submit(")) || !bytes.Contains(client, []byte("return skgo.SubmitForm(")) {
		t.Fatalf("Go client lacks the %s Form submission binding", caseName)
	}
	if e.first.buildErr != nil {
		t.Fatalf("changed %s bindings do not compile: %v\n%s", caseName, e.first.buildErr, e.first.buildOut)
	}
}

// Consumers live outside internal/skgo, so the `go build ./...` the fixture
// runs over the bindings never sees one, and each test has its own directory
// so the two can build at once.
func consumerDir(app, name string) string {
	return filepath.Join(app, "contractconsumer", name)
}

func assertConsumerCompiles(t *testing.T, app, name, source string) {
	t.Helper()
	writeConsumer(t, app, name, source)
	if out, err := runGoBuild(consumerDir(app, name)); err != nil {
		t.Fatalf("updated consumer did not compile: %v\n%s", err, out)
	}
}

func writeConsumer(t *testing.T, app, name, source string) {
	t.Helper()
	dir := consumerDir(app, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "consumer.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The evolved example (evolution_test.go) added a second supported Form,
// submitAgain, and a field to the Form result, and its first generation
// started with the Go client deleted.
func testFormClientGenerationRecoversAndTracksContract(t *testing.T) {
	e := requireEvolved(t)
	first := e.first.clients
	if !bytes.Contains(first[0], []byte("SubmitAgain")) {
		t.Fatal("second supported Form has no generated client")
	}
	if bytes.Contains(first[0], []byte("SendMessage")) {
		t.Fatal("file-upload Form unexpectedly gained a scalar client")
	}

	root, err := repoRoot()
	if err != nil {
		t.Fatal(err)
	}
	// Full-example regeneration is already asserted byte for byte by
	// example.TestNothingGeneratedWasWrittenByHand. Here recovery and changed
	// contracts use only the fixture's own optional/stream/contact routes.
	stale := e.stale()
	if stale.err != nil {
		t.Fatalf("replace stale clients: %v\n%s", stale.err, stale.out)
	}
	for i, file := range clientFiles {
		if !bytes.Equal(stale.clients[i], first[i]) {
			t.Fatalf("%s was not restored", file)
		}
	}

	// The evolved Result gained Extra, so its decoder is not the one the
	// example commits, which was generated before that field existed.
	before := readGenerated(t, filepath.Join(root, "example", filepath.FromSlash(clientFiles[0])))
	if bytes.Contains(before, []byte("extra")) {
		t.Fatal("the example's committed client decoder already mentions extra; pick another field name")
	}
	if codec := first[0]; bytes.Equal(codec, before) || !bytes.Contains(codec, []byte("extra")) {
		t.Fatal("changed Form result was not projected into client decoder")
	}
	if e.first.buildErr != nil {
		t.Fatalf("changed client does not compile: %v\n%s", e.first.buildErr, e.first.buildOut)
	}
}

func TestEmptyAppRemovesOldFormClient(t *testing.T) {
	t.Parallel()
	web := t.TempDir()
	if err := os.MkdirAll(filepath.Join(web, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "go.mod"), []byte("module emptyapp\n\ngo 1.27\n"), 0644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(web, "generated")
	clientDir := filepath.Join(out, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_gen.go"} {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte(goHeader+"stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(fixtureConfig(Config{Web: web, Out: out})); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_gen.go"} {
		if _, err := os.Stat(filepath.Join(clientDir, name)); !os.IsNotExist(err) {
			t.Fatalf("old client %s survived empty generation: %v", name, err)
		}
	}
}
