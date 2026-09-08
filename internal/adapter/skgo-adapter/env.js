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
 * its entry chunk folds it into the one script Go evaluates. Kit's remote
 * entries can share application modules with that entry, so the fold follows
 * every imported emitted chunk while leaving unrelated entries behind.
 */

import { createRequire } from 'node:module';
import { existsSync, readdirSync, readFileSync, realpathSync, rmSync, statSync } from 'node:fs';
import { dirname, isAbsolute, join, relative, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

/**
 * Rolldown, resolved from the app rather than from this package.
 *
 * A plain `import ... from 'vite/rolldown'` resolves beside this file, and this
 * file is in `node_modules/@skgo/adapter`, whose own dependencies are not the
 * app's. Two of them would be fatal even where both exist: kit checks its dev
 * SSR environment with an `instanceof` against the project's resolved vite, so
 * a build driven by a second copy carries classes from a different module realm
 * and kit does not recognise them. The one rolldown that may build this app is
 * the one the app's vite ships, and the app is where it is asked for — the same
 * way kit's own internals are located, below.
 *
 * `process.cwd()` is the vite root: kit's build runs there, and the adapter
 * already reads `skgo.remotes.json` out of it.
 */
const app = createRequire(join(process.cwd(), 'package.json'));

/** @param {string} specifier */
async function fromApp(specifier) {
	try {
		return await import(pathToFileURL(app.resolve(specifier)).href);
	} catch (cause) {
		throw new Error(
			`skgo: the adapter builds the SSR bundle with the app's own vite, and ${specifier} ` +
				`could not be resolved from ${process.cwd()}. Run the install first.`,
			{ cause }
		);
	}
}

const { rolldown } = await fromApp('vite/rolldown');
const { transformSync } = await fromApp('vite/rolldown/experimental');
// `fetchModule` operates on the app's own `DevEnvironment`, so it has to be
// the app's copy for the same reason rolldown is: a second module realm's
// vite does not recognise this one's environments.
const { fetchModule, isCSSRequest } = await fromApp('vite');

// These are the same two Kit functions its own dev server calls in
// `update_manifest`: one derives the route/node graph from the authored tree,
// and the other writes the generated client modules that graph describes.
// The adapter reaches the app's installed Kit by absolute path because these
// internals are intentionally not package exports.
const kitRoot = realpathSync(join(process.cwd(), 'node_modules/@sveltejs/kit'));
const createManifestData = (
	await import(pathToFileURL(join(kitRoot, 'src/core/sync/create_manifest_data/index.js')).href)
).default;
const kitSync = await import(pathToFileURL(join(kitRoot, 'src/core/sync/sync.js')).href);

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
const APP_PATHS = join(here, 'app-paths.js');
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
 * The namespace the environment's own virtual modules live in. `\0` is
 * rollup's convention for an id no other plugin should touch.
 */
const PREFIX = '\0skgo:';

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
		// kit's own `$app/paths` server implementation, which app-paths.js
		// re-exports `resolve` and `asset` out of, and the pathname decoder kit's
		// own `match` prepares its argument with.
		'skgo:kit/paths-server': join(runtime, 'app/paths/server.js'),
		'skgo:kit/url': join(kit, 'src/utils/url.js'),
		// devalue resolved from kit rather than from the app: kit is what
		// depends on it, and under pnpm the app's own node_modules has no such
		// directory. The entry needs it directly for the one thing kit's
		// `parse` cannot do — read a promise placeholder back.
		'skgo:devalue': createRequire(join(kit, 'package.json')).resolve('devalue')
	};
}

/**
 * Whether a module is one of this package's own runtime files.
 *
 * @param {string} file
 */
function ours(file) {
	const rel = relative(here, file);
	return rel !== '' && !rel.startsWith('..') && !isAbsolute(rel);
}

/** Whether a specifier names a package rather than a path or a virtual module.
 *
 * @param {string} id
 */
