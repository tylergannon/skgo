package adapter

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestTheAdapterEndpointCheckAcceptsQueryAndFallbackAndRefusesMismatch drives
// the endpoint half of the adapter directly. Kit reports a compiled endpoint's
// methods as its exports, with `fallback` spelled `'*'`
// (`core/postbuild/analyse.js`); `skgo.remotes.json` writes the same spelling,
// so the check compares the two lists literally. It has to let `QUERY` and `'*'`
// through — both are ordinary endpoint methods — and still refuse a set that
// does not match what kit compiled, in either direction.
//
// The check lives in `skgo-adapter/generated.js` so it runs with no SvelteKit
// build around it. A missing node is a failure, not a skip.
func TestTheAdapterEndpointCheckAcceptsQueryAndFallbackAndRefusesMismatch(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatalf("node is not on PATH, so the adapter's endpoint check cannot be exercised — which is not the same as passing: %v", err)
	}
	module, err := filepath.Abs(filepath.Join("skgo-adapter", "generated.js"))
	if err != nil {
		t.Fatal(err)
	}

	const snippet = `import { pathToFileURL } from 'node:url';
const { checkEndpoints } = await import(pathToFileURL(process.argv[1]).href);
const { built, declared } = JSON.parse(process.argv[2]);
checkEndpoints(built, declared);
`
	run := func(built, declared string) (string, error) {
		input := `{"built":` + built + `,"declared":` + declared + `}`
		cmd := exec.Command(node, "--input-type=module", "-e", snippet, module, input)
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	// Kit's own reporting for a route that answers GET, QUERY and a fallback.
	const built = `{"routes":[{"id":"/api/thing","api":{"methods":["GET","QUERY","*"]}}]}`
	if out, err := run(built, `{"/api/thing":["GET","QUERY","*"]}`); err != nil {
		t.Fatalf("the adapter refused QUERY and the fallback export: %v\n%s", err, out)
	}

	tests := []struct {
		name     string
		declared string
		want     string
	}{
		{
			name:     "a generated method kit did not compile",
			declared: `{"/api/thing":["GET","POST","QUERY","*"]}`,
			want:     "Go answers *, GET, POST, QUERY",
		},
		{
			name:     "a compiled method the manifest omits",
			declared: `{"/api/thing":["GET","*"]}`,
			want:     "Go answers *, GET",
		},
		{
			name:     "a route the manifest does not describe",
			declared: `{}`,
			want:     "compiled but not generated",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := run(built, tc.declared)
			if err == nil {
				t.Fatalf("the adapter accepted a mismatched export set: %s", tc.declared)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("the refusal does not name %q:\n%s", tc.want, out)
			}
		})
	}
}

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
		"prerender": {"root": "..", "package": "./internal/skgo/prerender"},
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
