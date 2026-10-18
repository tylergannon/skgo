// The `$app/paths` the SSR bundle sees.
//
// Like app-server.js, it is a module of the `goja` environment and of no
// other: the adapter's plugin resolves `$app/paths` to this file only while
// that environment is building, so kit's own `ssr` build keeps kit's module.
//
// `resolve` and `asset` are kit's own (runtime/app/paths/server.js),
// unchanged — they are pure string logic over the app's compiled-in base and
// assets path, needing nothing this engine lacks — and they arrive here by
// re-export rather than by copy.
//
// `match` delegates the full URL to the request's Go routing boundary, which
// applies the Go reroute hook and paired parameter matchers before selection.
import { asset, resolve } from 'skgo:kit/paths-server';

export { asset, resolve };

export async function match(url) {
	const target = typeof url === 'string' ? url : url.href;
	const raw = globalThis.__skgo_match(target);

	return JSON.parse(raw);
}
