package adapter

import (
	"encoding/json"
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
import { readFileSync } from 'node:fs';
const { built, declarations } = JSON.parse(readFileSync(0, 'utf8'));
console.log(JSON.stringify(declarations.map(declared => {
 try { checkEndpoints(built, declared); return ''; }
 catch (error) { return String(error.message ?? error); }
})));
`

	// Kit's own reporting for a route that answers GET, QUERY and a fallback.
	const built = `{"routes":[{"id":"/api/thing","api":{"methods":["GET","QUERY","*"]}}]}`

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
	declarations := []string{`{"/api/thing":["GET","QUERY","*"]}`}
	for _, tc := range tests {
		declarations = append(declarations, tc.declared)
	}
	cmd := exec.Command(node, "--input-type=module", "-e", snippet, module)
	cmd.Stdin = strings.NewReader(`{"built":` + built + `,"declarations":[` + strings.Join(declarations, ",") + `]}`)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adapter endpoint checks did not execute: %v\n%s", err, output)
	}
	var results []*string
	if err := json.Unmarshal(output, &results); err != nil || len(results) != len(declarations) {
		t.Fatalf("missing endpoint check results: %v\n%s", err, output)
	}
	for _, result := range results {
		if result == nil {
			t.Fatal("missing endpoint check result")
		}
	}
	if *results[0] != "" {
		t.Fatalf("the adapter refused QUERY and the fallback export: %s", *results[0])
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := *results[i+1]
			if out == "" {
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
const inputs = JSON.parse(readFileSync(0, 'utf-8'));
console.log(JSON.stringify(inputs.map(input => {
 try { validateGenerated(input); return ''; }
 catch (error) { return String(error.message ?? error); }
})));
`

	good := `{
		"remotes": ["2b61k/status"],
		"loads": ["src/routes/a/+page.server.js", "src/routes/a/+layout.server.ts", "src/routes/b/+layout.server.js"],
		"prerender": {"root": "..", "package": "./internal/skgo/prerender"},
		"actions": ["src/routes/a/+page.server.js", "src/routes/b/+page.server.ts"],
		"endpoints": {"/api/thing": ["GET", "POST"]}
	}`

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
	inputs := []string{good}
	for _, tc := range tests {
		inputs = append(inputs, tc.input)
	}
	cmd := exec.Command(node, "--input-type=module", "-e", snippet, module)
	cmd.Stdin = strings.NewReader("[" + strings.Join(inputs, ",") + "]")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("adapter module checks did not execute: %v\n%s", err, output)
	}
	var results []*string
	if err := json.Unmarshal(output, &results); err != nil || len(results) != len(inputs) {
		t.Fatalf("missing module check results: %v\n%s", err, output)
	}
	for _, result := range results {
		if result == nil {
			t.Fatal("missing module check result")
		}
	}
	if *results[0] != "" {
		t.Fatalf("the adapter refused server modules from both languages: %s", *results[0])
	}
	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := *results[i+1]
			if out == "" {
				t.Fatalf("the adapter accepted a bad module path:\n%s", tc.input)
			}
			if !strings.Contains(out, tc.want) {
				t.Errorf("the refusal does not name the problem %q:\n%s", tc.want, out)
			}
		})
	}
}

func TestPrerenderModulesPreserveNativeDeclarationsAndRejectMissingMetadata(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs(filepath.Join("skgo-adapter", "prerender-modules.js"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	const snippet = `import assert from 'node:assert/strict';
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { pathToFileURL } from 'node:url';
const { prerenderModules } = await import(pathToFileURL(process.argv[1]).href);
const root = process.argv[2];
const module = 'src/data.remote.ts';
mkdirSync(join(root, 'src'));
const source = "// Code generated by skgo. DO NOT EDIT.\nimport { query, command, prerender } from '$app/server';\nconst unimplemented = (): never => {throw new Error('skgo: implemented in Go');};\nexport const live = query.live('unchecked', (_arg: string): AsyncIterable<string> => unimplemented());\nexport const change = command((): string => unimplemented());\nexport const empty = prerender(async (): Promise<string> => unimplemented());\nexport const names = prerender('unchecked', async (_arg: string): Promise<string> => unimplemented(),\n);\n";
writeFileSync(join(root, module), source);
const registry = { remotes: [], loads: [], actions: [], endpoints: {}, build: {[module]: {remotes: [{name:'empty',argument:false,inputs:false}, {name:'names',argument:true,inputs:true}]}} };
const write = () => writeFileSync(join(root, 'skgo.remotes.json'), JSON.stringify(registry));
const plugin = prerenderModules();
assert.equal(plugin.apply, 'build');
for (const name of ['client', 'goja', 'serviceWorker']) assert.equal(plugin.applyToEnvironment({name}), false);
assert.equal(plugin.applyToEnvironment({name:'ssr'}), true);
plugin.configResolved({root});
write(); plugin.buildStart();
const result = plugin.load(join(root, module));
assert.ok(result.includes("export const live = query.live('unchecked', (_arg: string): AsyncIterable<string> => unimplemented());"));
assert.ok(result.includes("export const change = command((): string => unimplemented());"));
assert.ok(result.includes('"empty", undefined, event)'));
assert.ok(result.includes('"names", _arg, event)'));
assert.ok(result.includes('inputs: () => $skgoInputs("src/data.remote.ts", "names")'));
assert.equal(readFileSync(join(root, module), 'utf8'), source);
registry.build[module].remotes[1].argument = false;
write(); plugin.buildStart();
assert.throws(() => plugin.load(join(root, module)), /build declarations disagree/);
delete registry.build[module];
write(); plugin.buildStart();
assert.throws(() => plugin.load(join(root, module)), /build declarations disagree/);
delete registry.build;
write(); assert.throws(() => plugin.buildStart(), /missing build declarations/);
const hook = 'src/hooks.server.ts';
registry.build = {[hook]: {hook:true}};
const policy = "// Code generated by skgo. DO NOT EDIT.\nexport {};\nexport const handleError = () => ({message:'native policy'});\n";
writeFileSync(join(root, hook), policy);
write(); plugin.buildStart();
const hooks = plugin.load(join(root, hook));
assert.ok(hooks.includes(policy));
assert.ok(hooks.includes('export const handle ='));
assert.ok(hooks.includes('export const handleFetch ='));
writeFileSync(join(root, hook), policy + 'export const handle = () => {};\n');
assert.throws(() => plugin.load(join(root, hook)), /build declarations disagree/);
`
	cmd := exec.Command(node, "--input-type=module", "-e", snippet, module, root)
	cmd.Dir = filepath.Join("..", "..", "example", "web")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build module declarations: %v\n%s", err, output)
	}
}
