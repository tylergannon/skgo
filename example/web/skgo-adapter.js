import { mkdirSync, readdirSync, readFileSync, rmSync, statSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';

/**
 * The skgo adapter. It emits everything the Go binary embeds and nothing else:
 * the client bundle, kit's own SPA boot document, and a manifest describing the
 * routes Go must answer with that document. No server bundle is copied, so no
 * JavaScript runs in production.
 *
 * It also carries the remote-function ids forward. `skgo generate` writes
 * `skgo.remotes.json` beside this file when it emits the `.remote.ts` modules;
 * the adapter checks that kit really compiled those modules and copies the list
 * into the build, so the Go binary can refuse to serve a frontend that was
 * built from a different set of remote functions than it answers.
 *
 * @param {{ out?: string }} [options]
 * @returns {import('@sveltejs/kit').Adapter}
 */
export default function skgo({ out = 'build' } = {}) {
	return {
		name: 'skgo',
		async adapt(builder) {
			rmSync(out, { force: true, recursive: true });

			const remotes = readRemotes();
			const hashes = checkRemoteHashes(builder, remotes);

			builder.writeClient(`${out}/client`);
			checkRemoteIds(`${out}/client`, remotes, hashes);

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
						routes: builder.routes.map((route) => ({
							id: route.id,
							pattern: route.pattern.source
						})),
						remotes
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
 * Reads the ids `skgo generate` emitted.
 *
 * A missing or shapeless file is a failure, not an empty list: an adapter that
 * quietly compares nothing to nothing reports success for an app whose two
 * halves were never checked against each other.
 *
 * @returns {string[]}
 */
function readRemotes() {
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
			throw new Error(`skgo: skgo.remotes.json lists ${JSON.stringify(id)}, which is not a <hash>/<name> id.`);
		}
	}
	return parsed.remotes;
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
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {string[]} remotes
 * @returns {Set<string>} the module hashes kit compiled
 */
function checkRemoteHashes(builder, remotes) {
	const manifest = builder.generateManifest({ relativePath: '.' });
	const block = manifest.match(/remotes:\s*\{([^}]*)\}/);
	if (!block) {
		throw new Error(
			'skgo: kit\'s generated manifest has no `remotes` map. The adapter cannot tell which remote modules were compiled, so it cannot check them; this build of SvelteKit is not one skgo has been taught to read.'
		);
	}
	const built = new Set([...block[1].matchAll(/'([^']+)':/g)].map((match) => match[1]));
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
 * @param {string} file
 * @param {string} contents
 */
function write(file, contents) {
	mkdirSync(dirname(file), { recursive: true });
	writeFileSync(file, contents);
}
