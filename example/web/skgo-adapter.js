import { mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { pathToFileURL } from 'node:url';

/**
 * The skgo adapter. It emits everything the Go binary embeds and nothing else:
 * the client bundle, kit's own SPA boot document, and a manifest describing the
 * routes Go must answer with that document. No server bundle is copied, so no
 * JavaScript runs in production.
 *
 * It also carries the remote-function ids and the server-load module paths
 * forward. `skgo generate` writes `skgo.remotes.json` beside this file when it
 * emits the `.remote.ts` modules and the `+*.server.ts` stubs; the adapter
 * checks that kit really compiled those modules and copies the lists into the
 * build, so the Go binary can refuse to serve a frontend that was built from a
 * different set of Go functions than it answers.
 *
 * @param {{ out?: string }} [options]
 * @returns {import('@sveltejs/kit').Adapter}
 */
export default function skgo({ out = 'build' } = {}) {
	return {
		name: 'skgo',
		async adapt(builder) {
			rmSync(out, { force: true, recursive: true });

			const generated = readGenerated();
			const kit = await readKitManifest(builder);
			const hashes = checkRemoteHashes(kit, generated.remotes);

			const nodes = readNodes(builder);
			checkServerLoads(nodes, generated.loads);

			builder.writeClient(`${out}/client`);
			checkRemoteIds(`${out}/client`, generated.remotes, hashes);

			builder.writePrerendered(`${out}/prerendered`);
			await builder.generateFallback(`${out}/index.html`);

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
						nodes,
						routes: kit._.routes.map((/** @type {any} */ route) => ({
							id: route.id,
							pattern: route.pattern.source,
							params: route.params,
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
						remotes: generated.remotes
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
 * @returns {{ remotes: string[], loads: string[] }}
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
	return { remotes: parsed.remotes, loads: parsed.loads };
}

/**
 * Kit's own server manifest, as an object rather than as text. `generateManifest`
 * returns the source of a module whose only imports sit inside lazy thunks, so
 * importing it resolves nothing and runs no application code.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @returns {Promise<any>}
 */
async function readKitManifest(builder) {
	const dir = builder.getBuildDirectory('skgo');
	const file = join(dir, 'kit-manifest.js');
	mkdirSync(dir, { recursive: true });
	writeFileSync(
		file,
		`export const manifest = ${builder.generateManifest({ relativePath: '.' })};\n`
	);
	try {
		// The cache buster matters: `vp build` can run twice in one process.
		return (await import(`${pathToFileURL(file).href}?t=${Date.now()}`)).manifest;
	} finally {
		rmSync(file, { force: true });
	}
}

/**
 * The vite-root-relative path of every node's `+*.server.ts`, indexed by node.
 * Kit records it as `server_id` in the node modules it builds, and that is the
 * only place the mapping from a branch slot back to an authored file survives
 * the build.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @returns {string[]}
 */
function readNodes(builder) {
	const dir = join(builder.getServerDirectory(), 'nodes');
	/** @type {string[]} */
	const nodes = [];
	for (const name of readdirSync(dir)) {
		if (!name.endsWith('.js')) continue;
		const source = readFileSync(join(dir, name), 'utf-8');
		const index = Number(source.match(/^export const index = (\d+);$/m)?.[1]);
		if (!Number.isInteger(index)) {
			throw new Error(`skgo: ${join(dir, name)} does not declare a node index`);
		}
		nodes[index] = source.match(/^export const server_id = "([^"]*)";$/m)?.[1] ?? '';
	}
	for (let i = 0; i < nodes.length; i += 1) nodes[i] ??= '';
	return nodes;
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
