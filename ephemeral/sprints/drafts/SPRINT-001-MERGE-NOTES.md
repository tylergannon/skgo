# Sprint 001 Merge Notes

## Claude Draft Strengths
- `all:` embed prefix with a unit test guarding it; content-hash ETags (embed.FS has no mtimes); route-aware 200/404 for the boot document; honest list of unverified items (playwright-bdd API, HMR through proxy).

## Codex Draft Strengths
- No placeholders: a fresh checkout cannot `go build` the example until `vp build` ran, by design. `httputil.ReverseProxy.Rewrite` + `SetURL/SetXForwarded`; raw-TCP upgrade test; MapFS unit tests; `X-Skgo-Mode` response header as proof the document crossed Go; `go.work`; `precompress: false`; Playwright run by plain commands.

## Gemini Draft Strengths
- Complete file bodies: package.json pins, vite.config.ts, tsconfig, Procfile, .air.toml, four Gherkin features and step definitions.

## Consensus Critiques (multiple agents agreed)
- Prerendered path is `build/prerendered/index.html` (Claude wrong).
- `//go:embed` without `all:` drops `_app/` (Codex wrong).
- adapter-node deletes `out/` each build, so `.gitkeep` placeholders cannot survive (Gemini wrong).
- sirv's `W/"<size>-<mtimeMs>"` ETag is meaningless on `embed.FS` (Gemini wrong).
- A layout link to a 404 path fails the prerender crawl (Gemini wrong).
- `go.work` is needed for `go build ./example/cmd` from the root.
- No Go-driven Playwright harness (three of four critiques; AGENTS.md).

## Valid Critiques Accepted
- All of the above. Also Claude's critique: the kit-sanctioned way to get the boot document is `builder.generateFallback`, reachable only from an adapter.

## Critiques Rejected (with reasoning)
- Codex critique's "keep Claude's Go harness": forbidden by AGENTS.md and the brief; Playwright is run by documented commands.
- Codex critique's "keep Gemini's placeholders": adapter output directory is wiped on every build; the fail-loud policy is better.
- Gemini draft's `.br/.gz` negotiation and Claude draft's mrmime generator: deferred; not needed to render routes.

## Interview Refinements Applied
- Tyler, after the drafts: prerendering routes is the wrong tool (routes with Go server loads must never be prerendered, or kit bakes/executes their loads at build). **Write a thin skgo adapter** that emits the client bundle, the boot document via `generateFallback`, and a small JSON manifest. adapter-node is dropped; the "prerender the root layout" trick is dropped.
- Tyler: minimal working setup per the requirements; no static-serving parity work; unknown-route status is not a product concern (Go serves the boot document; with the manifest at hand Go returns 404 for unknown routes, otherwise 200).
- Tyler: overmind + Procfile for dev; playwright-bdd in `example/e2e`; prod = `go build` the example, run the binary with Vite stopped, browse.

## Final Decisions
- Adapter: `example/web/skgo-adapter.js` (~20 lines) — the only JavaScript beyond the app and the tests.
- Embed: `example/web/dist.go` with `//go:embed all:build`; fresh checkout must run `vp build` first.
- Library API: `skgo.NewDevProxy(target *url.URL) http.Handler`, `skgo.NewStaticHandler(build fs.FS) (http.Handler, error)` reading `index.html` and `skgo.manifest.json` from the FS.
- Proof: playwright-bdd scenarios against `BASE_URL`, run once against the overmind dev stack and once against the bare binary; `go test ./...` covers proxy and static handler with MapFS; the embed guard test lives in `example/`.
- Pins: junkyard-validated JS pins; `@playwright/test` 1.63.0; `playwright-bdd` 9.2.0.
