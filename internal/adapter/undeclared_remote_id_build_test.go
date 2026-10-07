package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeBuildRejectsClientCalledRemoteMissingFromGoRegistration(t *testing.T) {
	t.Parallel()
	fixture := prepareMinimalInputsApp(t)
	app := filepath.Join(fixture, "web")
	writeFixtureFile(t, filepath.Join(app, "src", "lib", "fixture.remote.go"), `package lib
import (
  "context"
  "github.com/tylergannon/skgo"
)
func declared(context.Context) (string, error) { return "declared", nil }
var _ = skgo.Query(declared)
`)
	writeFixtureFile(t, filepath.Join(app, "src", "routes", "+page.svelte"), `<script>
  import { manual } from '../lib/fixture.remote';
  const result = manual();
</script>
<h1>{#await result then value}{value}{/await}</h1>
`)
	generateMinimalInputsApp(t, fixture)

	manifestBytes, err := os.ReadFile(filepath.Join(app, "skgo.remotes.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	const declaredID = "3215r6/declared"
	if len(manifest.Remotes) != 1 || manifest.Remotes[0] != declaredID {
		t.Fatalf("Go registration manifest = %v; want exactly %q", manifest.Remotes, declaredID)
	}
	const manualID = "3215r6/manual"
	remoteModule := filepath.Join(app, "src", "lib", "fixture.remote.ts")
	generatedStub, err := os.ReadFile(remoteModule)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generatedStub), "import { query } from \"$app/server\"") {
		t.Fatalf("generated Go remote stub does not provide Kit query import for same-module control:\n%s", generatedStub)
	}
	generatedStub = append(generatedStub, []byte("\nexport const manual = query(() => \"manual\");\n")...)
	if err := os.WriteFile(remoteModule, generatedStub, 0o644); err != nil {
		t.Fatal(err)
	}

	output, err := buildMinimalInputsApp(t, fixture)
	if err == nil {
		t.Fatalf("VP build accepted client-called undeclared remote id %s:\n%s", manualID, output)
	}
	for _, want := range []string{
		"the built frontend calls remote functions that `skgo generate` did not write.",
		manualID,
		"Go answers only the generated ids, so every one of these would 404 in the browser.",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("build failure for undeclared id %s omitted %q:\n%s", manualID, want, output)
		}
	}
}
