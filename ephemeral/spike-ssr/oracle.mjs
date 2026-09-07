// SPIKE. The oracle: runs the very same bundle goja runs, in Node, and prints
// the rendered head/body per scenario as JSON. Anything the two engines
// disagree about is an engine difference, not a bundling difference.
//
// Usage: node oracle.mjs [dist/ssr.js] > oracle.json
import { readFileSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import vm from 'node:vm';

const HERE = dirname(fileURLToPath(import.meta.url));
const bundle_path = resolve(HERE, process.argv[2] ?? 'dist/ssr.js');
const fixtures = JSON.parse(readFileSync(join(HERE, 'fixtures.json'), 'utf8'));

const out = {};
for (const [name, req] of Object.entries(fixtures.requests)) {
	// A fresh context per scenario, the same way a fresh goja Runtime is fresh.
	const sandbox = {
		console,
		__skgo_remote(id, payload) {
			const key = id + '/' + payload;
			const answer = fixtures.remotes[key];
			if (!answer) throw new Error(`no fixture for remote key ${key}`);
			return JSON.stringify(answer);
		}
	};
	sandbox.globalThis = sandbox;
	const context = vm.createContext(sandbox);
	new vm.Script(readFileSync(bundle_path, 'utf8'), { filename: 'ssr.js' }).runInContext(context);
	// Both kit and svelte assign their AsyncLocalStorage from a `.then()` on a
	// dynamic import, so it is not in place until the microtask queue has been
	// drained once after the program is evaluated.
	await new Promise((r) => setImmediate(r));

	const result = sandbox.__skgo_render(JSON.stringify(req));
	// let the microtask queue drain
	for (let i = 0; i < 200 && !result.done; i++) await new Promise((r) => setImmediate(r));
	out[name] = {
		done: result.done,
		error: result.error,
		head: result.head,
		body: result.body,
		data: JSON.parse(JSON.stringify(result.data))
	};
}

process.stdout.write(JSON.stringify(out, null, '\t') + '\n');
