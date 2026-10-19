# Correct hash-router documents — #245

Source: https://github.com/tylergannon/skgo/issues/245

## Original body

Released skgo v0.17.0 (`b3aae63e4ddf98e282a7c3539e7288cd18ba2d2f`), adapter 0.17.0 and Kit 3.0.0 build a `router.type: 'hash'` app, but the embedded Go server still selects the requested pathname and renders its page.

The current complete project measurement independently demonstrates this without relying on the browser history timeout:

- Ordinary HTTP `GET /about` returns 200 HTML containing the authored `HASH_ROUTER_ABOUT_42` page literal.
- The document response for `/about#/about` has the same authored About SSR content. The fragment is absent from the HTTP request, so this is another `/about` document request.
- Each retained document body is 3,966 bytes. The independent app-template marker is `HASH_ROUTER_SHELL_42`; the shell contract also requires Kit's native start/app imports and no authored Home/About page content.
- Root client boot, native `#/about` navigation and Back work. Forward/reload time out, but attribution of that separate browser result awaits a stock Kit oracle; it is not needed to establish this Go document-handler defect.

Reproduction uses the actual released CLI and canonical VP build:

1. Generate a minimal TypeScript project with `skgo new -skgo-version v0.17.0 -module example.com/hash <owned-directory> -- --template minimal --types ts --add vitest=usages:unit`.
2. Set `router: { type: 'hash' }` in the existing `sveltekit({...})` configuration. Put `<h1>HASH_ROUTER_HOME_42</h1><a href="#/about">About</a>` in the root page, and `<h1>HASH_ROUTER_ABOUT_42</h1><a href="#/">Home</a>` in `src/routes/about/+page.svelte`. Add the independent shell marker in `src/app.html`.
3. Run `vp build`, compile and serve the ordinary embedded Go application on its owned origin, and request `/about` directly.

Expected: a hash-routed build serves its client app shell for document pathname requests, with SSR/prerender disabled under Kit's hash-router contract. JavaScript-off visitors receive that shell; the native client renders the hash-selected page. Preserve ordinary pathname-router behavior and Go-owned endpoint/static-asset handling. Include a real embedded-server regression with literal shell/page expectations and retain the existing native history check separately.

Public evidence: [complete measurement 37246997393](https://github.com/tylergannon/skgo-project/actions/runs/37246997393), source revision `4474bb45079e959c0d18af9c6fc074d4c9fa731f`. The source-check artifact contains `router-modes-artifacts/hash/http-about.body.html`, `http-about.headers.txt`, `browser-deep-about.body.html` and `browser-requests.json`. Existing [project matrix issue #121](https://github.com/tylergannon/skgo-project/issues/121) tracks both hash mode and the separate server-resolution refusal. This core issue is scoped to the independently demonstrated document-shell behavior; it adds no matrix cell and claims no repair.


## Comment by tylergannon — 2026-10-18T16:57:46Z

Source: https://github.com/tylergannon/skgo/issues/245#issuecomment-6252596862

**Triage — 2026-10-18**

Checked against current [`main` at `920b6aca`](https://github.com/tylergannon/skgo/tree/920b6aca738a65d6c046a67f4db2373596fafd03). Classification: `bounded-hard`. 

The current adapter manifest/document path still has no explicit hash-router shell selection, so the production document concern remains relevant after the development rewrite. Keep this scoped to the independently demonstrated pathname document-shell contract, preserving Go endpoints/assets and ordinary pathname mode. Browser Forward/reload attribution remains a separate oracle question; do not bundle it into the core fix.

<!-- skgo-issue-triage-20261018-920b6aca -->

## Cache annotations and execution contract

Cache root: `/Users/tyler/src/skgo-token-cache`; routed from `index/README.md` through `index/upstream-docs/kit-reference.md:70` and `index/promise-audit.md` to `promise-audit-2026-10-17/facts/matrix/hash-router.md`. Historical results are context, not current verification.

**Goal.** A hash-routed production build serves Kit's actual client shell for document pathname requests such as `/about`, without selecting or rendering an authored pathname page. SSR and route prerendering remain disabled according to Kit. Preserve ordinary pathname-mode documents and precedence/behavior of Go-owned endpoints and static assets.

**Cache findings.** `promise-audit-2026-10-17/facts/matrix/hash-router.md:7-19` preserves the exact distinction: native root boot/hash navigation/Back succeeded, ordinary `/about` documents wrongly contained `HASH_ROUTER_ABOUT_42`, and Forward/reload failed separately. Lines25-27 explicitly mark those results as old v0.17.1/Kit3.0.0 observations. Cached Kit contract `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/packages/kit/types/index.d.ts:2042-2053` says hash routing disables SSR/prerender and links begin `#/`. Neither the audit nor this issue authorizes new matrix work or a history fix.

**Pinned Kit3.1.0 mapping.** Verified installed package version. Current `example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:613-627` renders hash/fallback responses with `ssr:false, csr:true` and the root layout, so the shell contains native client boot rather than authored Home/About markup. `src/core/postbuild/fallback.js:30-44` generates that native shell through Kit's own server. `src/core/postbuild/prerender.js:156-173` special-cases hash mode by generating one fallback artifact and returning before the normal page crawler. Current skgo already calls `builder.generateFallback` to emit its SPA shell; no new JavaScript shell renderer is needed.

Keep two distinct upstream boundaries visible: `respond.js:137-139` returns404 for nonroot pathname requests to Kit's dynamic hash server; that does not authorize authored pathname SSR in skgo. The accepted issue explicitly requires skgo's adapter-hosted document fallback to return the client shell for document pathname requests. Kit also rejects framework server files in hash mode (`src/core/sync/create_manifest_data/index.js:281-283`) and disallowed universal page-option exports (`src/core/postbuild/analyse.js:73-81`). Preserve these restrictions rather than enabling a new hash-mode application-server feature. Existing Go endpoint/asset handling must continue to precede document fallback; test it without weakening Kit's build restrictions.

**Independent proof.** Use a real production Kit build with literal app-template shell marker `HASH_ROUTER_SHELL_42` and distinct authored Home/About page literals, compile/embed/serve it through the ordinary Go application path, and request `/`, `/about`, and a deep document pathname. Require correct shell status/content type, the independent shell marker, Kit's real start/app imports, and absence of both authored page literals. A blank response must fail. The fragment is absent from HTTP, so `/about#/about` is the same document request; do not turn fragments into server routes. Verify static assets retain exact fixture bytes and Go endpoint responses retain literal status/body, and retain a pathname-mode control that still returns its authored page. Reuse one real build per mode for all corresponding assertions. Parent may separately open the running box to check native client boot; Forward/reload attribution is explicitly excluded.

**HITL.** Executable now. Kit answers the rendering shape and restrictions; the issue answers adapter fallback hosting and the preservation boundary. No design interview is required. Browser history repairs, `router.resolution:'server'`, new matrix cells, or broader router architecture are outside this item.

**Independent parent browser inspection.** Opened the reused real embedded hash build at `http://127.0.0.1:4185/about#/about` through CUA. Native client boot displayed `HASH_ROUTER_ABOUT_42` and the Home link. Clicking Home changed the hash to `#/` while retaining pathname `/about` and displayed `HASH_ROUTER_HOME_42` with an About link. Parent inspected the rendered Home screenshot: normal page content and link, no blank page/error boundary. This complements the raw-document handler assertions; it makes no Forward/reload attribution claim.
