import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import skgo from '@skgo/adapter';

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
			// Every page in this app is rendered by Go per request right now —
			// none is prerendered. `/about` was, until the root layout gained a
			// Go load (src/routes/layout.server.go, for #81's error.html
			// fixture): kit can only prerender a branch with nothing Go-only in
			// it, and the root layout sits in every branch, so nothing here can
			// be prerendered until skgo can answer a server load while the build
			// prerenders (see about/+page.ts's own doc comment). Go's own engine
			// never prerenders anything either way — prerendering is kit's own
			// build-time pass, entirely before skgo's binary exists — so
			// `prerender` is always false on skgo's side of the ternary
			// (newDocumentCSP, csp.go), and `auto` resolves to nonce mode for
			// every page this build actually serves. The other half of the
			// ternary — a prerendered page resolving `auto` to hash, and kit
			// baking the result into the static file as a `<meta http-equiv>`
			// tag rather than a header — is proven at the Go test level instead
			// (csp_test.go's `TestCSPAutoMode_DynamicIsNonceModePrerenderedWouldBeHashMode`),
			// anchored to kit's own `Csp` constructor and `render.js`'s
			// prerendering branch, until a prerenderable page exists here again.
			//
			// `/stream` and `/live` are the two pages a CSP-and-streaming claim
			// has to be checked against: a value a load promises arrives later,
			// on the same response, as its own inline `<script>`
			// (data_serializer.js:103) — and under a nonce policy that script has
			// to carry the same nonce the boot script did, or the browser drops
			// it and the promised value never fills in.
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
