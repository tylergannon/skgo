# Mission: the example front page indexes every capability, plus two missing fixtures (#63)

Capability when done: a visitor landing on `/` of the example app sees what
skgo does, one entry per capability, each linking to the page that
demonstrates it, with a one-line statement of what to look for. And the two
paths #63 names have fixtures and load-bearing scenarios. Closes #63.

## The front page today

`example/web/src/routes/+page.svelte` (16 lines): "Home", `Greeting`,
`getSite()` in a boundary, `data-testid` hooks `title`, `site-name`,
`colocated`, `site-failed`. `routes.feature`, `app.feature`,
`colocation.feature` and `ssr.feature` assert on this page; keep those hooks
or update those steps. Nav is `+layout.svelte`:26-35 (ten links).

The capability list is the route table in `README.md` in this directory; the
live and batch pages (`/live`, `/batch`) are landing concurrently from the
live-batch mission and belong in the index if they are on main when you open
the PR (rebase first; the nav lines will conflict trivially).

Svelte 5 / kit 3: `await` in markup inside `<svelte:boundary>`, `#lib`
imports, no `svelte.config.js`. Use the Svelte MCP autofixer on any component
you write.

## Fixture 1: a load that fails in the root layout takes the static error.html path

Go side: `document.go` `staticErrorPage`:677 and `serveLoadError`:333
(`nearestErrorPages`:706, `buildErrorChain`:727 decide when no `+error.svelte`
can render and kit's static `error.html` is served; the template is read by
the adapter, `readErrorTemplate`:662). Kit: `runtime/server/page/respond_with_error.js`
and `runtime/server/page/index.js` (root layout load failure has no error
boundary above it, so kit serves `error.html`).

The example has no root `+layout.server.ts`; `+layout.ts` is universal.
Adding a root server load that fails on a trigger the scenario controls (a
cookie or query parameter the test sets, never something ambient) gives the
fixture. The scenario must assert on content that only `error.html` has
(`example/web/src/error.html` if present, otherwise kit's default template)
so a rendered `+error.svelte` cannot satisfy it.

## Fixture 2: a transported type in a remote answer during SSR

`/(marketing)/pricing`: `+page.svelte`, `+page.server.ts`, `page.server.go`,
`pricing.remote.{go,ts}`, `types.ts`. Transport hook: `example/web/src/hooks.ts`
and `hooks.go`; skgo side `transport.go` (`reducers`:128, `revivers`:197),
`transport_encode.go`; `transport.feature` covers the browser wire. Today the
plans list sits in a pending boundary so `getPlans` is never called during
render. The fixture is a render-time call (top-level `await` in the component,
or a load) whose value is a transported type; the scenario must prove the
value in the *server-rendered* markup (fetch the document without JavaScript,
or assert before hydration) so a client-side revive cannot satisfy it.

## Collisions

Live-batch mission edits `+layout.svelte` nav and `todos/`; adapter mission
does not touch example routes. Rebase before the PR.
