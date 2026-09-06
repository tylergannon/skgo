import { mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

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
			checkRemoteHashes(builder, remotes);

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
	return JSON.parse(raw).remotes ?? [];
}

/**
 * Kit's own manifest lists the remote *modules* it compiled, keyed by the hash
 * it derived from each module's path. If that set is not the one `skgo
 * generate` wrote, the two halves of the app were generated from different
 * sources and the build must not succeed.
 *
 * @param {import('@sveltejs/kit').Builder} builder
 * @param {string[]} remotes
 */
function checkRemoteHashes(builder, remotes) {
	const manifest = builder.generateManifest({ relativePath: '.' });
	const block = manifest.match(/remotes:\s*\{([^}]*)\}/);
	const built = new Set(
		[...(block?.[1] ?? '').matchAll(/'([^']+)':/g)].map((match) => match[1])
	);
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
 * @param {string} file
 * @param {string} contents
 */
function write(file, contents) {
	mkdirSync(dirname(file), { recursive: true });
	writeFileSync(file, contents);
}
