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
			// "hash", not "nonce", and not the naive choice: nonce mode bakes a
			// fresh random value into *every* render of a page, which fails two
			// things at once. First, kit itself: `mode: 'nonce'` combined with
			// prerendering — `/about` is prerendered
			// (src/routes/about/+page.ts) — is refused outright at build time
			// ("Cannot use prerendering if config.csp.mode === 'nonce'",
			// render.js) before the build ever reaches Go. Second, and true even
			// for a build with nothing prerendered at all: kit's own ETag is a
			// hash of the transformed HTML *after* the nonce is substituted into
			// it (render.js's `headers.set('etag', hash(transformed))`), so a
			// nonced page's ETag changes on every single render — kit's own
			// conditional-GET support quietly stops working for any page that
			// needs one. skgo relies on exactly the guarantee nonce mode breaks
			// (example/ssr_test.go's TestARenderedPageDoesNotRepeatItself: the
			// same page rendered twice with unchanged data is byte-identical,
			// which is what makes an ETag meaningful at all), so nonce mode is
			// the wrong choice for a shared app config.
			//
			// Hash mode has neither problem — the hash is a pure function of the
			// boot script's own text, so it is as deterministic as the render
			// itself — and it is still the harder path to get right: it requires
			// Go to hash the *exact* bytes it is about to write into the
			// `<script>` tag, and a single wrong byte (a mismatched sha256, a
			// wrong base64 alphabet, a truncated string) fails silently in Go and
			// loudly in the browser — the tag simply won't hydrate. Nonce mode's
			// header/attribute pairing is exercised just as rigorously, but at
			// the Go test level, anchored to kit's own `Csp` output (csp_test.go).
			csp: { mode: 'hash', directives: { 'script-src': ['self'] } }
		})
	]
});
