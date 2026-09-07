# form-nojs: fixing the four validator nits

Branch `form-nojs` (PR #58). Task: fix four kit-fidelity nits the validator
found, rebase onto main, rerun suites, push.

decision: submittedInput's file-entry rewrite (`formdata.Entry{Name: entry.Name}`)
was defeating ConvertRaw's own `entry.File != nil` filter — passing the file
entry through unchanged is the fix, not adding a second filter. ConvertRaw
already had the right behavior (`TestConvertRawKeepsTextAsText` proved it);
the bug was entirely in the caller building a fake text entry ahead of it.

discovery: devalue's `uneval` (both the pinned JS source and skgo's Go port)
keeps an object key whose value is `undefined`, rendering `key:void 0` rather
than omitting the key. Checked
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/devalue/src/uneval.js`
directly (`Object.keys(thing)` finds the key regardless of its value, and the
`default:` object-stringify branch always emits every key it found) before
writing the test assertion — do not assume "undefined" means "key absent."

friction: `just test` failed right after the rebase with "the built frontend
and this binary disagree about the remote functions" for the `/empty` route
main's #54 added. Cause: stale `example/web/build` from before the rebase.
Fix: `ORIGIN=... just build` before `just vet`/`just test`, matching the
Justfile's own comment ("build the frontend (do this first in a fresh
tree)") and the existing memory note about root-checkout gates — this
applies equally after a rebase in a worktree, not just on first checkout.

doc_bug: the Justfile has no recipe for the dev-mode Go server (only `just
dev` for vite and `just serve` for the embedded prod build). Running the dev
e2e suite requires manually starting `go run ./example/cmd -listen
127.0.0.1:<port> -proxy http://localhost:5173 -origin http://127.0.0.1:<port>`
alongside `just dev`, because playwright's BASE_URL always points at Go in
both modes (`example/e2e/playwright.config.ts`'s own comment says so). Worth
a `just dev-serve` or similar recipe so this isn't reverse-engineered every
time.

correction: `vp dev` binds `[::1]:5173` (IPv6 loopback) only, confirmed via
`ephemeral/worklog/202609061700-server-routes.md` — a `-proxy
http://127.0.0.1:5173` target gets a 502; use `http://localhost:5173`.

decision (fix #4 scope): "pin the re-render URL" has no literal URL anywhere
in the rendered document or boot script to assert on — checked the full
prod-mode document body and found no hostname/URL literal at all. The one
place kit's own behavior ties the render URL to visible output is the
RemoteForm's `action` getter (`runtime/app/server/remote/form.js`), which
recomputes `?<other params>&/remote=<id>` from `event.url.search` at every
render. Asserted on that (`action="?/remote=<id>"` surviving the re-render)
rather than inventing a URL echo that doesn't exist in kit. Noted for future
readers: with no other query params on `/contact`, this assertion doesn't
actually distinguish "URL threaded through correctly" from "URL dropped
entirely" (kit's getter produces the same string either way when the search
string was otherwise empty) — a fully adversarial version would need a
route accepting an extra query param to prove fidelity. Left as the literal,
low-risk interpretation of what was asked.

decision (fix #3 scope): only added `Allow: GET` to the `id == ""` 405 in
document.go (kit's `handle_action_request`/`actions.js:90-94`, exactly what
was flagged). Did NOT touch the parallel 405 in document_form.go's
`runFormAction` (`fn, ok := s.remotes.Lookup(id); !ok`), which maps to kit's
*other* `method_not_allowed_result` call site
(`runtime/server/remote-functions.js:551`, `handle_remote_form_post_internal`)
and has the identical missing-header defect — its own comment already
(wrongly) claims the header is set. Left alone because it wasn't in the
validator's four-item list; flagging separately rather than scope-creeping.

Rebase onto main was conflict-free (the two branches' owned files never
overlapped); only one screenshot needed regenerating
(`ephemeral/screenshots/dev/dev/...404.png`, invalidated by main's `/empty`
nav link). `just generate` produced no diff.
