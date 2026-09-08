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
// `match` is the one substitution. Kit's own asks a manifest for the route,
// through `get_hooks().reroute` and `manifest._.matchers()`/`find_route`
// (utils/routing.js). Neither exists here: skgo does its own routing in Go,
// and Go already owns the exact table every page, load and endpoint request
// matches against, so building a second one for the engine would be the
// reimplementation this project's own rules forbid. What is below mirrors
// kit's own preparation of the pathname — decode it, strip the base — and
// then asks Go for the match, the same way a query or a command asks Go to
// run it.
//
// skgo has no reroute hook (Go does the routing, so `get_hooks` is always
// `{}`) and no param matchers (a matcher is a JavaScript function and skgo
// runs none — Loads.match and Endpoints.match both already match on the
// pattern alone), so both of kit's other inputs to match() are empty in this
// engine too, not just unavailable.
import { asset, resolve, base } from 'skgo:kit/paths-server';
import { decode_pathname } from 'skgo:kit/url';

export { asset, resolve };

export async function match(url) {
	const target = typeof url === 'string' ? new URL(url, 'https://skgo.internal/') : url;
	let pathname = decode_pathname(target.pathname);
	if (base && pathname.startsWith(base)) {
		pathname = pathname.slice(base.length) || '/';
	}
	const raw = globalThis.__skgo_match(pathname);
	return JSON.parse(raw);
}
