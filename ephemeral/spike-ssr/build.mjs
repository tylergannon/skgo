// SPIKE (not production). Builds an SSR bundle from the example app that a
// JS engine embedded in the Go process can run. Build-time only; the artefact
// it emits is what Go loads.
//
// Usage: node build.mjs [--als=stub|webcontainer|none] [--out=dist/ssr.js]
import { readFileSync, readdirSync, writeFileSync, mkdirSync, realpathSync } from 'node:fs';
import { dirname, join, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import * as esbuild from 'esbuild';

const HERE = dirname(fileURLToPath(import.meta.url));
const WEB = resolve(HERE, '../../example/web');
const OUT_SERVER = join(WEB, '.svelte-kit/output/server');
// realpath: pnpm links the package, and esbuild would otherwise look for
// kit's own dependencies (devalue, cookie, ...) next to the symlink.
const KIT = join(realpathSync(join(WEB, 'node_modules/@sveltejs/kit')), 'src');
const SVELTE = join(WEB, 'node_modules/svelte');

const args = Object.fromEntries(
	process.argv.slice(2).map((a) => {
		const [k, v] = a.replace(/^--/, '').split('=');
		return [k, v ?? true];
	})
);
const ALS = args.als ?? 'stub';
const OUTFILE = join(HERE, args.out ?? 'dist/ssr.js');
const TARGET = args.target ?? 'es2022';

const { compile, compileModule } = await import(
	pathToFileURL(join(SVELTE, 'src/compiler/index.js')).href
);

// ---------------------------------------------------------------------------
// Read kit's own build output for the node table and the route table. These are
// facts about the app kit just built, not guesses.
// ---------------------------------------------------------------------------

/**
 * The component source behind a compiled entry, taken from the sourcemap kit's
 * own build emitted. Reversing kit's `[id]` -> `_id_` filename encoding by hand
 * would be a guess; `sources` is the record of what was compiled.
 */
function entry_to_source(entry) {
	const map_path = join(OUT_SERVER, entry.replace(/^\.\.\//, '') + '.map');
	const sources = JSON.parse(readFileSync(map_path, 'utf8')).sources;
	const own = sources[sources.length - 1];
	return relative(WEB, resolve(dirname(map_path), own));
}

const node_files = readdirSync(join(OUT_SERVER, 'nodes'))
	.filter((f) => /^\d+\.js$/.test(f))
	.sort((a, b) => parseInt(a) - parseInt(b));

const nodes = node_files.map((f) => {
	const text = readFileSync(join(OUT_SERVER, 'nodes', f), 'utf8');
	const m = /import\('(\.\.\/entries\/pages\/[^']+)'\)/.exec(text);
	const index = parseInt(/export const index = (\d+)/.exec(text)[1]);
	const server_id = /export const server_id = "([^"]+)"/.exec(text)?.[1] ?? null;
	return { index, component: m ? entry_to_source(m[1]) : null, server_id };
});
nodes.sort((a, b) => a.index - b.index);
for (let i = 0; i < nodes.length; i++) {
	if (nodes[i].index !== i) throw new Error(`node table is not dense at ${i}`);
}

// manifest-full.js, never manifest.js: the latter is prerender-trimmed and
// renumbers the node table, so its `leaf`/`layouts` indices do not line up
// with `nodes/N.js` on disk.
const manifest_src = readFileSync(join(OUT_SERVER, 'manifest-full.js'), 'utf8');
const routes = [];
for (const m of manifest_src.matchAll(
	/id: "([^"]*)",\s*pattern: (\/[^\n]*\/),\s*params: (\[[^\]]*\]),\s*page: (\{[^}]*\}|null)/g
)) {
	const page = m[4] === 'null' ? null : eval('(' + m[4].replace(/,\s*\]/g, ',null]') + ')');
	routes.push({ id: m[1], page });
}

mkdirSync(dirname(OUTFILE), { recursive: true });
writeFileSync(
	join(dirname(OUTFILE), 'app.json'),
	JSON.stringify({ nodes, routes }, null, '\t') + '\n'
);

