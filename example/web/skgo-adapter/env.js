/**
 * The SSR bundle, built as a fourth environment of kit's own Vite build.
 *
 * skgo used to bundle the engine's JavaScript itself, with esbuild, which
 * meant reimplementing the parts of kit's build that a server environment
 * already does: the Svelte server compile, the TypeScript strip, the `$app/*`
 * alias table, the twenty-three `__SVELTEKIT_*` defines and the epilogue that
 * gives every remote function the `<hash>/<name>` id the browser addresses it
 * by. Every one of those was a second answer to a question kit had already
 * answered, and each could drift from kit's answer without saying so.
 *
 * Declaring an environment instead means kit compiles the engine's bundle the
 * same way it compiles its own server: this file adds no compile step and no
 * alias for anything `$app/*` names. What is left is the one thing skgo has to
 * change — what `$app/server` means — and it stays a source-level substitution,
 * which is only possible before kit's graph is linked.
 *
 * Kit's remote plugin emits one entry chunk per `.remote` module into every
 * server environment (`exports/vite/index.js`, the `emitFile` at 686), which
 * makes this environment a code-splitting build, and rolldown refuses `iife`
 * for those. So the environment emits esm and a second `rolldown()` call over
 * its entry chunk alone folds it into the one script Go evaluates; the emitted
 * chunks are unreachable from that entry and fall away.
 */

import { rolldown } from 'vite/rolldown';
import { transformSync } from 'vite/rolldown/experimental';
import { createRequire } from 'node:module';
import { existsSync, readFileSync, realpathSync, rmSync } from 'node:fs';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';

/**
 * The runtime JavaScript, as files on disk beside this one — the way kit's own
 * adapters carry theirs (`adapter-node`, `adapter-vercel` and the rest locate a
 * `files/` directory from `import.meta.url`). They used to be `String.raw`
 * constants inside the adapter, which no editor, formatter or type checker
 * could see into.
 */
const here = dirname(fileURLToPath(import.meta.url));
const ENTRY = join(here, 'entry.js');
const APP_SERVER = join(here, 'app-server.js');
const POLYFILL = join(here, 'polyfill.js');

/**
 * The target the environment is compiled to. es2022 keeps native private class
 * fields; downlevelling them rewrites Svelte's server `Renderer` into WeakMap
 * lookups and costs between 2x and 7x, measured. Anything lower is refused —
 * the modules the engine's parser actually rejects are lowered one at a time
 * below instead.
 */
export const SSR_TARGET = 'es2022';

/**
 * The kit internals the entry reaches that no `$app/*` specifier names. They
 * are absolute paths, so the bundle holds one instance of each — one installed
 * transport, one devalue.
 *
 * realpath, because pnpm links the package: a module reached through the link
 * looks for kit's own dependencies (devalue, cookie) beside the link instead of
 * beside kit, and the app's node_modules has no such directory. Vite realpaths
 * what its own resolver returns; an id handed back from `resolveId` is taken as
 * given.
 *
 * @param {string} root the vite root
 * @returns {Record<string, string>}
 */
function kitAliases(root) {
	const kit = realpathSync(join(root, 'node_modules/@sveltejs/kit'));
	const runtime = join(kit, 'src/runtime');
	return {
		// kit's real `$app/server`, which app-server.js re-exports everything
		// it does not itself replace out of.
		'skgo:kit/app-server': join(runtime, 'app/server/index.js'),
		'skgo:kit/remote': join(runtime, 'app/server/remote/index.js'),
		'skgo:kit/shared': join(runtime, 'shared.js'),
		'skgo:kit/transport': join(runtime, 'app/internal/transport.js'),
		'skgo:kit/props': join(runtime, 'props.svelte.js'),
		'skgo:kit/root': join(runtime, 'components/root.svelte'),
		// devalue resolved from kit rather than from the app: kit is what
		// depends on it, and under pnpm the app's own node_modules has no such
		// directory. The entry needs it directly for the one thing kit's
		// `parse` cannot do — read a promise placeholder back.
		'skgo:devalue': createRequire(join(kit, 'package.json')).resolve('devalue')
	};
}

