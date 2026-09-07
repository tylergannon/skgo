# Handle is no longer a load concern (#51)

## kit runs `handle` once, outermost, independent of route dispatch

`runtime/server/respond.js`'s `internal_respond` resolves the route and calls
`hooks.handle({event, resolve})` before `resolve` ever branches on
data-request / remote-call / page / endpoint. It is the single top-level
dispatcher for every request kind kit answers, not a page-loads concern. skgo
had it wired the other way: `Handle` lived on `LoadConfig`, and only
`Loads.Intercept` ran it — so an app with zero server loads had to build a
`Loads` registry (with an empty `generated.Loads()`) purely to get `Handle` to
run at all, or `LocalOf` silently returned nothing.

## The fix: `Handle` is now its own middleware

`Handle` (the `func(ctx) error` type) gained an `Intercept(HandleConfig, next)`
method, independent of `*Loads`. It mounts outermost over whatever the app
has — `Remotes` alone, or `Loads`/`Remotes`/`Endpoints` together — and does
the asset-skip / refusal-shaping work `Loads.Intercept` used to do.
`HandleConfig` needs only `AppDir`, `Base`, `Version`, `OnPanic` — no `Origin`
or `Dev`, because the hook's event can only read cookies (never write them),
so the `secure` cookie default is moot there. `LoadConfig.Handle` and
`Loads.handle`/`runHandle`/`runHandleGuarded`/`refuse` are gone;
`Loads.Intercept` now only answers `__data.json`.

## Deliberate narrowing: no route info inside Handle

The old `Loads`-hosted `Handle` built its event via `loadRequest.event()`,
which left `e.load` non-nil, so `URL()`/`RouteID()`/`Param()` accidentally
worked inside a hook mounted with Loads (though for a remote call the "URL"
was the remote endpoint, not the page — already documented as wrong
elsewhere). Nothing in the codebase used this, and the doc comment never
promised it. The decoupled `Handle.Intercept` builds a bare `&Event{jar: ...}`
with `load` nil, so those methods now consistently return zero values
everywhere, matching kit's own restriction on a *narrowed* event rather than
silently working only when a Loads registry happened to be mounted. If a
future issue wants kit's full `event.route`/`event.url` inside `handle`, it
needs its own route-matching decoupled from `Loads`'s server-load table —
out of scope here.

## `ephemeral/screenshots/` was never gitignored

CLAUDE.md and prior memory both assert it is; `.gitignore` only had
`example/e2e/screenshots/`. Added `/ephemeral/screenshots/` to `.gitignore` in
this PR so the auth-scenario screenshots (copied there per the proof
requirement) can't land in a commit by accident.

## The e2e suite already screenshots every `Then`/`And` step

`example/e2e/steps/fixtures.ts`'s `AfterStep` hook photographs the page after
every outcome step, saved to `example/e2e/screenshots/<mode>/<feature>/<scenario>/`.
No ad-hoc screenshot script was needed for the auth scenarios — just copy the
already-produced PNGs and look at them.