function bare(id) {
	return !id.startsWith('.') && !id.startsWith('\0') && !id.startsWith('/') && !isAbsolute(id);
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
							// A server consumer gets Node builtins by default. This one
							// executes in goja and has none: leaving them external can
							// produce a successful build whose IIFE calls an undefined
							// `node_module` global before the first render.
							builtins: [],
							// Deliberately neither `node` nor `browser`. Kit and Svelte's
							// server entry points use their default exports, while an app
							// dependency such as `yaml` can choose its portable default
							// instead of a Node-only conditional export.
							conditions: ['production', 'module', 'import', 'default']
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
								// Vite otherwise gives every server consumer the Node
								// platform, which may synthesize `createRequire` while
								// linking a chunk shared with Kit's remote entry. The engine
								// is a neutral JavaScript host, not Node.
								platform: 'neutral',
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
			async handler(id, importer) {
				if (id in sources) return PREFIX + id;
				if (id.startsWith(PREFIX)) return id;
				if (id in aliases) return aliases[id];
				// The two substitutions. Kit aliases `$app/server` and
				// `$app/paths` to its own runtime modules; this gets there first,
				// and only here.
				if (id === '$app/server') return APP_SERVER;
				if (id === '$app/paths') return APP_PATHS;
				if (id === 'esm-env') return PREFIX + 'skgo:esm-env';
				if (id === '<sveltekit:generated>') return PREFIX + 'skgo:generated';
				if (id === 'node:async_hooks' || id === 'async_hooks') return PREFIX + 'skgo:missing';
				// A bare specifier in one of this package's own runtime files —
				// `svelte/server`, `@sveltejs/kit/internal/server` — names a
				// package the app depends on and this one does not. Resolving it
				// beside these files looks in node_modules/@skgo/adapter and
				// finds nothing; the app is where kit and Svelte are installed,
				// and it is the copy of each that the rest of this build already
				// uses. So the app asks on the file's behalf, through vite's own
				// resolver, which is the only thing here that knows the
				// conditions this environment resolves under.
				if (importer && ours(importer) && bare(id)) {
					return this.resolve(id, join(root, 'package.json'), { skipSelf: true });
				}
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
		 * all. See engineTransform.
		 */
		transform: {
			order: 'post',
			handler(code, id) {
				return engineTransform(code, id, { lowered });
			}
		}
	};

	/**
	 * Builds the environment and folds it into the one script Go evaluates.
	 *
	 * The fold is a second `rolldown()` over the environment's own entry chunk.
	 * It reaches that chunk and everything it imports, including chunks shared
	 * with entries emitted by kit's remote plugin. Unrelated entry chunks are
	 * left behind, and `iife` is legal because the fold itself has one entry.
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

/**
 * The engine's per-module transform, shared by the build and by dev.
 *
 * oxc's `target` is a single coarse ES version with no esbuild-style
 * per-feature `supported` map, so lowering `for await` and async generators
 * for the whole environment would also downlevel every private class field —
 * the 2x-to-7x SSR_TARGET exists to avoid. Only the handful of modules that
 * carry the two constructs goja's parser rejects are lowered, and none of them
 * is under `svelte/src/internal/server/`.
 *
 * `helpers` is only passed in dev. A module lowered to es2017 imports
 * `@oxc-project/runtime/helpers/...` by bare specifier, and this transform runs
 * after vite's import analysis, so nothing rewrites it: the module runner would
 * ask `fetchModule` for that specifier with an importer inside `svelte/`, and
 * under pnpm the package is a dependency of vite's own core rather than of the
 * app, so it cannot be resolved from there. The build never sees this, because
 * resolution there starts from the bundler. Naming the file outright is what
 * keeps dev working.
 *
 * @param {string} code
 * @param {string} id
 * @param {{ lowered?: Set<string>, helpers?: string }} options
 */
function engineTransform(code, id, { lowered, helpers } = {}) {
	let out = code;
	// svelte/src/internal/server/crypto.js hides a `node:crypto` import behind
	// a variable so bundlers cannot resolve it. goja rejects the syntax
	// whether or not the branch is reached.
	if (out.includes('obfuscated_import')) {
		out = out.replace(
			/=>\s*import\(([\s\S]*?)\)/,
			'=> Promise.reject(new Error("skgo: no dynamic import in the SSR engine"))'
		);
	}
	if (!UNPARSEABLE.test(out)) {
		return out === code ? null : { code: out, map: null };
	}
	lowered?.add(id);
	const result = transformSync(id.replace(/\0/g, '_'), out, { target: 'es2017' });
	let lower = result.code;
	if (helpers) {
		lower = lower.replace(
			/(['"`])@oxc-project\/runtime\/helpers\/([A-Za-z0-9_$]+)\1/g,
			(_m, quote, name) => quote + join(helpers, 'src/helpers/esm', name + '.js') + quote
		);
	}
	return { code: lower, map: null };
}

/**
 * Where oxc's lowering helpers live, resolved the way dev has to resolve them:
 * from vite rather than from the app. Under pnpm `@oxc-project/runtime` is a
 * dependency of vite's core and the app's own node_modules has no such
 * directory.
 *
 * @param {string} root the vite root
 */
function oxcHelpers(root) {
	// Not the module-level `fromApp`, which imports; this resolves a path.
	const fromRoot = createRequire(join(root, 'package.json'));
	const fromVite = createRequire(fromRoot.resolve('vite/package.json'));
	return dirname(fromVite.resolve('@oxc-project/runtime/package.json'));
}

/**
 * The paths in the document kit's own dev server writes, which are not the
 * paths a build writes: kit's client entry is served straight out of the
 * installed package over `/@fs`, its app module out of the generated dev tree,
 * and neither has any import, stylesheet or font beside it
 * (`exports/vite/dev/index.js`, the `_.client` of the dev manifest). The boot
 * global is `__sveltekit_dev` rather than `__sveltekit_<hash>`
 * (`core/utils.js`, `get_global_name`).
 *
 * @param {string} root the vite root
 * @param {string} outDir kit's `outDir`
 */
function devClient(root, outDir) {
	const kit = realpathSync(join(root, 'node_modules/@sveltejs/kit'));
	const runtime = join(kit, 'src/runtime');
	// kit's own get_runtime_base: a runtime inside the project is addressed by
	// a root-relative path, and one outside it — which is what pnpm's link
	// farm gives — over `/@fs`.
	const start = runtime.startsWith(root + '/')
		? '/' + posix(runtime.slice(root.length + 1)) + '/client/entry.js'
		: '/@fs' + posix(runtime) + '/client/entry.js';
	return {
		start,
		app: '/@fs' + posix(join(outDir, 'generated/dev/client/app.js')),
		imports: [],
		stylesheets: [],
		fonts: [],
		usesEnvDynamicPublic: true
	};
}

/** @param {string} p */
function posix(p) {
	return p.replace(/\\/g, '/');
}

/**
 * The node table in kit's own dev numbering.
 *
 * `create_manifest_data` is Kit's route scanner and `nodes` is its ordered
 * output. Reading that snapshot directly matters: the generated numbered files
 * are written one at a time and old high-numbered files can remain briefly
 * after a route is removed, so enumerating that directory can combine two
 * different route graphs. A node with no component leaves a hole, exactly as
 * the built table does.
 *
 * @param {any} manifest Kit's current manifest data
 * @param {string} root Vite's project root
 */
function devNodeTable(manifest, root) {
	if (!manifest) throw new Error('skgo: the live Kit manifest is not ready');
	/** @type {string[]} */
	const imports = [];
	/** @type {string[]} */
	const table = [];
	for (const [index, node] of manifest.nodes.entries()) {
		if (!node.component) {
			table.push('undefined');
			continue;
		}
		imports.push(`import N${index} from ${JSON.stringify(resolve(root, node.component))};`);
		table.push(`N${index}`);
	}
	return `${imports.join('\n')}\nexport const components = [${table.join(', ')}];\n`;
}

/**
 * The engine's environment in `vite dev`.
 *
 * `vite dev` never runs an adapter — kit reaches `adapt()` only from the
 * `apply: 'build'` plugin that finalises a build — but it does hand the
 * adapter's `vite.plugins.post` to vite unconditionally
 * (`exports/vite/index.js`, the returned plugin array). So the same `goja`
 * environment the build compiles can be declared in dev, where there is no
 * bundle at all, and Go can pull one transformed module at a time out of it
 * over vite's own `fetchModule`.
 *
 * Nothing else changes. The entry, the `$app/server` substitution, the kit
 * aliases and the per-module lowering are the build's; only where the modules
 * come from is different, and three things dev states differently from a
 * build:
 *
 *   - `esm-env`'s `DEV` is **true**. `vite-plugin-svelte` compiles components
 *     with Svelte's dev instrumentation in serve, and that instrumentation
 *     reads bookkeeping Svelte's runtime only keeps when `DEV` is true
 *     (`svelte/src/internal/server/context.js`, `push`). Saying false gives a
 *     component compiled one way a runtime built the other, and the render
 *     dies in `push_element`.
 *   - the node table is kit's dev numbering rather than the build's.
 *   - lowered modules name oxc's helper files outright. See engineTransform.
 *
 * @param {{ outDir?: string }} [options]
 */
export function gojaDevEnvironment({ outDir = '.svelte-kit' } = {}) {
	const root = process.cwd();
	const out = resolve(root, outDir);
	const aliases = kitAliases(root);
	const helpers = oxcHelpers(root);
	let manifestData;

	/** @type {Record<string, () => string>} */
	const sources = {
		'skgo:nodes': () => devNodeTable(manifestData, root),
		'skgo:hooks': () => hooksModule(root),
		'skgo:esm-env': () => 'export const DEV = true; export const BROWSER = false;',
		'skgo:generated': () => 'export const get_hooks = () => ({});',
		'skgo:missing': () =>
			'throw new Error("skgo: node:async_hooks is unavailable in the SSR engine");'
	};

	// Every file the dev server has seen change, oldest first, and the cursor
	// Go reads them with. It is filled from `hotUpdate` rather than from the
	// watcher because `hotUpdate` runs after vite has already invalidated its
	// own module graph for the file — a listener on the raw watcher races that,
	// and the module Go re-fetched would be the one it already had.
	/** @type {string[]} */
	const changed = [];
	let manifestVersion = 0;
	let manifestDirty = true;

	/** @type {import('vite').Plugin} */
	const plugin = {
		name: 'skgo-goja-dev-environment',
		apply: 'serve',

		config() {
			return {
				environments: {
					goja: {
						consumer: 'server',
						resolve: {
							noExternal: true,
							external: [],
							// The dev module runner executes in the same neutral, no-builtins
							// host as the production bundle. Keeping its resolver identical
							// prevents a package from changing implementation between modes.
							builtins: [],
							conditions: ['production', 'module', 'import', 'default']
						}
					}
				}
			};
		},

		configureServer(server) {
			const environment = server.environments.goja;
			if (!environment) {
				throw new Error(
					'skgo: vite created no `goja` environment for the dev server. The adapter declares one in a `config` hook; this build of vite did not take it.'
				);
			}

			const setup = server.config.plugins.find(
				(candidate) => candidate.name === 'vite-plugin-sveltekit-setup'
			);
			const kit = setup?.api?.options;
			if (!kit) {
				throw new Error(
					'skgo: Kit exposed no validated options on vite-plugin-sveltekit-setup; the dev manifest cannot mirror what Kit is serving.'
				);
			}

			const generated = JSON.parse(readFileSync(join(root, 'skgo.remotes.json'), 'utf-8'));
			const endpointMethods = generated.endpoints ?? {};

			const refreshManifest = () => {
				if (!manifestDirty && manifestData) return manifestData;
				manifestData = createManifestData(kit, root);
				// A skgo page request does not pass through Kit's SSR middleware, so
				// it cannot rely on Kit's first request to write these modules. This
				// is Kit's own sync.create call with the same arguments its dev
				// update_manifest uses.
				kitSync.create(kit, root, manifestData, false);
				// Invalidate only after the complete manifest snapshot is installed.
				// A watcher fires before this point and would make Vite cache a node
				// table generated from the preceding snapshot.
				const module = environment.moduleGraph.getModuleById(PREFIX + 'skgo:nodes');
				if (module) environment.moduleGraph.invalidateModule(module);
				changed.push(NODE_TABLE_URL);
				manifestDirty = false;
				manifestVersion++;
				return manifestData;
			};

			const devManifest = () => {
				const data = refreshManifest();
				return {
					version: manifestVersion,
					nodes: data.nodes.map((node) => node.server ?? ''),
					routes: data.routes
						.filter((route) => route.page || route.endpoint)
						.map((route) => ({
							id: route.id,
							pattern: route.pattern.source,
							params: route.params,
							endpoint: route.endpoint
								? { methods: endpointMethods[route.id] ?? [] }
								: null,
							page: route.page
								? {
										layouts: Array.from(
											{ length: route.page.layouts.length },
											(_, i) => route.page.layouts[i] ?? -1
										),
										errors: Array.from(
											{ length: route.page.errors.length },
											(_, i) => route.page.errors[i] ?? -1
										),
										leaf: route.page.leaf
									}
								: null
						})),
					ssr: {
						assets: kit.paths.assets,
						relative: kit.paths.relative,
						csp: kit.csp,
						nodes: data.nodes.map((node, index) => ({
							index,
							component: !!node.component,
							ssr: node.page_options?.ssr ?? null,
							csr: node.page_options?.csr ?? null
						}))
					},
					template: readFileSync(kit.files.appTemplate, 'utf-8'),
					errorTemplate: readDevErrorTemplate(kit)
				};
			};

			// Do this before Go can observe `/info`. It closes the clean-worktree
			// race where the server was listening but Kit had not yet received a
			// page request and therefore had not written its dev client/node tree.
			refreshManifest();

			// A route being added or removed changes Kit's node numbering. Record
			// the virtual table itself, not the generated client files Kit happens
			// to rewrite: those files are not an atomic snapshot and old numbered
			// files can remain while the tree is settling.
			server.watcher.on('all', (event, file) => {
				// `hotUpdate` is not dispatched for a custom environment until it
				// has its own HMR transport. Go is that environment's consumer, so
				// record and invalidate its graph directly from Vite's watcher.
				// Generated client files are browser inputs and, more importantly,
				// are rewritten by refreshManifest itself; feeding them back here
				// creates a change storm with no Go module to invalidate.
				if (normalize(file).startsWith(normalize(out) + '/')) return;
				for (const module of environment.moduleGraph.getModulesByFile(file) ?? []) {
					environment.moduleGraph.invalidateModule(module);
				}
				// Repeated edits are distinct cursor positions. In particular, an
				// editor replacing text and a test restoring it can be consecutive
				// events for the same path; collapsing them strands runtimes on the
				// temporary version forever.
				changed.push(file);

				const routeFile = normalize(file).startsWith(normalize(kit.files.routes) + '/');
				if (routeFile && (event === 'add' || event === 'unlink')) manifestDirty = true;
				const name = relative(kit.files.routes, file).split(/[\\/]/).at(-1);
				if (routeFile && /^\+(?:page|layout)(?:\.server)?\.[jt]s$/.test(name ?? '')) {
					manifestDirty = true;
				}
			});

			// What Go needs to know before it can ask for anything: which
			// module is the render entry, and what a dev document boots.
			server.middlewares.use('/__skgo_dev/info', (_req, res) => {
				try {
					json(res, 200, {
						entry: ENTRY,
						client: devClient(root, out),
						globalName: '__sveltekit_dev',
						manifest: devManifest()
					});
				} catch (e) {
					json(res, 500, { error: errorText(e) });
				}
			});

			// Kit's dev renderer inlines every CSS dependency of the nodes in the
			// rendered branch. Go asks after it has evaluated those nodes, so the
			// goja environment's module graph contains the same dependency edges
			// Kit's renderer would walk.
			server.middlewares.use('/__skgo_dev/styles', async (req, res) => {
				let body = '';
				for await (const chunk of req) body += chunk;
				try {
					const { nodes = [] } = JSON.parse(body || '{}');
					const data = refreshManifest();
					const deps = new Set();
					for (const index of nodes) {
						const node = data.nodes[index];
						if (!node) continue;
						for (const file of [node.component, node.universal]) {
							if (!file) continue;
							const id = resolve(root, file);
							const module =
								environment.moduleGraph.getModuleById(id) ??
								(await environment.moduleGraph.getModuleByUrl('/' + posix(file)));
							if (module) await findDevDeps(environment, module, deps);
						}
					}

					const styles = [];
					const runner = server.environments.ssr.runner;
					for (const dep of deps) {
						if (!isCSSRequest(dep.url) || VITE_CSS_QUERY.test(dep.url)) continue;
						const inline = dep.url.includes('?')
							? dep.url.replace('?', '?inline&')
							: dep.url + '?inline';
						try {
							styles.push((await runner.import(inline)).default);
						} catch {
							// Kit deliberately ignores a CSS module that vanished from the
							// graph between traversal and import (typically a dynamic import).
						}
					}
					json(res, 200, { styles });
				} catch (e) {
					json(res, 500, { error: errorText(e) });
				}
			});

			// One transformed module, exactly as vite's own module runner would
			// receive it (`ssr/fetchModule.ts`): the SSR transform's output and
			// the file it came from.
			server.middlewares.use('/__skgo_dev/module', async (req, res) => {
				let body = '';
				for await (const chunk of req) body += chunk;
				try {
					const { url, importer } = JSON.parse(body || '{}');
					json(
						res,
						200,
						await fetchModule(server.environments.goja, url, importer, {
							inlineSourceMap: false
						})
					);
				} catch (e) {
					json(res, 500, { error: errorText(e) });
				}
			});

			// What has changed since Go last asked. Go drops exactly the
			// modules these files reach and re-imports the entry; everything
			// else in its module cache is still the one the last render used.
			server.middlewares.use('/__skgo_dev/changed', (req, res) => {
				const since = Number(new URL(req.url ?? '/', 'http://skgo').searchParams.get('since'));
				const from = Number.isInteger(since) && since >= 0 ? since : 0;
				json(res, 200, {
					version: changed.length,
					// A cursor from before this server started is a Go process
					// that outlived a vite restart. It cannot know what it
					// missed, so it is told to start over.
					reset: from > changed.length,
					files: from > changed.length ? [] : changed.slice(from)
				});
			});
		},

		applyToEnvironment(environment) {
			return environment.name === 'goja';
		},

		// Runs for this environment after vite has invalidated its module graph
		// for the file (`handleHMRUpdate`), which is the ordering the cursor
		// above depends on.
		hotUpdate({ file }) {
			const name = relative(kit.files.routes, file).split(/[\\/]/).at(-1);
			if (/^\+(?:page|layout)(?:\.server)?\.[jt]s$/.test(name ?? '')) {
				manifestDirty = true;
			}
			return undefined;
		},

		resolveId: {
			order: 'pre',
			async handler(id, importer) {
				if (id in sources) return PREFIX + id;
				if (id.startsWith(PREFIX)) return id;
				if (id in aliases) return aliases[id];
				if (id === '$app/server') return APP_SERVER;
				if (id === '$app/paths') return APP_PATHS;
				if (id === 'esm-env') return PREFIX + 'skgo:esm-env';
				if (id === '<sveltekit:generated>') return PREFIX + 'skgo:generated';
				if (id === 'node:async_hooks' || id === 'async_hooks') return PREFIX + 'skgo:missing';
				if (id.startsWith(OXC_HELPERS)) {
					return join(helpers, 'src/helpers/esm', id.slice(OXC_HELPERS.length) + '.js');
				}
				// The same question the build's environment answers, and for the
				// same reason: this package's own runtime files name packages
				// the app depends on and this one does not, and they are read
				// from wherever `@skgo/adapter` is installed rather than from
				// inside the app. The app asks on their behalf.
				if (importer && ours(importer) && bare(id)) {
					return this.resolve(id, join(root, 'package.json'), { skipSelf: true });
				}
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

		transform: {
			order: 'post',
			handler(code, id) {
				return engineTransform(code, id, { helpers });
			}
		}
	};

	return plugin;
}

const OXC_HELPERS = '@oxc-project/runtime/helpers/';

/**
 * The URL vite gives the node table, which is how Go names it when it drops it.
 * A virtual module has no file for a change to be attributed to, so the change
 * log carries this instead.
 */
const NODE_TABLE_URL = '/@id/__x00__skgo:skgo:nodes';
const VITE_CSS_QUERY = /(?:\?|&)(?:raw|url|inline)(?:&|$)/;

/**
 * Kit's dev `find_deps`, with the environment made explicit. Its own copy is
 * closed over the `ssr` environment; skgo's component modules were evaluated
 * in `goja`, so that is the graph whose transform-result URLs are authoritative.
 *
 * @param {import('vite').DevEnvironment} environment
 * @param {import('vite').EnvironmentModuleNode} node
 * @param {Set<import('vite').EnvironmentModuleNode>} deps
 */
async function findDevDeps(environment, node, deps) {
	const branches = [];
	const add = async (dependency) => {
		if (deps.has(dependency)) return;
		deps.add(dependency);
		await findDevDeps(environment, dependency, deps);
	};
	const addByURL = async (url) => {
		const dependency = await environment.moduleGraph.getModuleByUrl(url);
		if (dependency) await add(dependency);
	};

	if (node.transformResult) {
		for (const url of node.transformResult.deps ?? []) branches.push(addByURL(url));
		for (const url of node.transformResult.dynamicDeps ?? []) branches.push(addByURL(url));
	} else {
		for (const dependency of node.importedModules) branches.push(add(dependency));
	}
	await Promise.all(branches);
}

/** @param {any} kit Kit's validated config */
function readDevErrorTemplate(kit) {
	try {
		return readFileSync(kit.files.errorTemplate, 'utf-8');
	} catch {
		return readFileSync(join(kitRoot, 'src/core/config/default-error.html'), 'utf-8');
	}
}

/** @param {string} p */
function normalize(p) {
	return posix(p);
}

/**
 * The module the engine reads the app's `transport` hook out of, in dev. The
 * build's equivalent is written by the adapter from `builder.config`; here the
 * file is found the way kit's own `resolve_entry` finds it.
 *
 * @param {string} root the vite root
 */
function hooksModule(root) {
	const file = resolveEntry(join(root, 'src/hooks'));
	if (!file) return 'export const transport = {};';
	return (
		`import * as hooks from ${JSON.stringify(file)};\n` +
		'export const transport = hooks.transport ?? {};\n'
	);
}

/**
 * kit's `resolve_entry` (packages/kit/src/utils/filesystem.js): an
 * extensionless path becomes the file that is actually there.
 *
 * @param {string} entry
 * @returns {string | null}
 */
function resolveEntry(entry) {
	if (existsSync(entry)) {
		if (statSync(entry).isFile()) return entry;
		const index = join(entry, 'index');
		if (existsSync(index + '.js') || existsSync(index + '.ts')) return resolveEntry(index);
	}

	const dir = dirname(entry);
	if (existsSync(dir)) {
		const base = entry.slice(entry.lastIndexOf('/') + 1);
		const found = readdirSync(dir).find(
			(file) => file.replace(/\.(js|ts)$/, '') === base && statSync(join(dir, file)).isFile()
		);
		if (found) return join(dir, found);
	}

	return null;
}

/**
 * @param {import('node:http').ServerResponse} res
 * @param {number} status
 * @param {unknown} body
 */
function json(res, status, body) {
	res.statusCode = status;
	res.setHeader('content-type', 'application/json');
	res.end(JSON.stringify(body));
}

/** @param {unknown} e */
function errorText(e) {
	if (e instanceof Error) return e.stack ?? e.message;
	return String(e);
}
