package adapter

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The adapter validates the module paths in `skgo.remotes.json` before it
// copies them into the build. A TypeScript app writes `.server.ts` and a
// JavaScript app writes `.server.js`, and both are first-class: the validation
// cannot know which language wrote the manifest, so it accepts either and
// refuses everything else.
//
// The validation lives in `skgo-adapter/generated.js`, apart from the entry's
// kit imports, so this Go test can run it with no SvelteKit build around it.
// Node is already required by this package's tests (`pnpm pack`); a missing
// node is a failure, not a skip, because a check that cannot run has not
// passed.
func TestTheAdapterAcceptsTypeScriptAndJavaScriptServerModules(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is not on PATH, so the adapter's validation cannot be exercised — which is not the same as passing: %v", err)
	}
	module, err := filepath.Abs(filepath.Join("skgo-adapter", "generated.js"))
	if err != nil {
		t.Fatal(err)
	}

	const snippet = `import { readFileSync } from 'node:fs';
import { pathToFileURL } from 'node:url';
const { validateGenerated } = await import(pathToFileURL(process.argv[1]).href);
validateGenerated(JSON.parse(readFileSync(0, 'utf-8')));
`
	run := func(input string) (string, error) {
		cmd := exec.Command(node, "--input-type=module", "-e", snippet, module)
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	good := `{
		"remotes": ["2b61k/status"],
		"loads": ["src/routes/a/+page.server.js", "src/routes/a/+layout.server.ts", "src/routes/b/+layout.server.js"],
		"actions": ["src/routes/a/+page.server.js", "src/routes/b/+page.server.ts"],
		"endpoints": {"/api/thing": ["GET", "POST"]}
	}`
	if out, err := run(good); err != nil {
		t.Fatalf("the adapter refused server modules from both languages: %v\n%s", err, out)
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "a load without the + prefix",
			input: `{"remotes":[],"loads":["src/routes/a/page.server.js"],"actions":[],"endpoints":{}}`,
			want:  "not a +page.server.(ts|js) or +layout.server.(ts|js) path",
		},
		{
			name:  "a load that is not a server module",
			input: `{"remotes":[],"loads":["src/routes/a/+page.js"],"actions":[],"endpoints":{}}`,
			want:  "not a +page.server.(ts|js) or +layout.server.(ts|js) path",
		},
		{
			name:  "an action in a layout",
			input: `{"remotes":[],"loads":[],"actions":["src/routes/a/+layout.server.js"],"endpoints":{}}`,
			want:  "not a +page.server.(ts|js) action path",
		},
		{
			name:  "an action in a server endpoint",
			input: `{"remotes":[],"loads":[],"actions":["src/routes/a/+server.js"],"endpoints":{}}`,
			want:  "not a +page.server.(ts|js) action path",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(tc.input)
			if err == nil {
				t.Fatalf("the adapter accepted a bad module path:\n%s", tc.input)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("the refusal does not name the problem %q:\n%s", tc.want, out)
			}
		})
	}
}
