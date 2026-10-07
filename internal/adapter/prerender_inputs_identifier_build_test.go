package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthoredSkgoRemoteInputsIdentifierBuildsInTypeScriptAndJavaScript(t *testing.T) {
	for _, language := range []string{"ts", "js"} {
		t.Run(language, func(t *testing.T) {
			fixture := prepareMinimalInputsApp(t)
			app := filepath.Join(fixture, "web")
			if language == "js" {
				if err := os.Remove(filepath.Join(app, "tsconfig.json")); err != nil {
					t.Fatalf("remove TypeScript config before JavaScript generation: %v", err)
				}
				writeFixtureFile(t, filepath.Join(app, "jsconfig.json"), `{
  "extends": "$app/tsconfig",
  "compilerOptions": { "allowJs": true, "checkJs": true, "strict": true }
}
`)
			}
			writeFixtureFile(t, filepath.Join(app, "src", "lib", "fixture.remote.go"), `package lib
import (
  "context"
  "fmt"
  "os"
  "github.com/tylergannon/skgo"
)
func note(value string) {
  path := os.Getenv("SKGO_IDENTIFIER_RECEIPT")
  if path == "" { return }
  file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
  if err != nil { panic(err) }
  defer file.Close()
  if _, err := fmt.Fprintln(file, value); err != nil { panic(err) }
}
func skgoRemoteInputs(_ context.Context, value string) (string, error) {
  note("body")
  return "body:" + value, nil
}
func fixtureInputs() ([]string, error) {
  note("producer")
  return []string{"atlas"}, nil
}
var _ = skgo.Prerender(skgoRemoteInputs, skgo.PrerenderOptions{Inputs: fixtureInputs})
`)
			writeFixtureFile(t, filepath.Join(app, "src", "routes", "+layout.ts"), `import '../lib/fixture.remote';
export const prerender = true;
`)
			writeFixtureFile(t, filepath.Join(app, "src", "routes", "+page.svelte"), `<script>
  import { skgoRemoteInputs } from '../lib/fixture.remote';
  const result = skgoRemoteInputs('atlas');
</script>
<h1>{#await result then value}{value}{/await}</h1>
`)
			generateMinimalInputsApp(t, fixture)

			extension := ".remote." + language
			stubPath := filepath.Join(app, "src", "lib", "fixture"+extension)
			stub, err := os.ReadFile(stubPath)
			if err != nil {
				t.Fatalf("generated %s remote stub: %v", language, err)
			}
			for _, want := range []string{
				"remoteInputs as $skgoRemoteInputs",
				"export const skgoRemoteInputs = prerender(",
				"$skgoRemoteInputs",
			} {
				if !strings.Contains(string(stub), want) {
					t.Fatalf("generated %s remote stub omitted %q:\n%s", language, want, stub)
				}
			}
			receipt := filepath.Join(fixture, "identifier-receipt")
			if _, err := os.Stat(receipt); !os.IsNotExist(err) {
				t.Fatalf("producer or remote body ran during generation: %v", err)
			}

			output, err := buildMinimalInputsApp(t, fixture, "SKGO_IDENTIFIER_RECEIPT="+receipt)
			if err != nil {
				t.Fatalf("minimal %s native build with authored skgoRemoteInputs identifier: %v\n%s", language, err, output)
			}
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
			const module = "src/lib/fixture.remote.go"
			var address string
			for _, remote := range manifest.Remotes {
				if strings.HasSuffix(remote, "/skgoRemoteInputs") {
					if address != "" {
						t.Fatalf("manifest repeats authored remote %s: %s", module, manifestBytes)
					}
					address = strings.TrimSuffix(remote, "/skgoRemoteInputs")
				}
			}
			if address == "" {
				t.Fatalf("manifest omitted authored module/name %s/skgoRemoteInputs: %s", module, manifestBytes)
			}
			artifact := filepath.Join(app, "build", "prerendered", "_app", "remote", filepath.FromSlash(address), "skgoRemoteInputs", "WyJhdGxhcyJd")
			contents, err := os.ReadFile(artifact)
			if err != nil {
				t.Fatalf("native literal input-key artifact: %v", err)
			}
			want := `{"type":"result","data":"[{\"_\":1},\"body:atlas\"]"}`
			// Kit's prerender wrapper records a dependency with only _. A
			// direct remote request additionally collects implicit data.p.
			// The page and declared-input queue may race to publish either.
			withImplicit := fmt.Sprintf(`{"type":"result","data":"[{\"_\":1,\"p\":2},\"body:atlas\",{\"%s/skgoRemoteInputs/WyJhdGxhcyJd\":3},{\"v\":1}]"}`, address)
			if string(contents) != want && string(contents) != withImplicit {
				t.Fatalf("native artifact for literal module/name/key = %s; want %s", contents, want)
			}
			entries, err := os.ReadDir(filepath.Dir(artifact))
			if err != nil || len(entries) != 1 || entries[0].Name() != "WyJhdGxhcyJd" {
				t.Fatalf("native artifact set for authored remote = %v, %v; want exactly the literal atlas key", entries, err)
			}
			calls, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}
			counts := map[string]int{}
			for _, call := range strings.Fields(string(calls)) {
				counts[call]++
			}
			if counts["producer"] != 1 || counts["body"] != 1 || len(counts) != 2 {
				t.Fatalf("build-time producer/body calls = %q; want one producer and one body", calls)
			}
		})
	}
}