// ---------------------------------------------------------------------------
// esbuild
// ---------------------------------------------------------------------------

const NODES_NS = 'skgo-nodes';

const alias = {
	// kit's user-facing `$app/*` specifiers, which only vite normally resolves.
	'$app/state': join(KIT, 'runtime/app/state/index.js'),
	'$app/navigation': join(KIT, 'runtime/app/navigation/index.js'),
	'$app/env': join(KIT, 'runtime/app/env/index.js'),
	'$app/environment': join(KIT, 'runtime/app/environment/index.js'),
	'$app/forms': join(KIT, 'runtime/app/forms/index.js'),
	'$app/paths': join(KIT, 'runtime/app/paths/index.js'),
	// `$app/server` goes to the shim, which re-exports kit's real remote
	// wrappers but substitutes the Go host call for the user function body.
	'$app/server': join(HERE, 'src/app-server.js'),
	// kit internals the shim and the entry need. Kit's package `exports` map
	// does not expose these paths, so they are aliased by absolute path rather
	// than deep-imported.
	'skgo:kit/remote': join(KIT, 'runtime/app/server/remote/index.js'),
	'skgo:kit/shared': join(KIT, 'runtime/shared.js'),
	'skgo:kit/props': join(KIT, 'runtime/props.svelte.js'),
	'skgo:kit/root': join(KIT, 'runtime/components/root.svelte'),
	'skgo:kit/collect': join(KIT, 'runtime/server/remote-functions.js')
};

