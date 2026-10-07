package adapter

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// Exercise the transform used by both build and dev on actual JavaScript
// syntax: comments and quoted text must not downlevel native private fields.
func TestEngineLoweringUsesSyntaxRatherThanComments(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	module, err := filepath.Abs("skgo-adapter/env.js")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../../example/web")
	if err != nil {
		t.Fatal(err)
	}
	const source = `
import { pathToFileURL } from 'node:url';
const { engineTransform } = await import(pathToFileURL(process.argv[1]));
const native = [
 '/** @param {Function} render_async\n */\nclass Renderer { #out = []; read() { return this.#out; } }',
 '// for await (const row of rows)\nclass Renderer { #out = []; }',
 'const message = "async function* and for await ("; class Renderer { #out = []; }',
 'const pattern = /async\\s*\\*/; class Renderer { #out = []; }',
 'const message = ` + "`async * and for await (`" + `; class Renderer { #out = []; }',
 '// async\nconst table = [' + Array(200_000).fill('0').join(',') + ']; class Renderer { #out = []; }'
];
for (const code of native) {
 const lowered = new Set();
 if (engineTransform(code, '/fixture.js', { lowered }) !== null || lowered.size !== 0) {
  throw new Error('non-code lowered private fields: ' + code);
 }
}
const unsupported = [
 'async function* rows() { yield 1; }',
 'const rows = async function* () { yield 1; };',
 'const rows = { async *values() { yield 1; } };',
 'async/*comment*/function* rows() { yield 1; }',
 'const rows = { async/*comment*/*values() { yield 1; } };',
 'async function rows() { for/*comment*/await (const row of source) consume(row); }',
 'async function rows() { for await/*comment*/(const row of source) consume(row); }',
 'async function rows() { for await (const row of source) consume(row); }',
 'const rows = ` + "`${async function* () { yield 1; }}`" + `;'
];
for (const code of unsupported) {
 const lowered = new Set();
 const result = engineTransform(code, '/fixture.js', { lowered });
 if (!result?.code || !lowered.has('/fixture.js')) throw new Error('unsupported syntax survived: ' + code);
}
`
	cmd := exec.Command(node, "--input-type=module", "-e", source, module)
	cmd.Dir = root
	cmd.Env = fixtureBuildEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("engine transform: %v\n%s", err, out)
	}
}
