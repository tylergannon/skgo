import { mkdirSync, rmSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

/**
 * The skgo adapter. It emits everything the Go binary embeds and nothing else:
 * the client bundle, kit's own SPA boot document, and a manifest describing the
 * routes Go must answer with that document. No server bundle is copied, so no
 * JavaScript runs in production.
 *
 * @param {{ out?: string }} [options]
 * @returns {import('@sveltejs/kit').Adapter}
 */
export default function skgo({ out = 'build' } = {}) {
	return {
		name: 'skgo',
		async adapt(builder) {
			rmSync(out, { force: true, recursive: true });

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
						}))
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
 * @param {string} file
 * @param {string} contents
 */
function write(file, contents) {
	mkdirSync(dirname(file), { recursive: true });
	writeFileSync(file, contents);
}
