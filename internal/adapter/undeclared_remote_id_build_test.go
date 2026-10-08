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
	// The compatible successful artifact/nil-callback build owns generation.
	// This refusal keeps its own mutable web root and real native Kit build.
	source, err := generatedMinimalInputsApp()
	if err != nil {
		t.Fatal(err)
	}
	fixture := cloneGeneratedInputsWeb(t, source)
	app := filepath.Join(fixture, "web")
	// Queries belong to a request-time page, as in the original refusal fixture.
	writeFixtureFile(t, filepath.Join(app, "src", "routes", "+page.ts"), "export const prerender = false;\n")
	writeFixtureFile(t, filepath.Join(app, "src", "routes", "+page.svelte"), `<script>
  import { manual } from '../lib/fixture.remote';
  const result = manual();
</script>
<h1>{#await result then value}{value}{/await}</h1>
`)
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
	wantIDs := map[string]bool{
		declaredID:     true,
		"3215r6/empty": true,
		"1vyw5d0/item": true,
		"1h38tvo/item": true,
	}
	if len(manifest.Remotes) != len(wantIDs) {
		t.Fatalf("Go registration manifest = %v; want the four fixture declarations", manifest.Remotes)
	}
	for _, id := range manifest.Remotes {
		if !wantIDs[id] {
			t.Fatalf("unexpected Go registration %q in %v", id, manifest.Remotes)
		}
		delete(wantIDs, id)
	}
	const manualID = "3215r6/manual"
	remoteModule := filepath.Join(app, "src", "lib", "fixture.remote.ts")
	generatedStub, err := os.ReadFile(remoteModule)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generatedStub), "import { prerender, query } from \"$app/server\"") ||
		!strings.Contains(string(generatedStub), "export const declared = query(") {
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