/** The constructs goja's parser rejects, and the only reason to lower a module. */
const UNPARSEABLE = /for\s+await\s*\(|async\s+function\s*\*|async\s*\*/;

/**
 * The goja environment: the vite plugin that declares it, and the call that
 * builds it and folds the result.
 *
 * They are one object because the plugin has to be handed to kit at
 * construction time — kit reads `adapter.vite.plugins.post` while it is
 * building its config — and the build cannot start until `adapt()` runs, which
 * is the first moment the node table is known.
 *
 */
export function gojaEnvironment() {
	/** filled in by build(), read by the plugin's virtual modules */
	const state = {
		nodes: '',
		hooks: '',
		// vite's ViteBuilder. Its own types do not name it in a form a JSDoc
		// annotation here can reach.
		/** @type {any} */
		builder: null
	};
	/** @type {Set<string>} the modules that had to be lowered, for the log */
	const lowered = new Set();

	const root = process.cwd();
	const aliases = kitAliases(root);

	const PREFIX = '\0skgo:';
	/** @type {Record<string, () => string>} */
	const sources = {
		// The node table the bundle renders through and the app's universal
		// hooks, both of which only exist once kit's build has finished.
		'skgo:nodes': () => state.nodes,
		'skgo:hooks': () => state.hooks,
		// esm-env's DEV/BROWSER are vite conditions kit's server build resolves
		// through package exports; this environment states them outright.
		'skgo:esm-env': () => 'export const DEV = false; export const BROWSER = false;',
		// `get_hooks` carries `reroute` and nothing else the engine needs; Go
		// does the routing.
		'skgo:generated': () => 'export const get_hooks = () => ({});',
		// Neither kit nor Svelte can reach AsyncLocalStorage here, and neither
		// needs to: the webcontainer flag in the banner selects Svelte's own
		// supported fallback — a module-global render context and a serialised
		// render queue — and flips the matching flag in kit.
		'skgo:missing': () =>
			'throw new Error("skgo: node:async_hooks is unavailable in the SSR engine");'
	};

	/** @type {import('vite').Plugin} */
	const plugin = {
		name: 'skgo-goja-environment',
		apply: 'build',

		/**
		 * The environment itself. A server consumer, like kit's own `ssr`, so
		 * that kit's `#app/*` imports resolve to their server variants; esm
		 * because kit's remote plugin makes it a code-splitting build; nothing
		 * external, because the engine has no module loader to reach anything
		 * with.
		 */
		config() {
			return {
				environments: {
					goja: {
						consumer: 'server',
						resolve: {
							noExternal: true,
							external: [],
							// deliberately no 'browser'
							conditions: ['node', 'production', 'module', 'import', 'default']
						},
						build: {
							outDir: GOJA_OUT,
							emptyOutDir: false,
							copyPublicDir: false,
							manifest: false,
							ssrEmitAssets: false,
							emitAssets: false,
							cssCodeSplit: false,
							minify: false,
							sourcemap: false,
							target: SSR_TARGET,
							rolldownOptions: {
								input: { bundle: ENTRY },
								preserveEntrySignatures: false,
								output: {
									format: 'esm',
									entryFileNames: 'bundle.js',
									assetFileNames: 'assets/[name][extname]',
									codeSplitting: false
								}
							}
						}
					}
				}
			};
		},

		// Vite hands every buildApp hook its builder, and kit's adapter hook
		// runs last (`order: 'post'`), so the builder is in hand by the time
		// adapt() asks for the environment to be built.
		buildApp: {
			order: 'pre',
			async handler(builder) {
				state.builder = builder;
			}
		},

		// Everything below is this environment's and no other's: kit's `ssr`
		// build keeps kit's own `$app/server`, and its client build is
		// untouched.
		applyToEnvironment(environment) {
			return environment.name === 'goja';
		},

		resolveId: {
			order: 'pre',
			handler(id) {
				if (id in sources) return PREFIX + id;
				if (id.startsWith(PREFIX)) return id;
				if (id in aliases) return aliases[id];
				// The one substitution. Kit aliases `$app/server` to its own
				// runtime module; this gets there first, and only here.
				if (id === '$app/server') return APP_SERVER;
				if (id === 'esm-env') return PREFIX + 'skgo:esm-env';
				if (id === '<sveltekit:generated>') return PREFIX + 'skgo:generated';
				if (id === 'node:async_hooks' || id === 'async_hooks') return PREFIX + 'skgo:missing';
				return null;
			}
		},

		load: {
			order: 'pre',
			handler(id) {
				if (!id.startsWith(PREFIX)) return null;
				const make = sources[id.slice(PREFIX.length)];
				if (!make) throw new Error('skgo: no virtual module ' + id.slice(PREFIX.length));
				return make();
			}
		},

		/**
		 * Per-module lowering, and the one module that cannot be bundled at
		 * all.
		 *
		 * oxc's `target` is a single coarse ES version with no esbuild-style
		 * per-feature `supported` map, so lowering `for await` and async
		 * generators for the whole environment would also downlevel every
		 * private class field — the 2x-to-7x the target above exists to avoid.
		 * Only the handful of modules that carry the two constructs goja's
		 * parser rejects are lowered, and none of them is under
		 * `svelte/src/internal/server/`.
		 */
		transform: {
			order: 'post',
			handler(code, id) {
				let out = code;
				// svelte/src/internal/server/crypto.js hides a `node:crypto`
				// import behind a variable so bundlers cannot resolve it. goja
				// rejects the syntax whether or not the branch is reached.
				if (out.includes('obfuscated_import')) {
					out = out.replace(
						/=>\s*import\(([\s\S]*?)\)/,
						'=> Promise.reject(new Error("skgo: no dynamic import in the SSR engine"))'
					);
				}
				if (!UNPARSEABLE.test(out)) {
					return out === code ? null : { code: out, map: null };
				}
				lowered.add(id);
				const result = transformSync(id.replace(/\0/g, '_'), out, { target: 'es2017' });
				return { code: result.code, map: null };
			}
		}
	};

	/**
	 * Builds the environment and folds it into the one script Go evaluates.
	 *
	 * The fold is a second `rolldown()` over the environment's own entry chunk.
	 * It reaches only that chunk and what it imports, so the entry chunks kit's
	 * remote plugin emitted are left behind, and `iife` is legal because what
	 * is left is a single entry.
	 *
	 * The polyfill is the fold's banner rather than a module of the bundle: in
	 * the folded file kit's shared chunk evaluates before the entry body, so a
	 * polyfill module would arrive after `new URL(...)` at kit's module scope.
	 *
	 * @param {{ nodes: string, hooks: string, log?: (message: string) => void }} generated
	 * @param {string} outfile
	 */
	async function build({ nodes, hooks, log = () => {} }, outfile) {
		if (!state.builder) {
			throw new Error(
				"skgo: vite never handed the adapter its builder, so the goja environment cannot be built. The adapter's `vite.plugins.post` hook did not reach kit."
			);
		}
		state.nodes = nodes;
		state.hooks = hooks;

		const environment = state.builder.environments.goja;
		if (!environment) {
			throw new Error(
				'skgo: vite built no `goja` environment. The adapter declares one in a `config` hook; this build of vite did not take it.'
			);
		}

		await state.builder.build(environment);
		if (lowered.size) {
			log(`skgo: lowered to es2017 for the engine's parser: ${lowered.size} module(s)`);
		}

		const chunk = resolve(root, environment.config.build.outDir, 'bundle.js');
		if (!existsSync(chunk)) {
			throw new Error(`skgo: the goja environment emitted no ${chunk}`);
		}

		const bundle = await rolldown({
			input: chunk,
			platform: 'neutral',
			// `import.meta` cannot survive into an iife, and one reaches this
			// graph: kit's `constants.js` exports `SRC_ROOT = import.meta.dirname`
			// for the benefit of its own prerenderer, which does not run here.
			// Saying so is what the previous build did too — esbuild's
			// `supported: { 'import-meta': false }` rewrote it silently. Naming
			// it keeps rolldown from warning about a rewrite that is the point.
			transform: { define: { 'import.meta': '{}' } }
		});
		try {
			await bundle.write({
				dir: dirname(resolve(root, outfile)),
				entryFileNames: basenameOf(outfile),
				format: 'iife',
				codeSplitting: false,
				banner: readFileSync(POLYFILL, 'utf-8')
			});
		} finally {
			await bundle.close();
		}
		rmSync(resolve(root, environment.config.build.outDir), { force: true, recursive: true });
	}

	return { plugin, build };
}

/**
 * Where the environment's own output goes before it is folded. It is removed
 * once the fold has read it; nothing downstream ever sees it.
 */
const GOJA_OUT = 'build/.goja';

/** @param {string} file */
function basenameOf(file) {
	return file.slice(file.lastIndexOf('/') + 1);
}

/**
 * The node table the bundle renders through: one static import per component
 * kit's build produced, in the manifest's own node order.
 *
 * @param {string} root
 * @param {Array<{ component: string | null }>} nodes
 */
export function nodeTable(root, nodes) {
	/** @type {string[]} */
	const imports = [];
	/** @type {string[]} */
	const table = [];
	nodes.forEach((node, i) => {
		if (!node.component) {
			table.push('undefined');
			return;
		}
		imports.push(`import N${i} from ${JSON.stringify(join(root, node.component))};`);
		table.push(`N${i}`);
	});
	return `${imports.join('\n')}\nexport const components = [${table.join(', ')}];\n`;
}
