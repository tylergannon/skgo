# Mission: the handleError hook runs for `__data.json`'s errors too (#82)

Capability when done: a visitor who reaches a failing page by clicking a link
sees the same message and support id as one who reloads it. Kit runs the
app's `handleError` hook on both wires; skgo runs it only on the document.
Closes #82.

## Where it stands

- `handle_error.go` is the hook's Go side: `CaughtError`:24, `HandleError`:77,
  and the header comment explains kit's `handle_error_and_jsonify`. The hook
  is held by `SSR` (`document.go` `SSROptions.HandleError`:82, stored :148,
  consulted at :368 for a rendered document, jsonified at :1108-1139).
- `Loads`, which answers `__data.json` in `data.go`, has no access to the hook.
  `data.go`:174 is the comment that records the deferral. `writeNodes`:296 and
  `serializeNode`:373 write the error node; today it carries kit's default
  `{"status":500,"message":"Internal Error"}`.
- The example's hook is `example/web/src/hooks.go` beside `hooks.ts`; the
  failing route is `example/web/src/routes/error/unexpected/`
  (`page.server.go` fails with a message holding a connection string; the
  document shows "Something went wrong on our end." and `case-<n>`).
  Existing scenarios: `ssr.feature` and `dev.feature`, steps in
  `steps/errors.ts`. Every one of them opens the page as a document, which is
  why nobody saw this.

## Kit as the specification

`runtime/server/data/index.js`:101 runs `handle_error_and_jsonify` per node
whose load threw, and :136 again in the outer catch; the same function
`runtime/server/page/index.js`:275 uses for the document. Its body is in
`runtime/server/errors.js`. Whatever shape the hook's return takes on the
document (status, message, then the hook's extra fields) is the shape the data
wire's error node must carry, because kit's client (`runtime/client/client.js`)
reads the two identically.

## Design note, not a design

The hook is the app's, not the renderer's; both `SSR` and `Loads` need it.
Where it lives (on `Loads` with `SSR` reading it, on a shared option both take,
or elsewhere) is yours to choose from how the example's `server.go` wires the
two today. Keep the public surface one thing an app sets once.

## Proof

A scenario that reaches `/error/unexpected` by client-side navigation from
another page (the `Data` fixture in `steps/fixtures.ts` counts `__data.json`
requests, so the scenario can prove the wire it took) and asserts the hook's
message and a support id, anchored in the hook's own text, not read off the
document first. A Go test in `example/` that fetches `__data.json` directly and
asserts the error node's fields. Break it (skip the hook on the data path) and
run only that feature to see the scenario fail.

## Collisions

PR #79 (showcase) is landing concurrently and touches `ssr.feature` and the
front page; its entry for `/error/unexpected` was deliberately weakened to
what held on both wires, with a scenario proving the weaker claim. After #79
lands, rebase, strengthen that entry's sentence and its scenario to the hook's
message, and keep both green. PR #80 (adapter) does not touch these files.
