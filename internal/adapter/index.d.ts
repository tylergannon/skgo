import { Adapter } from '@sveltejs/kit';

interface AdapterOptions {
	/**
	 * The directory the build is written to, relative to the vite root. It
	 * defaults to `build`, which is what the app's Go embeds.
	 * @default 'build'
	 */
	out?: string;
	/** Authored Go build-service package, relative to the app's Go module root.
	 * Use RunPrerenderService to share renderer defaults with production SSR.
	 * Defaults to the generated prerender entry. */
	prerenderPackage?: string;
	/**
	 * Whether to write gzip and brotli copies of the assets and the prerendered
	 * pages beside them. The Go server serves whichever the browser asked for.
	 * @default true
	 */
	precompress?: boolean;
}

/**
 * The skgo adapter. It emits the client bundle, kit's SPA fallback document,
 * the SSR bundle the Go process renders pages with, and the manifest naming
 * what Go must answer.
 *
 * The manifest records which skgo this package is, and the Go that reads it
 * refuses a build written by a different one, so the two halves cannot drift.
 */
export default function skgo(options?: AdapterOptions): Adapter;
