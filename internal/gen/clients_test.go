package gen

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const optionalLinkImport = "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a"

func TestFormClientInputContractEvolution(t *testing.T) {
	app := sandboxExample(t)
	source := filepath.Join(app, "web", "src", "routes", "optional", "optional.remote.go")
	replaceFixture(t, source,
		"Enabled polytype.Optional[bool]   `json:\"enabled,omitzero\"`",
		"Enabled polytype.Optional[string] `json:\"enabled,omitzero\"`")
	replaceFixture(t, source, "present: %t", "present: %q")
	assertGeneratedContract(t, app, "input", `"enabled"?: string;`, `"enabled"?: boolean;`,
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
	assertConsumerCompiles(t, app, fmt.Sprintf(consumer, "string", `"yes"`))
	assertStaleConsumerRejected(t, app, fmt.Sprintf(consumer, "bool", "true"), "Optional[bool]", "Optional[string]")
}

func TestFormClientResultContractEvolution(t *testing.T) {
	app := sandboxExample(t)
	source := filepath.Join(app, "web", "src", "routes", "optional", "optional.remote.go")
	replaceFixture(t, source, "Operations int    `json:\"operations\"`", "Operations string `json:\"operations\"`")
	replaceFixture(t, source, "result.Operations = operations.byName[in.Name]", "result.Operations = fmt.Sprint(operations.byName[in.Name])")
	assertGeneratedContract(t, app, "result", `"operations": string;`, `"operations": number;`,
		`Operations string`, `Operations int`)
	codec := readGenerated(t, filepath.Join(app, "internal", "skgo", "client", "skgo_client_devalue_gen.go"))
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
	assertConsumerCompiles(t, app, fmt.Sprintf(consumer, "string"))
	assertStaleConsumerRejected(t, app, fmt.Sprintf(consumer, "int"), "result.Operations", "string", "int")
}

func replaceFixture(t *testing.T, path, old, replacement string) {
	t.Helper()
	raw := readGenerated(t, path)
	if strings.Count(string(raw), old) != 1 {
		t.Fatalf("fixture %s must contain exactly one %q", path, old)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(raw), old, replacement, 1)), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readGenerated(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertGeneratedContract(t *testing.T, app, caseName, browserWant, browserOld, linkWant, linkOld string) {
	t.Helper()
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("generate changed %s contract: %v\n%s", caseName, err, out)
	}
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
	server := readGenerated(t, filepath.Join(app, "internal", "skgo", "skgo_bindings_gen.go"))
	submit := bytes.SplitN(server, []byte("func remote_submit("), 2)
	if len(submit) != 2 {
		t.Fatal("server Form binding is missing")
	}
	submitHandler := bytes.SplitN(submit[1], []byte("\nfunc "), 2)[0]
	if !bytes.Contains(submitHandler, []byte("skgo.DecodeForm(call.Arg, &in)")) ||
		!bytes.Contains(submitHandler, []byte("Skgo_submit(ctx, in)")) ||
		!bytes.Contains(submitHandler, []byte("EncodeRoot")) {
		t.Fatalf("server Form binding does not decode, call, and encode the changed %s contract", caseName)
	}
	client := readGenerated(t, filepath.Join(app, "internal", "skgo", "client", "skgo_client_gen.go"))
	if !bytes.Contains(client, []byte("func (c Client) Submit(")) || !bytes.Contains(client, []byte("return skgo.SubmitForm(")) {
		t.Fatalf("Go client lacks the %s Form submission binding", caseName)
	}
	if out, err := runGoBuild(filepath.Join(app, "internal", "skgo")); err != nil {
		t.Fatalf("changed %s bindings do not compile: %v\n%s", caseName, err, out)
	}
}

func assertConsumerCompiles(t *testing.T, app, source string) {
	t.Helper()
	writeConsumer(t, app, source)
	if out, err := runGoBuild(filepath.Join(app, "internal", "skgo", "contractconsumer")); err != nil {
		t.Fatalf("updated consumer did not compile: %v\n%s", err, out)
	}
}

func assertStaleConsumerRejected(t *testing.T, app, source string, diagnostics ...string) {
	t.Helper()
	writeConsumer(t, app, source)
	out, err := runGoBuild(filepath.Join(app, "internal", "skgo", "contractconsumer"))
	if err == nil {
		t.Fatalf("stale consumer unexpectedly compiled:\n%s", out)
	}
	for _, diagnostic := range diagnostics {
		if !strings.Contains(out, diagnostic) {
			t.Fatalf("stale consumer failed for a reason other than the changed contract; missing %q:\n%s", diagnostic, out)
		}
	}
}

func writeConsumer(t *testing.T, app, source string) {
	t.Helper()
	dir := filepath.Join(app, "internal", "skgo", "contractconsumer")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "consumer.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFormClientGenerationRecoversAndTracksContract(t *testing.T) {
	app := sandboxExample(t)
	source := filepath.Join(app, "web", "src", "routes", "optional", "optional.remote.go")
	raw, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	second := string(raw) + "\nfunc submitAgain(ctx context.Context, in Input) (Result, error) { return submit(ctx, in) }\nvar _ = skgo.Form(submitAgain)\n"
	if err := os.WriteFile(source, []byte(second), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(app, "internal", "skgo", "client")
	files := []string{filepath.Join(dir, "skgo_client_gen.go"), filepath.Join(dir, "skgo_client_devalue_gen.go")}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("generate missing clients: %v\n%s", err, out)
	}
	first := make([][]byte, len(files))
	for i, file := range files {
		var err error
		first[i], err = os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
	}
	if !bytes.Contains(first[0], []byte("SubmitAgain")) {
		t.Fatal("second supported Form has no generated client")
	}
	if bytes.Contains(first[0], []byte("SendMessage")) {
		t.Fatal("file-upload Form unexpectedly gained a scalar client")
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("repeat generation: %v\n%s", err, out)
	}
	for i, file := range files {
		again, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(again, first[i]) {
			t.Fatalf("%s changed on repeat generation", file)
		}
		if err := os.WriteFile(file, []byte("stale generated client\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("replace stale clients: %v\n%s", err, out)
	}
	for i, file := range files {
		restored, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(restored, first[i]) {
			t.Fatalf("%s was not restored", file)
		}
	}

	changed := strings.Replace(second, "type Result struct {", "type Result struct {\n\tExtra string `json:\"extra\"`", 1)
	if changed == second {
		t.Fatal("fixture result contract changed; update test")
	}
	if err := os.WriteFile(source, []byte(changed), 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := runGoGenerate(app); err != nil {
		t.Fatalf("generate changed result contract: %v\n%s", err, out)
	}
	codec, err := os.ReadFile(files[1])
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(codec, first[1]) || !bytes.Contains(codec, []byte("extra")) {
		t.Fatal("changed Form result was not projected into client decoder")
	}
	if out, err := runGoBuild(filepath.Join(app, "internal", "skgo")); err != nil {
		t.Fatalf("changed client does not compile: %v\n%s", err, out)
	}
}

func TestEmptyAppRemovesOldFormClient(t *testing.T) {
	web := t.TempDir()
	if err := os.MkdirAll(filepath.Join(web, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(web, "generated")
	clientDir := filepath.Join(out, "client")
	if err := os.MkdirAll(clientDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_client_gen.go", "skgo_client_devalue_gen.go"} {
		if err := os.WriteFile(filepath.Join(clientDir, name), []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := Run(Config{Web: web, Out: out}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"skgo_client_gen.go", "skgo_client_devalue_gen.go"} {
		if _, err := os.Stat(filepath.Join(clientDir, name)); !os.IsNotExist(err) {
			t.Fatalf("old client %s survived empty generation: %v", name, err)
		}
	}
}
