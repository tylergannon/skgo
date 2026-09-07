import {
	mkdirSync,
	readdirSync,
	readFileSync,
	realpathSync,
	rmSync,
	statSync,
	writeFileSync
} from 'node:fs';
import { basename, dirname, join, relative, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import * as esbuild from 'esbuild';

/**
 * The skgo adapter. It emits everything the Go binary embeds and nothing else:
 * the client bundle, kit's own SPA boot document, the SSR bundle the Go process
 * renders pages with, and a manifest describing what Go must answer.
 *
 * The SSR bundle is the only JavaScript that runs in production, and it does no
 * I/O: it is kit's own root component and Svelte's renderer, with every remote
 * function's body replaced by a call back into Go.
 *
 * It also carries the remote-function ids and the server-load module paths
 * forward. `skgo generate` writes `skgo.remotes.json` beside this file when it
 * emits the `.remote.ts` modules and the `+*.server.ts` stubs; the adapter
 * checks that kit really compiled those modules and copies the lists into the
 * build, so the Go binary can refuse to serve a frontend that was built from a
 * different set of Go functions than it answers.
 *
 * @param {{ out?: string, precompress?: boolean }} [options]
 * @returns {import('@sveltejs/kit').Adapter}
 */
export default function skgo({ out = 'build', precompress = true } = {}) {
	return {
		name: 'skgo',
		async adapt(builder) {
			rmSync(out, { force: true, recursive: true });

			const generated = readGenerated();
			const { manifest: kit, source } = await readKitManifest(builder);
			const hashes = checkRemoteHashes(kit, generated.remotes);

			const nodes = await readNodes(builder, source, kit);
			const serverIds = nodes.map((node) => node.server);
			checkServerLoads(serverIds, generated.loads);

			const endpoints = checkEndpoints(builder, generated.endpoints);

			builder.writeClient(`${out}/client`);
			checkRemoteIds(`${out}/client`, generated.remotes, hashes);

			builder.writePrerendered(`${out}/prerendered`);
			const prerendered = readPrerendered(builder);
			// The SPA shell is still emitted: it is what a page whose branch turns
			// SSR off is answered with.
			await builder.generateFallback(`${out}/index.html`);

			// The document template, verbatim. Go substitutes `%sveltekit.head%`
			// and `%sveltekit.body%` into it the way kit's compiled template
			// function does, so the file a developer edits is the file that is
			// served.
			write(`${out}/app.html`, readFileSync(builder.config.files.appTemplate, 'utf-8'));
			await buildServerBundle(builder, nodes, `${out}/ssr/bundle.js`);

			// Kit's own `builder.compress` writes a `.br` and a `.gz` beside every
			// file whose extension it compresses, and Go chooses one per request
			// from Accept-Encoding. Same contract adapter-node ships with its
			// `precompress: true` default.
			if (precompress) {
				await builder.compress(`${out}/client`);
				await builder.compress(`${out}/prerendered`);
			}

			// `builder` has no writeJson in kit 3.0.0-next.25 (it existed on the
			// kit 2 line); write the manifest ourselves.
			write(
				`${out}/skgo.manifest.json`,
				JSON.stringify(
					{
						appDir: builder.config.appDir,
						base: builder.config.paths.base,
						version: builder.config.version.name,
						// One entry per node, positionally: the vite-root-relative
						// path of its `+*.server.ts`, or "" for a node that has
						// none. It is how Go finds the load that answers a slot of
						// a route's branch, and it is the same key kit itself
						// records in the node module it builds.
						nodes: serverIds,
						ssr: describeSSR(builder, kit, nodes),
						routes: kit._.routes.map((/** @type {any} */ route) => ({
							id: route.id,
							pattern: route.pattern.source,
							params: route.params,
							// The methods kit's build says the compiled `+server.ts`
							// exports. Without it a route that is an endpoint and
							// nothing else is indistinguishable from a page route, and
							// Go answered it with the boot document at HTTP 200.
							endpoint: endpoints.has(route.id)
								? { methods: endpoints.get(route.id) }
								: null,
							page: route.page
								? {
										// `[...layouts, leaf]` is the branch the
										// client's `x-sveltekit-invalidated` string
										// is positional over. A hole in `layouts` is
										// a slot no layout fills; it travels as -1
										// because JSON has no holes.
										layouts: Array.from(
											{ length: route.page.layouts.length },
											(_, i) => route.page.layouts[i] ?? -1
										),
										leaf: route.page.leaf
									}
								: null
						})),
						remotes: generated.remotes,
						prerendered,
						precompressed: precompress
					},
					null,
					'\t'
				)
			);

			builder.log.minor(`skgo: wrote ${out}/`);
		}
	};
}

/**
 * Reads what `skgo generate` emitted.
 *
 * A missing or shapeless file is a failure, not an empty list: an adapter that
 * quietly compares nothing to nothing reports success for an app whose two
 * halves were never checked against each other.
 *
 * @returns {{ remotes: string[], loads: string[], endpoints: Record<string, string[]> }}
 */
function readGenerated() {
	let raw;
	try {
		raw = readFileSync('skgo.remotes.json', 'utf-8');
	} catch {
		throw new Error(
			'skgo: skgo.remotes.json is missing. Run `go generate ./...` before building the frontend.'
		);
	}
	const parsed = JSON.parse(raw);
	if (!Array.isArray(parsed.remotes)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `remotes` array. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const id of parsed.remotes) {
		if (typeof id !== 'string' || !/^[^/]+\/[^/]+$/.test(id)) {
			throw new Error(
				`skgo: skgo.remotes.json lists ${JSON.stringify(id)}, which is not a <hash>/<name> id.`
			);
		}
	}
	if (!Array.isArray(parsed.loads)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `loads` array. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const module of parsed.loads) {
		if (typeof module !== 'string' || !/\/\+(page|layout)\.server\.ts$/.test(module)) {
			throw new Error(
				`skgo: skgo.remotes.json lists ${JSON.stringify(module)}, which is not a +page.server.ts or +layout.server.ts path.`
			);
		}
	}
	if (typeof parsed.endpoints !== 'object' || parsed.endpoints === null || Array.isArray(parsed.endpoints)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `endpoints` object. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const [id, methods] of Object.entries(parsed.endpoints)) {
		if (!id.startsWith('/') || !Array.isArray(methods) || methods.length === 0) {
			throw new Error(
				`skgo: skgo.remotes.json maps ${JSON.stringify(id)} to ${JSON.stringify(methods)}, which is not a route id and its methods.`
			);
		}
	}
	return { remotes: parsed.remotes, loads: parsed.loads, endpoints: parsed.endpoints };
}

/**
 * The same check for server routes, against the one place kit reports what it
 * compiled: `builder.routes[].api.methods`, which kit derives by importing each
 * built `+server.js` and reading its exports
 * (packages/kit/src/core/postbuild/analyse.js, `analyse_endpoint`). A `fallback`
 * export travels there as `'*'`, and `skgo generate` writes the same spelling,
 * so the two lists are compared literally.
 *
 * This is the check that makes a hand-written `+server.ts` fail the build rather
 * than 404 in the browser: kit would compile it, Go would never have been told
 * about it, and the route would answer nothing.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {Record<string, string[]>} declared
 * @returns {Map<string, string[]>} the methods kit compiled, per route id
 */
function checkEndpoints(builder, declared) {
	/** @type {Map<string, string[]>} */
	const built = new Map();
	for (const route of builder.routes) {
		if (route.api.methods.length > 0) built.set(route.id, [...route.api.methods].sort());
	}

	/** @type {string[]} */
	const problems = [];
	for (const [id, methods] of Object.entries(declared)) {
		const compiled = built.get(id);
		if (!compiled) {
			problems.push(`  generated but not compiled: ${methods.join(', ')} ${id}`);
			continue;
		}
		const want = [...methods].sort().join(', ');
		const got = compiled.join(', ');
		if (want !== got) {
			problems.push(`  ${id}: Go answers ${want}, the built +server.ts exports ${got}`);
		}
	}
	for (const [id, methods] of built) {
		if (!(id in declared)) {
			problems.push(`  compiled but not generated: ${methods.join(', ')} ${id}`);
		}
	}

	if (problems.length) {
		throw new Error(
			'skgo: skgo.remotes.json does not describe the server routes kit just compiled.\n' +
				problems.join('\n') +
				'\n  Every server route is written in Go. Run `go generate ./...`.'
		);
	}
	return built;
}

/**
 * The pathnames the build wrote a file for, exactly as kit records them:
 * percent-decoded and carrying the configured base. Go serves the prerendered
 * tree off this list and issues the trailing-slash 308 off it, which is what
 * adapter-node does with the same array.
 *
 * A prerendered *redirect* is refused rather than dropped. Kit records one when
 * a page it rendered redirected somewhere, and skgo has nothing that would
 * replay it — an app that produced one would silently lose it.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @returns {string[]}
 */
function readPrerendered(builder) {
	if (builder.prerendered.redirects.size > 0) {
		throw new Error(
			'skgo: the build prerendered a redirect, which skgo does not serve:\n' +
				[...builder.prerendered.redirects]
					.map(([from, { status, location }]) => `  ${from} -> ${status} ${location}`)
					.join('\n')
		);
	}
	return [...builder.prerendered.paths].sort();
}

/**
 * Kit's own server manifest, as an object *and* as text. `generateManifest`
 * returns the source of a module whose only imports sit inside lazy thunks, so
 * importing it resolves nothing and runs no application code — but the source
 * is worth keeping, because the thunks carry information the imported object
 * has already hidden inside closures. See readNodes.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @returns {Promise<{ manifest: any, source: string }>}
 */
async function readKitManifest(builder) {
	const dir = builder.getBuildDirectory('skgo');
	const file = join(dir, 'kit-manifest.js');
	const source = builder.generateManifest({ relativePath: '.' });
	mkdirSync(dir, { recursive: true });
	writeFileSync(file, `export const manifest = ${source};\n`);
	try {
		// The cache buster matters: `vp build` can run twice in one process.
		return { manifest: (await import(`${pathToFileURL(file).href}?t=${Date.now()}`)).manifest, source };
	} finally {
		rmSync(file, { force: true });
	}
}

/**
 * The vite-root-relative path of every node's `+*.server.ts`, in the positions
 * the manifest's own route branches point at. Kit records the path as
 * `server_id` in the node modules it builds, and that is the only place the
 * mapping from a branch slot back to an authored file survives the build.
 *
 * The positions are the trap. Kit *renumbers* nodes when it writes a manifest:
 * `generate_manifest` collects the nodes the surviving routes use and hands each
 * one a fresh consecutive index, so a route's `leaf` is a position in the
 * manifest's node array and not the number the node's own module declares. The
 * two agree only while every node is used — and prerendering a single page drops
 * that page's node and shifts every later one down by one.
 *
 * It shows up as a page whose Go load never runs while the layout above it works
 * perfectly, which reads like a bug in the load rather than in the manifest.
 *
 * The renumbering is recoverable because kit writes the original index into each
 * node's import path (`__memo(() => import('./nodes/6.js'))`), in the new order.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {string} source kit's generated manifest, as text
 * @param {any} kit the same manifest, imported
 * @returns {string[]}
 */
async function readNodes(builder, source, kit) {
	const dir = join(builder.getServerDirectory(), 'nodes');
	/** @type {Map<number, string>} */
	const files = new Map();
	for (const name of readdirSync(dir)) {
		if (!name.endsWith('.js')) continue;
		const file = join(dir, name);
		const index = Number(readFileSync(file, 'utf-8').match(/^export const index = (\d+);$/m)?.[1]);
		if (!Number.isInteger(index)) {
			throw new Error(`skgo: ${file} does not declare a node index`);
		}
		files.set(index, file);
	}

	const order = [...source.matchAll(/import\('[^']*\/nodes\/(\d+)\.js'\)/g)].map((m) =>
		Number(m[1])
	);
	if (order.length !== kit._.nodes.length) {
		throw new Error(
			`skgo: kit's manifest holds ${kit._.nodes.length} node(s) but ${order.length} node import(s) could be read out of it. ` +
				'The manifest no longer says which node module each branch slot points at, and skgo would silently run the wrong load.'
		);
	}

	/** @type {Node[]} */
	const nodes = [];
	for (const index of order) {
		const file = files.get(index);
		if (!file) {
			throw new Error(`skgo: kit's manifest imports nodes/${index}.js, which the build did not write`);
		}
		const module = await import(pathToFileURL(file).href);

		if (typeof module.universal?.load === 'function') {
			throw new Error(
				`skgo: ${module.universal_id} exports a \`load\`. Every load in a skgo app is written in Go; ` +
					'a universal load would have to run in the browser and in the SSR engine, and skgo runs neither.'
			);
		}

		nodes.push({
			server: module.server_id ?? '',
			component: module.component ? componentSource(file, dir) : null,
			imports: module.imports ?? [],
			stylesheets: module.stylesheets ?? [],
			fonts: module.fonts ?? [],
			ssr: option(module, 'ssr'),
			csr: option(module, 'csr')
		});
	}

	return nodes;
}

/**
 * @typedef {{
 *   server: string,
 *   component: string | null,
 *   imports: string[],
 *   stylesheets: string[],
 *   fonts: Array<{ file: string, filename: string }>,
 *   ssr: boolean | null,
 *   csr: boolean | null
 * }} Node
 */

/**
 * One of the two page options a node can set, from wherever it set it. Kit
 * reads the universal module first and falls back to the server one
 * (packages/kit/src/utils/page_nodes.js); Go does the reduction over the branch.
 *
 * @param {any} module
 * @param {'ssr' | 'csr'} option
 */
function option(module, option) {
	return module.universal?.[option] ?? module.server?.[option] ?? null;
}

/**
 * The `.svelte` file a node's component was compiled from, vite-root-relative.
 *
 * It comes out of the sourcemap kit's own build wrote beside the compiled
 * entry, because that is the record of what was compiled. Reversing kit's
 * `[id]` -> `_id_` directory encoding by hand is a guess that breaks on
 * `[...rest]`, and a wrong guess here renders the wrong page.
 *
 * @param {string} file the node module
 * @param {string} dir the directory it lives in
 */
function componentSource(file, dir) {
	const entry = readFileSync(file, 'utf-8').match(/import\('(\.\.\/entries\/[^']+)'\)/)?.[1];
	if (!entry) {
		throw new Error(`skgo: ${file} declares a component but does not import one`);
	}
	const map = resolve(dir, entry) + '.map';
	const sources = JSON.parse(readFileSync(map, 'utf-8')).sources;
	const own = resolve(dirname(map), sources[sources.length - 1]);
	if (!/^\+(page|layout|error)\.svelte$/.test(basename(own))) {
		throw new Error(
			`skgo: ${map} names ${own} as the last source of ${entry}, which is not a page, layout or error component. ` +
				'The sourcemap no longer says which component each node renders.'
		);
	}
	return relative(process.cwd(), own);
}

/**
 * Everything the Go process needs to render a document that is not already in
 * the manifest: where the bundle and the template are, the global the boot
 * script assigns, the client entry points and each node's client assets, and
 * the `ssr`/`csr` options Go reduces over a route's branch.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {any} kit kit's own server manifest
 * @param {Node[]} nodes
 */
function describeSSR(builder, kit, nodes) {
	const client = kit._.client;
	if (!client) {
		throw new Error('skgo: kit built no client bundle, so there is nothing for a document to boot.');
	}
	if (client.inline) {
		throw new Error(
			"skgo: `output.bundleStrategy: 'inline'` is not supported. skgo assembles the boot script itself and only knows the split form."
		);
	}
	if (client.routes) {
		throw new Error(
			"skgo: `router.resolution: 'server'` is not supported. skgo's routing is Go's, and the client resolves its own routes."
		);
	}

	return {
		bundle: 'ssr/bundle.js',
		template: 'app.html',
		target: SSR_TARGET,
		globalName: globalName(builder.config),
		assets: builder.config.paths.assets,
		relative: builder.config.paths.relative,
		client: {
			start: client.start,
			app: client.app ?? '',
			imports: client.imports ?? [],
			stylesheets: client.stylesheets ?? [],
			fonts: client.fonts ?? [],
			usesEnvDynamicPublic: !!client.uses_env_dynamic_public
		},
		nodes: nodes.map((node) => ({
			component: !!node.component,
			ssr: node.ssr,
			csr: node.csr,
			imports: node.imports,
			stylesheets: node.stylesheets,
			fonts: node.fonts
		}))
	};
}

/**
 * Kit's own manifest lists the remote *modules* it compiled, keyed by the hash
 * it derived from each module's path. If that set is not the one `skgo
 * generate` wrote, the two halves of the app were generated from different
 * sources and the build must not succeed.
 *
 * Modules are as far as this check can reach. `generate_manifest` in kit's
 * packages/kit/src/core/generate_manifest/index.js emits
 * `'<hash>': __memo(() => import('./chunks/remote-<hash>.js'))` and nothing
 * else; kit resolves the function half of an id at *runtime*, by looking the
 * name up on the imported module namespace
 * (packages/kit/src/runtime/server/remote-functions.js). There is no
 * per-function list in the manifest to compare against. checkRemoteIds below
 * reads the other build artifact that does carry the names.
 *
 * @param {any} kit kit's own server manifest
 * @param {string[]} remotes
 * @returns {Set<string>} the module hashes kit compiled
 */
function checkRemoteHashes(kit, remotes) {
	if (!kit._ || typeof kit._.remotes !== 'object' || kit._.remotes === null) {
		throw new Error(
			"skgo: kit's generated manifest has no `remotes` map. The adapter cannot tell which remote modules were compiled, so it cannot check them; this build of SvelteKit is not one skgo has been taught to read."
		);
	}
	const built = new Set(Object.keys(kit._.remotes));
	const declared = new Set(remotes.map((id) => id.split('/')[0]));

	const missing = [...declared].filter((hash) => !built.has(hash));
	const extra = [...built].filter((hash) => !declared.has(hash));
	if (missing.length || extra.length) {
		throw new Error(
			'skgo: skgo.remotes.json does not describe the remote modules kit just compiled.\n' +
				(missing.length ? `  generated but not compiled: ${missing.join(', ')}\n` : '') +
				(extra.length ? `  compiled but not generated: ${extra.join(', ')}\n` : '') +
				'  A generated .remote.ts only reaches the build once app code imports it. Run `go generate ./...`.'
		);
	}
	return built;
}

/**
 * Checks the remote functions the shipped bundle can actually call.
 *
 * The module-level check above is blind to an extra export added by hand to an
 * already-generated `.remote.ts`: the module's hash does not change, so both
 * sides still agree, the bundle calls `<hash>/<newName>`, and the Go binary —
 * which was never told about it — answers 404 in the browser.
 *
 * Kit's client carries the full id as a literal. Its vite plugin rewrites every
 * remote module's client half into one `export const <name> =
 * __remote.<type>('<hash>/<name>')` per export
 * (packages/kit/src/exports/vite/index.js), and the client runtime pastes that
 * id straight into `${base}/${app_dir}/remote/${id}`
 * (packages/kit/src/runtime/client/remote-functions/query/index.js). So the ids
 * survive bundling and minification as string literals, and the set of them in
 * the client we are about to ship is exactly the set of remote calls the
 * browser can make.
 *
 * Every one of those must be a function `skgo generate` declared. The reverse
 * is not required: a declared function no component imports is treeshaken out
 * of the client, and Go serving a function nobody calls yet breaks nothing.
 *
 * @param {string} clientDir the client bundle this build will ship
 * @param {string[]} remotes
 * @param {Set<string>} hashes the module hashes kit compiled
 */
function checkRemoteIds(clientDir, remotes, hashes) {
	const declared = new Set(remotes);
	const called = new Map();

	for (const file of walk(clientDir)) {
		if (!file.endsWith('.js')) continue;
		const source = readFileSync(file, 'utf-8');
		for (const [, hash, name] of source.matchAll(
			/['"`]([A-Za-z0-9]{1,16})\/([A-Za-z_$][A-Za-z0-9_$]*)['"`]/g
		)) {
			if (!hashes.has(hash)) continue;
			called.set(`${hash}/${name}`, file);
		}
	}

	if (called.size === 0 && hashes.size > 0) {
		throw new Error(
			`skgo: kit compiled ${hashes.size} remote module(s) but no remote-function id appears in ${clientDir}. Either nothing imports them, or the ids no longer survive bundling as literals and this check has stopped meaning anything.`
		);
	}

	const undeclared = [...called].filter(([id]) => !declared.has(id));
	if (undeclared.length) {
		throw new Error(
			'skgo: the built frontend calls remote functions that `skgo generate` did not write.\n' +
				undeclared.map(([id, file]) => `  ${id} (in ${file})`).join('\n') +
				'\n  Go answers only the generated ids, so every one of these would 404 in the browser.\n' +
				'  A remote function written by hand in a .remote.ts is not a supported mode: write it in the matching .remote.go and run `go generate ./...`.'
		);
	}
}

/**
 * @param {string} dir
 * @returns {Generator<string>}
 */
function* walk(dir) {
	for (const entry of readdirSync(dir)) {
		const path = join(dir, entry);
		if (statSync(path).isDirectory()) yield* walk(path);
		else yield path;
	}
}

/**
 * The same check for server loads. Kit decides whether its client asks for
 * `__data.json` at all by looking for a `load` export in the *built*
 * `+*.server.js`, so a stub that did not reach the build is a page whose Go load
 * is never called, and a compiled stub with no Go load behind it is a page whose
 * data nobody answers.
 *
 * @param {string[]} nodes
 * @param {string[]} loads
 */
function checkServerLoads(nodes, loads) {
	const built = new Set(nodes.filter(Boolean));
	const declared = new Set(loads);

	const missing = [...declared].filter((id) => !built.has(id));
	const extra = [...built].filter((id) => !declared.has(id));
	if (missing.length === 0 && extra.length === 0) return;

	throw new Error(
		'skgo: skgo.remotes.json does not describe the server loads kit just compiled.\n' +
			(missing.length ? `  generated but not compiled: ${missing.join(', ')}\n` : '') +
			(extra.length ? `  compiled but not generated: ${extra.join(', ')}\n` : '') +
			'  Every server load is written in Go. Run `go generate ./...`.'
	);
}

/**
 * @param {string} file
 * @param {string} contents
 */
function write(file, contents) {
	mkdirSync(dirname(file), { recursive: true });
	writeFileSync(file, contents);
}
/**
 * The target the SSR bundle is compiled to. es2022 keeps native private class
 * fields; downlevelling them rewrites Svelte's server `Renderer` into WeakMap
 * lookups and costs between 2x and 7x. Anything lower is refused.
 */
const SSR_TARGET = 'es2022';

/**
 * The web globals kit's runtime reaches at module-evaluation time that a bare
 * ECMAScript engine does not have. Deliberately small, and inside the bundle so
 * that the engine and a Node run of the same file execute identical code.
 *
 * Forced by `runtime/utils.js`, which constructs a `TextEncoder` and a
 * `TextDecoder` and calls `btoa`/`atob` on its top level, and by
 * `utils/url.js`, which constructs a `URL` on its top level. The real request
 * URL is parsed by Go; this parser exists so the modules evaluate.
 */
const SSR_POLYFILL = String.raw`
if (typeof globalThis.TextEncoder === 'undefined') {
	globalThis.TextEncoder = class TextEncoder {
		get encoding() { return 'utf-8'; }
		encode(str) {
			const bytes = [];
			for (let i = 0; i < str.length; i++) {
				let code = str.charCodeAt(i);
				if (code >= 0xd800 && code <= 0xdbff && i + 1 < str.length) {
					const next = str.charCodeAt(i + 1);
					if (next >= 0xdc00 && next <= 0xdfff) {
						code = (code - 0xd800) * 0x400 + next - 0xdc00 + 0x10000;
						i++;
					}
				}
				if (code < 0x80) bytes.push(code);
				else if (code < 0x800) bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
				else if (code < 0x10000)
					bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
				else
					bytes.push(
						0xf0 | (code >> 18),
						0x80 | ((code >> 12) & 0x3f),
						0x80 | ((code >> 6) & 0x3f),
						0x80 | (code & 0x3f)
					);
			}
			return new Uint8Array(bytes);
		}
	};
}

if (typeof globalThis.TextDecoder === 'undefined') {
	globalThis.TextDecoder = class TextDecoder {
		get encoding() { return 'utf-8'; }
		decode(input) {
			const bytes =
				input instanceof Uint8Array
					? input
					: new Uint8Array(input.buffer ?? input, input.byteOffset ?? 0, input.byteLength);
			let out = '';
			for (let i = 0; i < bytes.length; ) {
				const b = bytes[i];
				let code, len;
				if (b < 0x80) (code = b), (len = 1);
				else if (b < 0xe0) (code = b & 0x1f), (len = 2);
				else if (b < 0xf0) (code = b & 0x0f), (len = 3);
				else (code = b & 0x07), (len = 4);
				for (let j = 1; j < len; j++) code = (code << 6) | (bytes[i + j] & 0x3f);
				i += len;
				if (code > 0xffff) {
					code -= 0x10000;
					out += String.fromCharCode(0xd800 + (code >> 10), 0xdc00 + (code & 0x3ff));
				} else {
					out += String.fromCharCode(code);
				}
			}
			return out;
		}
	};
}

const B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

if (typeof globalThis.btoa === 'undefined') {
	globalThis.btoa = (binary) => {
		let out = '';
		for (let i = 0; i < binary.length; i += 3) {
			const a = binary.charCodeAt(i);
			const b = binary.charCodeAt(i + 1);
			const c = binary.charCodeAt(i + 2);
			out += B64[a >> 2];
			out += B64[((a & 3) << 4) | (isNaN(b) ? 0 : b >> 4)];
			out += isNaN(b) ? '=' : B64[((b & 15) << 2) | (isNaN(c) ? 0 : c >> 6)];
			out += isNaN(c) ? '=' : B64[c & 63];
		}
		return out;
	};
}

if (typeof globalThis.atob === 'undefined') {
	globalThis.atob = (b64) => {
		const clean = b64.replace(/=+$/, '');
		let out = '';
		let bits = 0;
		let acc = 0;
		for (const ch of clean) {
			const v = B64.indexOf(ch);
			if (v < 0) continue;
			acc = (acc << 6) | v;
			bits += 6;
			if (bits >= 8) {
				bits -= 8;
				out += String.fromCharCode((acc >> bits) & 0xff);
			}
		}
		return out;
	};
}

if (typeof globalThis.URL === 'undefined') {
	const ABS = /^([a-zA-Z][a-zA-Z0-9+.-]*:)\/\/([^/?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/;
	const SCHEME_ONLY = /^([a-zA-Z][a-zA-Z0-9+.-]*:)(.*)$/;

	class SearchParams {
		constructor(search) {
			this._ = [];
			for (const pair of String(search).replace(/^\?/, '').split('&')) {
				if (!pair) continue;
				const i = pair.indexOf('=');
				const k = decodeURIComponent((i < 0 ? pair : pair.slice(0, i)).replace(/\+/g, ' '));
				const v = i < 0 ? '' : decodeURIComponent(pair.slice(i + 1).replace(/\+/g, ' '));
				this._.push([k, v]);
			}
		}
		get(name) {
			for (const [k, v] of this._) if (k === name) return v;
			return null;
		}
		getAll(name) { return this._.filter(([k]) => k === name).map(([, v]) => v); }
		has(name) { return this.get(name) !== null; }
		keys() { return this._.map(([k]) => k)[Symbol.iterator](); }
		values() { return this._.map(([, v]) => v)[Symbol.iterator](); }
		entries() { return this._.map(([k, v]) => [k, v])[Symbol.iterator](); }
		[Symbol.iterator]() { return this.entries(); }
		toString() {
			return this._.map(([k, v]) => encodeURIComponent(k) + '=' + encodeURIComponent(v)).join('&');
		}
	}

	globalThis.URLSearchParams = SearchParams;

	globalThis.URL = class URL {
		constructor(input, base) {
			let href = String(input);
			if (base !== undefined && !SCHEME_ONLY.test(href)) {
				const b = base instanceof URL ? base : new URL(String(base));
				if (href.startsWith('//')) href = b.protocol + href;
				else if (href.startsWith('/')) href = b.origin + href;
				else {
					const dir = b.pathname.slice(0, b.pathname.lastIndexOf('/') + 1);
					const parts = (dir + href).split('/');
					const out = [];
					for (const p of parts) {
						if (p === '.') continue;
						if (p === '..') out.pop();
						else out.push(p);
					}
					href = b.origin + out.join('/');
				}
			}

			const m = ABS.exec(href);
			if (m) {
				this.protocol = m[1];
				this.host = m[2];
				this.hostname = m[2].split(':')[0];
				this.port = m[2].includes(':') ? m[2].split(':')[1] : '';
				this.pathname = m[3] || '/';
				this.search = m[4] || '';
				this.hash = m[5] || '';
				this.origin = this.protocol + '//' + this.host;
			} else {
				const s = SCHEME_ONLY.exec(href);
				if (!s) throw new TypeError('Invalid URL: ' + href);
				this.protocol = s[1];
				this.host = '';
				this.hostname = '';
				this.port = '';
				this.pathname = s[2];
				this.search = '';
				this.hash = '';
				this.origin = 'null';
			}
			this.searchParams = new SearchParams(this.search);
		}
		get href() {
			return (this.origin === 'null' ? this.protocol : this.origin) + this.pathname + this.search + this.hash;
		}
		toString() { return this.href; }
	};
}
`;

/**
 * The `$app/server` the SSR bundle sees.
 *
 * Kit's own `query`/`command`/`form` wrappers are kept: the argument-keyed
 * cache, `state.remote.implicit`, the `<id>/<payload>` key, the rule that an
 * argument needs a validator, the refusal to run a command during a render and
 * the derived event whose `url`, `params` and `route` throw inside a query are
 * all kit's, unchanged. The only substitution is the user function body —
 * the generated stub that throws — which becomes a call into Go.
 *
 * That inversion is the proof: the generated `.remote.ts` still throws, and the
 * host binding is the only path by which a value can reach the engine.
 */
const SSR_APP_SERVER = String.raw`
import * as real from 'skgo:kit/remote';
import { stringify_remote_arg } from 'skgo:kit/shared';

export { getRequestEvent } from '@sveltejs/kit/internal/server';

/**
 * Calls Go. Synchronous: Go has the answer in this process, so there is nothing
 * for an event loop to wait on.
 */
function host(id, payload) {
	const raw = globalThis.__skgo_remote(id, payload);
	const res = JSON.parse(raw);
	if (res.e) {
		const err = new Error(res.e.message);
		err.status = res.e.status ?? 500;
		throw err;
	}
	return res.v;
}

export function query(validate_or_fn, maybe_fn) {
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.query(validate_or_fn, fn) : real.query(fn);
	return wrapper;
}

query.batch = (validate_or_fn, maybe_fn) => {
	const fn = (args) => {
		const results = args.map((arg) => host(wrapper.__.id, stringify_remote_arg(arg)));
		return (_arg, i) => results[i];
	};
	const wrapper = maybe_fn ? real.query.batch(validate_or_fn, fn) : real.query.batch(fn);
	return wrapper;
};

query.live = (validate_or_fn, maybe_fn) => {
	// A live query is a stream and nothing in a render drives one. Declaring one
	// at module scope has to keep working; awaiting one during a render is what
	// fails, and it fails loudly.
	const fn = function* () {
		throw new Error('skgo: a live query cannot be awaited during server-side rendering');
	};
	return maybe_fn ? real.query.live(validate_or_fn, fn) : real.query.live(fn);
};

export function command(validate_or_fn, maybe_fn) {
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.command(validate_or_fn, fn) : real.command(fn);
	return wrapper;
}

export const form = real.form;
export const prerender = real.prerender;
export const requested = real.requested;
`;

/**
 * The entry point. It mirrors the one region of kit's `render_response` that
 * executes code (packages/kit/src/runtime/server/page/render.js): build the
 * `Props` linked list, call `render(Root, ...)` inside kit's request store, and
 * hand back `{ head, body }`. Everything on either side of that call — the boot
 * script, the hydration array, the remote data, the head buckets, the template
 * — is string assembly Go does with data Go already has.
 */
const SSR_ENTRY = String.raw`
import 'skgo:polyfill';
import { render } from 'svelte/server';
import Root from 'skgo:kit/root';
import { Props, RenderNode } from 'skgo:kit/props';
import { with_request_store } from '@sveltejs/kit/internal/server';
import { components } from 'skgo:nodes';

/**
 * A RequestState (packages/kit/src/types/internal.d.ts). Only the fields
 * kit's remote wrappers read during a render are populated. is_in_render is
 * what makes kit refuse a command.
 */
function make_state() {
	return {
		getClientAddress: () => '127.0.0.1',
		error: false,
		rerouted_url: null,
		depth: 0,
		remote: {
			data: null,
			implicit: null,
			explicit: null,
			forms: null,
			requested: null,
			batches: null,
			live_iterators: null
		},
		is_in_remote_function: false,
		is_in_render: true
	};
}

/**
 * A RequestEvent stand-in. run_remote_function spreads it and derives the
 * event a query actually sees, which is where kit makes url, params and
 * route throw. Go owns the real request; nothing here does I/O.
 */
function make_event(req, url) {
	return {
		cookies: {
			get: (name) => (req.cookies ?? {})[name],
			getAll: () => Object.entries(req.cookies ?? {}).map(([name, value]) => ({ name, value })),
			set: () => {},
			delete: () => {},
			serialize: () => ''
		},
		fetch: () => {
			throw new Error('skgo: fetch is not available during server-side rendering; load the data in Go');
		},
		getClientAddress: () => req.client_address ?? '127.0.0.1',
		locals: {},
		params: req.params ?? {},
		platform: undefined,
		request: { headers: { get: () => null }, method: 'GET' },
		route: { id: req.route_id ?? null },
		setHeaders: () => {},
		url,
		isDataRequest: false,
		isSubRequest: false,
		isRemoteRequest: false,
		tracing: { enabled: false }
	};
}

function build_props(req, url) {
	const page = {
		error: req.error ?? null,
		params: req.params ?? {},
		route: { id: req.route_id ?? null },
		status: req.status ?? 200,
		url,
		data: {},
		form: req.form ?? null,
		shallow: null,
		state: {}
	};

	const branch = req.branch ?? [];
	const error_components = (req.error_components ?? []).map((i) =>
		i == null ? undefined : components[i]
	);

	const props = new Props({
		page,
		tree: new RenderNode(components[branch[0].node], undefined),
		form: req.form ?? null,
		error: req.error ?? undefined
	});

	let current_node = props.tree;
	let data = props.page.data;

	for (let i = 0; i < branch.length; i += 1) {
		data = { ...data, ...branch[i].data };
		current_node.data = data;

		if (i < branch.length - 1) {
			current_node = current_node.child = new RenderNode(
				components[branch[i + 1].node],
				error_components[i + 1]
			);
		}
	}

	props.page.data = data;
	return props;
}

/**
 * Renders one page. The result object is filled in as the promise chain
 * settles; the host drains the job queue when this call returns, so done is
 * true by then or the render never finished — which is a Go error, not a
 * partial document.
 */
globalThis.__skgo_render = function (req_json) {
	const result = { done: false, error: '', head: '', body: '' };

	try {
		const req = JSON.parse(req_json);
		const url = new URL(req.url);
		const props = build_props(req, url);
		const state = make_state();
		const event = make_event(req, url);

		const options = { context: new Map([['__request__', { page: props.page }]]) };
		const promise = with_request_store({ event, state }, () => render(Root, { ...options, props }));

		Promise.resolve(promise).then(
			(rendered) => {
				result.head = rendered.head;
				result.body = rendered.body;
				result.done = true;
			},
			(err) => {
				result.error = (err && (err.stack || err.message)) || String(err);
				result.done = true;
			}
		);
	} catch (err) {
		result.error = (err && (err.stack || err.message)) || String(err);
		result.done = true;
	}

	return result;
};

// A liveness check Go calls once, when it puts a fresh runtime into the pool.
globalThis.__skgo_ping = function () {
	return 'ok';
};
`;

/**
 * The SSR bundle: the only JavaScript skgo runs at request time.
 *
 * It is kit's own `Root` component, kit's `Props`/`RenderNode`, kit's remote
 * function wrappers and Svelte's server renderer, bundled together with the
 * app's own components into one es2022 IIFE that a bare ECMAScript engine can
 * evaluate. Nothing in it does I/O: every remote function's body is a call back
 * into Go, and there is no `fetch`, no timer and no file system.
 *
 * `target` must stay at es2022. Downlevelling private class fields turns
 * Svelte's server `Renderer` into WeakMap lookups, which costs 2x to 7x and was
 * measured; `supported` turns off the four syntaxes the engine's parser rejects.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {Node[]} nodes the node table, in the manifest's own order
 * @param {string} outfile
 */
async function buildServerBundle(builder, nodes, outfile) {
	const cwd = process.cwd();
	// realpath: pnpm links the package, and esbuild would otherwise look for
	// kit's own dependencies (devalue, cookie, ...) next to the symlink.
	const kit = join(realpathSync(join(cwd, 'node_modules/@sveltejs/kit')), 'src');
	const svelte = join(cwd, 'node_modules/svelte');
	const { compile, compileModule } = await import(
		pathToFileURL(join(svelte, 'src/compiler/index.js')).href
	);

	const config = builder.config;
	const virtual = 'skgo-virtual';

	/** kit's user-facing specifiers, which only vite normally resolves. */
	const alias = {
		'$app/state': join(kit, 'runtime/app/state/index.js'),
		'$app/navigation': join(kit, 'runtime/app/navigation/index.js'),
		'$app/env': join(kit, 'runtime/app/env/index.js'),
		'$app/environment': join(kit, 'runtime/app/environment/index.js'),
		'$app/forms': join(kit, 'runtime/app/forms/index.js'),
		'$app/paths': join(kit, 'runtime/app/paths/index.js'),
		// `$app/server` is the one substitution: kit's real `query`/`command`/
		// `form` wrappers are kept, and only the user function body — the
		// generated stub that throws — is replaced by the call into Go.
		'$app/server': 'skgo:app-server',
		'skgo:kit/remote': join(kit, 'runtime/app/server/remote/index.js'),
		'skgo:kit/shared': join(kit, 'runtime/shared.js'),
		'skgo:kit/props': join(kit, 'runtime/props.svelte.js'),
		'skgo:kit/root': join(kit, 'runtime/components/root.svelte')
	};

	/** @type {Record<string, string>} */
	const sources = {
		'skgo:entry': SSR_ENTRY,
		'skgo:polyfill': SSR_POLYFILL,
		'skgo:app-server': SSR_APP_SERVER,
		'skgo:nodes': nodeTable(cwd, nodes),
		'skgo:esm-env': 'export const DEV = false; export const BROWSER = false;',
		'skgo:generated': 'export const get_hooks = () => ({});',
		// Neither kit nor Svelte can reach AsyncLocalStorage here, and neither
		// needs to: the webcontainer flag in the banner selects Svelte's own
		// supported fallback — a module-global render context and a serialised
		// render queue — and flips the matching flag in kit.
		'skgo:missing': 'throw new Error("skgo: node:async_hooks is unavailable in the SSR engine");'
	};

	const plugin = {
		name: 'skgo',
		/** @param {import('esbuild').PluginBuild} b */
		setup(b) {
			for (const [from, to] of Object.entries({ ...alias, ...Object.fromEntries(Object.keys(sources).map((k) => [k, k])) })) {
				const pattern = new RegExp('^' + from.replace(/[$/:\\.]/g, '\\$&') + '$');
				b.onResolve({ filter: pattern }, () =>
					to in sources ? { path: to, namespace: virtual } : { path: to }
				);
			}

			// esm-env's DEV/BROWSER are vite conditions; `<sveltekit:generated>`
			// and `node:async_hooks` are a virtual module and a Node builtin that
			// this bundle has neither of.
			b.onResolve({ filter: /^esm-env$/ }, () => ({ path: 'skgo:esm-env', namespace: virtual }));
			b.onResolve({ filter: /^<sveltekit:generated>/ }, () => ({
				path: 'skgo:generated',
				namespace: virtual
			}));
			b.onResolve({ filter: /^node:async_hooks$/ }, () => ({
				path: 'skgo:missing',
				namespace: virtual
			}));

			b.onLoad({ filter: /.*/, namespace: virtual }, (a) => ({
				contents: sources[a.path],
				loader: 'js',
				resolveDir: cwd
			}));

			// A `.svelte` file becomes its server-mode JavaScript, with the
			// TypeScript stripped out of `<script lang="ts">` first. This is the
			// app's own Svelte compiler in the mode kit itself uses.
			b.onLoad({ filter: /\.svelte$/ }, async (a) => {
				const source = await stripTypeScript(readFileSync(a.path, 'utf-8'));
				const { js, warnings } = compile(source, {
					filename: a.path,
					generate: 'server',
					experimental: { async: !!config.compilerOptions?.experimental?.async }
				});
				for (const w of warnings) {
					if (!/unused|a11y/.test(w.code)) builder.log.minor(`skgo: ${w.code}: ${w.message}`);
				}
				return { contents: js.code, loader: 'js', resolveDir: dirname(a.path) };
			});

			// `.svelte.js` modules carry runes — kit's own `props.svelte.js` uses
			// `$state.raw` — so they go through the compiler too.
			b.onLoad({ filter: /\.svelte\.js$/ }, (a) => {
				const { js } = compileModule(readFileSync(a.path, 'utf-8'), {
					filename: a.path,
					generate: 'server'
				});
				return { contents: js.code, loader: 'js', resolveDir: dirname(a.path) };
			});

			// A `.remote.ts` gets exactly the epilogue kit's own vite plugin
			// appends (packages/kit/src/exports/vite/index.js, `transform`), so
			// each export carries the `<hash>/<name>` id the browser addresses it
			// by and the Go host call is looked up under the same id.
			b.onLoad({ filter: /\.remote\.ts$/ }, async (a) => {
				const file = relative(cwd, a.path).replaceAll('\\', '/');
				const hash = djb2(file);
				const { code } = await esbuild.transform(readFileSync(a.path, 'utf-8'), {
					loader: 'ts',
					tsconfigRaw: { compilerOptions: { verbatimModuleSyntax: true } }
				});
				const epilogue = `
import * as $$self from ${JSON.stringify('./' + basename(a.path))};
import { init_remote_functions as $$init } from '@sveltejs/kit/internal/server';
$$init($$self, ${JSON.stringify(file)}, ${JSON.stringify(hash)});
for (const [$$name, $$fn] of Object.entries($$self)) {
	$$fn.__.id = ${JSON.stringify(hash)} + '/' + $$name;
	$$fn.__.name = $$name;
}
`;
				return { contents: code + epilogue, loader: 'js', resolveDir: dirname(a.path) };
			});
		}
	};

	const result = await esbuild.build({
		entryPoints: ['skgo:entry'],
		bundle: true,
		format: 'iife',
		platform: 'neutral',
		target: SSR_TARGET,
		supported: {
			// The engine's parser rejects these four. esbuild rewrites dynamic
			// import and `import.meta`, and lowers `for await` and async
			// generators, which kit's live-query and streaming paths use.
			'dynamic-import': false,
			'import-meta': false,
			'for-await': false,
			'async-generator': false
		},
		mainFields: ['module', 'main'],
		// no 'browser' condition: kit's `#app/*` imports must resolve to the
		// server variants.
		conditions: [],
		nodePaths: [join(cwd, 'node_modules')],
		absWorkingDir: cwd,
		define: ssrDefines(builder),
		banner: {
			js:
				'var __skgo_track = function () {};' +
				// Svelte's no-AsyncLocalStorage fallback is gated on exactly this
				// check (svelte/src/internal/server/render-context.js), and kit
				// reads the same flag (kit/src/constants.js) to stop nulling its
				// synchronous request store. It is Svelte's own supported path;
				// the cost is one render per runtime at a time, which the pool in
				// Go pays.
				"globalThis.process = { versions: { webcontainer: 'skgo' } };"
		},
		plugins: [plugin],
		outfile,
		logLevel: 'warning'
	});

	if (result.errors.length) {
		throw new Error(`skgo: the SSR bundle did not build:\n${result.errors.map((e) => '  ' + e.text).join('\n')}`);
	}
}

/**
 * The compile-time constants kit's own vite plugin injects. They are read from
 * the same config kit validated, so the bundle sees the app it was built for.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 */
function ssrDefines(builder) {
	const c = builder.config;
	const s = JSON.stringify;
	return {
		__SVELTEKIT_ADAPTER_NAME__: s('skgo'),
		__SVELTEKIT_APP_DIR__: s(c.appDir),
		__SVELTEKIT_APP_VERSION__: s(c.version.name),
		__SVELTEKIT_APP_VERSION_FILE__: s(`${c.appDir}/version.json`),
		__SVELTEKIT_APP_VERSION_POLL_INTERVAL__: s(c.version.pollInterval),
		__SVELTEKIT_APP_VERSION_CHECKS_ENABLED__: s(c.output.bundleStrategy !== 'inline'),
		__SVELTEKIT_CLIENT_ROUTING__: s(c.router.resolution === 'client'),
		__SVELTEKIT_CSRF_CHECK_ORIGIN__: s(!c.csrf.trustedOrigins.includes('*')),
		__SVELTEKIT_DEV__: 'false',
		__SVELTEKIT_EMBEDDED__: s(c.embedded),
		__SVELTEKIT_FORK_PRELOADS__: s(c.experimental.forkPreloads),
		__SVELTEKIT_GLOBAL_NAME__: s(globalName(c)),
		__SVELTEKIT_HASH_ROUTING__: s(c.router.type === 'hash'),
		__SVELTEKIT_LINK_HEADER_PRELOAD__: s(c.output.linkHeaderPreload),
		__SVELTEKIT_PATHS_ASSETS__: s(c.paths.assets),
		__SVELTEKIT_PATHS_BASE__: s(c.paths.base),
		__SVELTEKIT_PATHS_ORIGIN__: s(c.paths.origin ?? ''),
		__SVELTEKIT_PATHS_RELATIVE__: s(c.paths.relative),
		__SVELTEKIT_SERVER_TRACING_ENABLED__: s(c.tracing.server),
		__SVELTEKIT_SERVICE_WORKER__: 'false',
		__SVELTEKIT_SUPPORTS_ASYNC__: s(!!c.compilerOptions?.experimental?.async),
		__SVELTEKIT_TRACK__: '__skgo_track'
	};
}

/**
 * The `globalThis.__sveltekit_xxx` name the boot script assigns and the client
 * reads, derived the way kit derives it (packages/kit/src/core/utils.js).
 *
 * @param {any} config
 */
function globalName(config) {
	return `__sveltekit_${djb2(config.version.name)}`;
}

/**
 * The node table the bundle renders through: one static import per component
 * kit's build produced, in the manifest's own node order.
 *
 * @param {string} cwd
 * @param {Node[]} nodes
 */
function nodeTable(cwd, nodes) {
	const imports = [];
	const table = [];
	nodes.forEach((node, i) => {
		if (!node.component) {
			table.push('undefined');
			return;
		}
		imports.push(`import N${i} from ${JSON.stringify(join(cwd, node.component))};`);
		table.push(`N${i}`);
	});
	return `${imports.join('\n')}\nexport const components = [${table.join(', ')}];\n`;
}

/**
 * Strips the TypeScript out of every `<script lang="ts">` in a component,
 * leaving the markup untouched. Svelte's own compiler does not read TypeScript;
 * vite normally does this with a preprocessor.
 *
 * @param {string} source
 */
async function stripTypeScript(source) {
	const script = /<script([^>]*)>([\s\S]*?)<\/script>/g;
	/** @type {Array<[number, number, string]>} */
	const edits = [];
	for (const m of source.matchAll(script)) {
		if (!/lang=["']ts["']/.test(m[1])) continue;
		const { code } = await esbuild.transform(m[2], {
			loader: 'ts',
			// keeps imports that are only referenced from the template
			tsconfigRaw: { compilerOptions: { verbatimModuleSyntax: true } }
		});
		edits.push([m.index + m[0].indexOf(m[2]), m[2].length, code]);
	}
	for (const [at, len, code] of edits.reverse()) {
		source = source.slice(0, at) + code + source.slice(at + len);
	}
	return source;
}

/** kit's djb2, packages/kit/src/utils/hash.js */
function djb2(...values) {
	let hash = 5381;
	for (const value of values) {
		let i = value.length;
		while (i) hash = (hash * 33) ^ value.charCodeAt(--i);
	}
	return (hash >>> 0).toString(36);
}
