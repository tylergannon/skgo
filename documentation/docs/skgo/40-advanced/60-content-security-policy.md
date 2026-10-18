---
title: Content security policy
summary: Kit's csp configuration, enforced by the Go renderer with per-request nonces and Go-computed hashes.
proof:
  - example/e2e/features/csp.feature
  - TestCSPAutoModeBehavesAsNonceBecauseSkgoNeverPrerenders
  - TestStreamedChunkCarriesTheSameNonceAsTheBootScript
  - TestStreamedChunkCarriesNoAttributeInHashMode
  - TestValidateReportOnlyRequiresAReportChannel
  - TestCSPHeaderMatchesKit_DeclarationOrderNotConfigOrder
  - TestProductionCSPHashReportingKeywords
---

You configure a content security policy the same way as in SvelteKit, with the [`csp`](https://svelte.dev/docs/kit/@sveltejs-kit-vite#csp) option. Kit's runtime does not build the header in production; the Go renderer builds the same header Kit would.

## Configuration

Set `csp` in the SvelteKit plugin options:

```ts
/// file: vite.config.ts
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";
import skgo from "@skgo/sveltekit-adapter";

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: skgo(),
			csp: { mode: "auto", directives: { "script-src": ["self"] } }
		})
	]
});
```

You do not configure anything in Go; a change to `csp` needs a rebuild. The policy goes out as a `Content-Security-Policy` response header, and `reportOnly` goes out as `Content-Security-Policy-Report-Only`. Directives appear in the header in Kit's declaration order, not in the order you wrote them.

## Modes

Kit decides per page whether to use a hash or a nonce: `hash` mode always uses hashes, `nonce` mode always uses nonces, and `auto` uses hashes for prerendered pages and nonces for everything else. SKGo follows the same rule. The difference is who renders which page.

- **Pages the Go server renders** are never prerendered, so `auto` resolves to nonce mode for every one of them: server-rendered pages, error pages, and the shell served when `ssr` is off.
- **Prerendered pages** are built by Kit's own prerender during `vp build`. With `auto` or `hash` mode, Kit writes the hash policy into the page as a `<meta http-equiv="content-security-policy">` tag, and the Go server serves that file as it is. Kit leaves `frame-ancestors`, `report-uri` and `sandbox` out of a meta policy, because browsers ignore them there. If you need those on a prerendered page, set them as headers yourself, in a wrapper around the handler.

### nonce

For each request the renderer generates a nonce, as Kit does, and adds it to the boot script, to the inline script that carries hydration data, and to `script-src`, or to `script-src-elem` when you declared that directive instead. `%sveltekit.nonce%` in `src/app.html` gets the same value.

### hash

In `hash` mode, a page the Go server renders gets a `sha256` hash of each inline script, computed in Go over the exact bytes it writes. The inline script for hydration data is hashed too. The script tags carry no nonce.

## Styles

SKGo links component CSS as files and never inlines it. The renderer adds sources only to script directives, so `style-src` reaches the header exactly as you configured it.

## Streaming

A page whose load returns a promise streams the value later, in an inline script chunk. In nonce mode, each chunk carries the same nonce as the boot script, so streamed values fill in under the policy. In hash mode, chunks carry neither a nonce nor a hash, so the browser blocks them. This is Kit's own limitation, and SKGo has it too. Use `nonce` or `auto` on routes that stream; see [Loading data](load#Streaming-with-promises).

## Report-only policies

`reportOnly` takes the same directives as `directives`. Both policies see every inline script, so each gets its own correct nonce or hashes. A report-only policy that sets any directive must also name a non-empty `report-to` or `report-uri`. Otherwise `skgo.NewSSR` returns `csp.reportOnly must be specified with either the report-to or report-uri directive, or both` when the server starts, where Kit would throw on the first render.