const plugin = {
	name: 'skgo-spike',
	setup(b) {
		// --- .svelte -> server-mode JS, with TS stripped from <script lang="ts">
		b.onLoad({ filter: /\.svelte$/ }, async (a) => {
			let source = readFileSync(a.path, 'utf8');
			const script = /<script([^>]*)>([\s\S]*?)<\/script>/g;
			const edits = [];
			for (const m of source.matchAll(script)) {
				if (!/lang=["']ts["']/.test(m[1])) continue;
				const out = await esbuild.transform(m[2], {
					loader: 'ts',
					// keeps imports that are only referenced from the template
					tsconfigRaw: { compilerOptions: { verbatimModuleSyntax: true } }
				});
				edits.push([m.index + m[0].indexOf(m[2]), m[2].length, out.code]);
			}
			for (const [at, len, text] of edits.reverse()) {
				source = source.slice(0, at) + text + source.slice(at + len);
			}
			const { js, warnings } = compile(source, {
				filename: a.path,
				generate: 'server',
				experimental: { async: true }
			});
			for (const w of warnings) {
				if (!/unused|a11y/.test(w.code)) console.log(`  svelte warn ${w.code}: ${w.message}`);
			}
			return { contents: js.code, loader: 'js', resolveDir: dirname(a.path) };
		});

		// --- `.svelte.js` modules carry runes (kit's `props.svelte.js` uses
		//     `$state.raw`) and must go through the compiler too.
		b.onLoad({ filter: /\.svelte\.js$/ }, (a) => {
			const { js } = compileModule(readFileSync(a.path, 'utf8'), {
				filename: a.path,
				generate: 'server'
			});
			return { contents: js.code, loader: 'js', resolveDir: dirname(a.path) };
		});

		// --- `.remote.ts` gets exactly the epilogue kit's vite plugin appends
		//     (packages/kit/src/exports/vite/index.js, the `transform` handler):
		//     init_remote_functions + `fn.__.id = hash(file) + '/' + name`.
		b.onLoad({ filter: /\.remote\.ts$/ }, async (a) => {
			const file = relative(WEB, a.path).replaceAll('\\', '/');
			const out = await esbuild.transform(readFileSync(a.path, 'utf8'), {
				loader: 'ts',
				tsconfigRaw: { compilerOptions: { verbatimModuleSyntax: true } }
			});
			const epilogue = `
import * as $$_self_$$ from './${a.path.split('/').pop()}';
import { init_remote_functions as $$_init_$$ } from '@sveltejs/kit/internal/server';
$$_init_$$($$_self_$$, ${JSON.stringify(file)}, ${JSON.stringify(djb2(file))});
for (const [name, fn] of Object.entries($$_self_$$)) {
	fn.__.id = ${JSON.stringify(djb2(file))} + '/' + name;
	fn.__.name = name;
}
`;
			return { contents: out.code + epilogue, loader: 'js', resolveDir: dirname(a.path) };
		});

		// --- aliases
		for (const [from, to] of Object.entries(alias)) {
			b.onResolve({ filter: new RegExp('^' + from.replace(/[$/]/g, '\\$&') + '$') }, () => ({
				path: to
			}));
		}

		// esm-env: DEV/BROWSER are conditions vite sets; pin them for production SSR.
		b.onResolve({ filter: /^esm-env$/ }, () => ({ path: 'esm-env', namespace: 'skgo-stub' }));
		// `#app/paths` (pulled in by kit's `$app/paths` and `paths/server.js`) wants
		// `<sveltekit:generated>/server.js`, a vite virtual module.
		b.onResolve({ filter: /^<sveltekit:generated>/ }, () => ({
			path: 'generated',
			namespace: 'skgo-stub'
		}));
		if (ALS === 'stub') {
			b.onResolve({ filter: /^node:async_hooks$/ }, () => ({
				path: 'async_hooks',
				namespace: 'skgo-stub'
			}));
		} else {
			b.onResolve({ filter: /^node:async_hooks$/ }, () => ({
				path: 'missing',
				namespace: 'skgo-stub'
			}));
		}

		b.onLoad({ filter: /.*/, namespace: 'skgo-stub' }, (a) => {
			if (a.path === 'esm-env')
				return { contents: 'export const DEV = false; export const BROWSER = false;' };
			if (a.path === 'generated')
				return { contents: 'export const get_hooks = () => ({});' };
			if (a.path === 'missing')
				return { contents: 'throw new Error("node:async_hooks unavailable");' };
			// The AsyncLocalStorage shim under test.
			//
			// Two things it must get right, both learned the hard way:
			//   1. The slot is PER INSTANCE. svelte's render context
			//      (svelte/src/internal/server/render-context.js:70) and kit's
			//      request store (kit/src/exports/internal/server/event.js:11)
			//      each construct their own AsyncLocalStorage. A single module-level
			//      slot lets svelte's `als.run(render_context, ...)` clobber kit's
			//      store, and `get_request_store()` then returns svelte's context —
			//      `{ event: undefined, state: undefined }`.
			//   2. `run` does NOT restore the previous store. `als.run(store, fn)`
			//      where `fn` is async returns at the first `await`; everything after
			//      that point still needs the store. Restoring in a `finally` empties
			//      it before the render is half done.
			// Both concessions are only sound while one Runtime renders one page at
			// a time.
			return {
				contents: `
export class AsyncLocalStorage {
	#store = undefined;
	getStore() { return this.#store; }
	run(s, fn) { this.#store = s; return fn(); }
	enterWith(s) { this.#store = s; }
	exit(fn) { const p = this.#store; this.#store = undefined; try { return fn(); } finally { this.#store = p; } }
}
export default { AsyncLocalStorage };
`
			};
		});

		// --- the generated node table: static imports of every page/layout/error
		//     component kit's build produced, keyed by kit's own node index.
		b.onResolve({ filter: /^skgo:nodes$/ }, () => ({ path: 'nodes', namespace: NODES_NS }));
		b.onLoad({ filter: /.*/, namespace: NODES_NS }, () => {
			const imports = [];
			const table = [];
			nodes.forEach((n, i) => {
				if (!n.component) {
					table.push('undefined');
					return;
				}
				imports.push(`import N${i} from ${JSON.stringify(join(WEB, n.component))};`);
				table.push(`N${i}`);
			});
			return {
				contents: `${imports.join('\n')}\nexport const components = [${table.join(', ')}];\n`,
				loader: 'js',
				resolveDir: WEB
			};
		});
	}
};

const result = await esbuild.build({
	entryPoints: [join(HERE, 'src/entry.js')],
	bundle: true,
	format: 'iife',
	platform: 'neutral',
	// es2022 keeps native private class fields. Downlevelling them makes
	// svelte's server Renderer unusable in goja.
	target: TARGET,
	// goja cannot parse `import(...)` or `import.meta`; esbuild rewrites both
	// when they are declared unsupported.
	supported: {
		// goja cannot parse `import(...)` or `import.meta`.
		'dynamic-import': false,
		'import-meta': false,
		// goja's parser rejects `for await` and async generators, which kit's
		// live-query and streaming code paths use.
		'for-await': false,
		'async-generator': false
	},
	mainFields: ['module', 'main'],
	// no 'browser' condition: kit's `#app/*` imports map must resolve to the
	// server variants.
	conditions: [],
	nodePaths: [join(WEB, 'node_modules')],
	absWorkingDir: WEB,
	define: {
		__SVELTEKIT_DEV__: 'false',
		__SVELTEKIT_EMBEDDED__: 'false',
		__SVELTEKIT_APP_VERSION__: '"skgo-spike"',
		__SVELTEKIT_ADAPTER_NAME__: '"skgo"',
		__SVELTEKIT_CLIENT_ROUTING__: 'true',
		__SVELTEKIT_HASH_ROUTING__: 'false',
		__SVELTEKIT_TRACK__: '__skgo_track',
		// kit's vite plugin injects these from svelte config; the values here are
		// the ones the example app's own build used (build/skgo.manifest.json).
		__SVELTEKIT_PATHS_BASE__: '""',
		__SVELTEKIT_PATHS_ASSETS__: '""',
		__SVELTEKIT_PATHS_ORIGIN__: '"http://127.0.0.1:8080"',
		__SVELTEKIT_PATHS_RELATIVE__: 'false',
		__SVELTEKIT_APP_DIR__: '"_app"',
		__SVELTEKIT_GLOBAL_NAME__: '"__sveltekit"',
		__SVELTEKIT_SERVICE_WORKER__: 'false',
		__SVELTEKIT_SUPPORTS_ASYNC__: 'true',
		__SVELTEKIT_SERVER_TRACING_ENABLED__: 'false',
		__SVELTEKIT_CSRF_CHECK_ORIGIN__: 'true',
		__SVELTEKIT_FORK_PRELOADS__: 'false',
		__SVELTEKIT_LINK_HEADER_PRELOAD__: 'false',
		__SVELTEKIT_APP_VERSION_FILE__: '"_app/version.json"',
		__SVELTEKIT_APP_VERSION_POLL_INTERVAL__: '0',
		__SVELTEKIT_APP_VERSION_CHECKS_ENABLED__: 'false',
		__SVELTEKIT_HAS_SERVER_LOAD__: 'true',
		__SVELTEKIT_HAS_UNIVERSAL_LOAD__: 'true',
		__SVELTEKIT_PAYLOAD__: 'undefined'
	},
	banner: {
		js:
			'var __skgo_track = function () {};' +
			// svelte's no-AsyncLocalStorage fallback is gated on this exact check
			// (svelte/src/internal/server/render-context.js:83), and kit reads the
			// same flag (kit/src/constants.js:30) to stop nulling its sync store.
			(ALS === 'webcontainer'
				? "globalThis.process = { versions: { webcontainer: 'skgo-spike' } };"
				: '')
	},
	plugins: [plugin],
	outfile: OUTFILE,
	logLevel: 'warning',
	metafile: true
});

/** kit's djb2, packages/kit/src/utils/hash.js */
function djb2(...values) {
	let hash = 5381;
	for (const value of values) {
		let i = value.length;
		while (i) hash = (hash * 33) ^ value.charCodeAt(--i);
	}
	return (hash >>> 0).toString(36);
}

const size = readFileSync(OUTFILE).length;
console.log(`built ${relative(HERE, OUTFILE)}  ${(size / 1024).toFixed(1)} KiB  als=${ALS} target=${TARGET}`);
if (result.warnings.length) console.log(`${result.warnings.length} warnings`);
