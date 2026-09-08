import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import skgo from './skgo-adapter.js';

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: skgo(),
			// Kit 3 fixes the app's origin at build time.
			paths: { origin: process.env.ORIGIN ?? 'http://127.0.0.1:8080' },
			experimental: { remoteFunctions: true },
			compilerOptions: { experimental: { async: true } },
			// `mode: 'auto'` is kit's own default (`list(['auto', 'hash',
			// 'nonce'])`'s first option, core/config/options.js) — the config a
			// SvelteKit developer gets without ever touching `csp.mode` at all, so
			// it is the one a shared example app should demonstrate. Kit's own
			// rule for what `auto` resolves to, per page, is `use_hashes = mode
			// === 'hash' || (mode === 'auto' && prerender)` (`Csp`'s constructor,
			// runtime/server/page/csp.js): a page kit prerenders gets a hash, a
			// page it renders per-request gets a nonce.
			//
			// This app has both kinds of page under that one config. `/about` is
			// prerendered (src/routes/about/+page.ts) — kit's own Node build
			// resolves `auto` to hash mode for it and bakes the result straight
			// into the static file, entirely before skgo's binary exists, the way
			// prerendering always has. Every other page is rendered by Go per
			// request, where `auto` resolves to nonce mode (newDocumentCSP,
			// csp.go): the engine never prerenders anything — prerendering is
			// kit's own build-time pass — so `prerender` is always false on
			// skgo's side of the ternary, and `auto` is nonce mode there
			// unconditionally. `/stream` and `/live` are both pages of this
			// second kind, which is what makes them the two pages a
			// CSP-and-streaming claim has to be checked against: a value a load
			// promises arrives later, on the same response, as its own inline
			// `<script>` (data_serializer.js:103) — and under a nonce policy that
			// script has to carry the same nonce the boot script did, or the
			// browser drops it and the promised value never fills in.
			//
			// Explicit `mode: 'hash'` stays exercised too, just no longer here:
			// hash mode's own hashing (Go has to hash the *exact* bytes it is
			// about to write into the `<script>` tag, and a single wrong byte
			// fails silently in Go and loudly in the browser) is anchored at the
			// Go test level against kit's own `Csp` output (csp_test.go), and
			// hash mode's one real limitation — kit never nonces or hashes a
			// streamed chunk at all, so hash mode and streaming are exactly as
			// incompatible in skgo as they are in kit itself — is proven there
			// too, rather than baked into a shared app config that also needs to
			// stream.
			csp: { mode: 'auto', directives: { 'script-src': ['self'] } }
		})
	]
});
