import { mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from 'node:fs';
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
			checkRemoteHashes(kit, generated.remotes);

			const nodes = readNodes(builder);
			checkServerLoads(nodes, generated.loads);

			builder.writeClient(`${out}/client`);
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
	return { remotes: parsed.remotes ?? [], loads: parsed.loads ?? [] };
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
 * @param {any} kit
 * @param {string[]} remotes
 */
function checkRemoteHashes(kit, remotes) {
	const built = new Set(Object.keys(kit._.remotes ?? {}));
	const declared = new Set(remotes.map((id) => id.split('/')[0]));

	const missing = [...declared].filter((hash) => !built.has(hash));
	const extra = [...built].filter((hash) => !declared.has(hash));
	if (missing.length === 0 && extra.length === 0) return;

	throw new Error(
		'skgo: skgo.remotes.json does not describe the remote modules kit just compiled.\n' +
			(missing.length ? `  generated but not compiled: ${missing.join(', ')}\n` : '') +
			(extra.length ? `  compiled but not generated: ${extra.join(', ')}\n` : '') +
			'  A generated .remote.ts only reaches the build once app code imports it. Run `go generate ./...`.'
	);
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
