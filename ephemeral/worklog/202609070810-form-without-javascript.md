# A form remote function without JavaScript (#39)

## The issue text is wrong about the slot, and the comment correcting it is right

Verified against the pinned kit source rather than taken on trust.
`handle_remote_form_post_internal` (`runtime/server/remote-functions.js`)
returns `{type: 'success', status, location}` with no `data`;
`render.js:97` computes `form_value` from `action_result.data`, so it is
`null`, and `uneval_action_response` at `render.js:485` is behind
`if (form_value)` and never runs on this path. It belongs to the classic
`+page.server.js` actions, which skgo has no equivalent of.

The one thing the comment gets wrong is the URL. `get_action_location` produces
the `location` field of a `ServerActionResult`, and `render_page` reads that
field only on the redirect and JSON arms — a successful or refused remote form
post is rendered at the request's own URL, `?/remote=<id>` and all. The form's
`action` getter is what strips the parameter, on the next render.

## `state.remote.data` is the only way a submission can reach the render

Kit's form instance reads `result`, `fields.*.issues()` and every control's
value out of `get_cache(__, state)['']`, keyed by the *internals object* — not
by a string. Go has only the id, so `$app/server`'s `form` wrapper in the SSR
bundle now keeps every instance it creates and the entry finds the one whose
`__.id` matches before it renders. There is no substitute: no host binding is
called for a form, and `page.form` is the wrong slot by kit's own design.

This is the reason the run had to touch `internal/adapter/skgo-adapter.js`
despite being told not to. Two constants, `SSR_APP_SERVER` and `SSR_ENTRY`;
transport encoding is untouched.

## A non-enhanced form post needs an empty refresh set, not no refresh set

`skgo.RefreshRequestedNoArg(ctx, q)` fails with *"a client's refresh request can
only be accepted by a command or a form"* when `refreshSetFrom(ctx)` is nil, and
`serveForm` sets one from the request's `x-sveltekit-remote-refreshes`. A
browser posting a form on its own sends none, so the event needs
`newRefreshSet(rs, nil)` — an empty set — or every handler that ends
`return out, skgo.RefreshRequestedNoArg(...)` answers 500 on the non-JS path.
The symptom is the app's own error page at 500 with nothing in the server log,
because it is an ordinary handler error and not a render failure.

## An empty `<input type="file">` is a multipart part, and `part.FileName()`
## cannot see it

A browser submits an untouched file input as a part with `filename=""` and no
bytes. `mime/multipart`'s `part.FileName()` returns `""` both for that and for a
part with no `filename` parameter at all, so a file control that was left alone
arrives as a *string* and lands on a `skgo.File` field as a type error. The
presence of the `filename` parameter is what distinguishes them:
`mime.ParseMediaType(part.Header.Get("Content-Disposition"))` and a
`_, ok := params["filename"]`. `example/form_noscript_test.go` posts exactly that
shape.

## A `<textarea>` does not come back filled without a script, and that is kit's

`fields.body.as('text')` returns a `value` property, which Svelte spreads onto
the element as an attribute. A `<textarea>`'s value is its content, not its
`value` attribute, so the server-rendered markup carries the text where the
browser ignores it. With scripting on, Svelte assigns the property during
hydration and the message reappears — visible in
`ephemeral/screenshots/form-noscript/prod/04-*.png`, where the textarea holds
"nope". The noscript scenario therefore asserts the two `<input>`s only;
claiming the textarea would be a scenario that fails for a reason skgo cannot
fix.

## Kit refuses a form POST that carries no Origin header at all

`is_csrf_forbidden` is `request_origin !== self_origin && (!request_origin || ...)`,
so a missing Origin is forbidden, not exempt. Mirrored. It matters for the
tests: an `httptest.NewRequest` with no Origin gets 403, which reads like a bug
until you find that line.

## A JavaScript-off scenario is a Playwright *project*, not a step

`javaScriptEnabled` is a context option. `example/e2e/playwright.config.ts` now
has two projects — `chromium` with `testIgnore: /form-noscript/` and `noscript`
with `javaScriptEnabled: false` and `testMatch: /form-noscript/` — over the
spec files bddgen writes into `.features-gen/<mode>/`.

`HTMLFormElement.submit()` is how a JavaScript-*on* browser makes the same
request: it fires no submit event, so kit's `enhance` never sees it. But it
returns before the request is issued, and `page.waitForLoadState('load')` on a
page that has not navigated yet resolves immediately — the step passed while
asserting nothing had happened. Arm a `page.waitForResponse` for a document POST
*before* calling it.

## The suite's todo list accumulates across runs against one server

`routes.feature`'s "The endpoint accepts a POST and keeps what it was given"
asserts `toHaveCount(1)`, and the fifth run against one `just serve` process
sees five. Restart the server between runs, or read a failure there as arithmetic
rather than as a regression. (Pre-existing; not touched here.)

## `rm -rf ephemeral/screenshots` deletes 111 tracked files

They are tracked evidence, and the current suite regenerates only the subset the
`shot` fixture names. `git ls-files -d -z -- ephemeral/screenshots | xargs -0 git
checkout --` puts back exactly what was lost without overwriting the fresh
frames. `example/e2e/screenshots/` is the gitignored one.
