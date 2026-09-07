# Streaming a deferred load value into the document (#38)

## A backtick in a comment inside `SSR_ENTRY` silently ends the bundle

`skgo-adapter.js` keeps the SSR entry, the polyfill and the `$app/server`
substitute in `String.raw` template literals. A JSDoc comment written the way
every other comment in the file is written — with `` `{#await}` `` in it —
terminated the literal. The error surfaces two hundred lines later as a rolldown
`[PARSE_ERROR] Expected a semicolon`, in `bundleConfigFile`, pointing at the
vite config; nothing in it says "template literal". Backticks and `${` are
forbidden inside those three constants.

`internal/newapp/newapp_test.go` compares the template copy byte for byte with
`example/web/skgo-adapter.js`, so any edit is two files or a failing test.

## Svelte's server renderer never awaits an `{#await}`

`svelte/src/internal/server/index.js`, `await_block`: if the value is a promise
it pushes `BLOCK_OPEN`, calls `promise.then(null, noop)` and renders the pending
branch — settled or not, it never waits. That is the fact the whole slice rests
on. Go does not have to resolve anything for the engine; it only has to put *a*
promise where the value goes, which is one new field on `ssr.Node` (`deferred`,
the names of the fields) and three lines in the bundle's `node_data`.

It also means the engine cannot render a `:then` branch at all, which is exactly
kit's behaviour and not a limitation to work around.

## Kit's streaming branch drops the status and the etag, and it is not an oversight to fix

`render_response` ends:

```js
if (!chunks) headers.set('etag', `"${hash(transformed)}"`);
return !chunks
    ? text(transformed, { status, headers })
    : new Response(stream_text(transformed + '\n', chunks), { headers });
```

`status` is passed on one branch and to nothing on the other, so a streamed
error page is a 200, and a streamed document carries no etag to revalidate
against. Both are mirrored. A future change that "fixes" either one makes skgo's
document differ from the document kit's own client was written for.

Two more from the same function, both mirrored: the chunks are yielded in
*settlement* order rather than by id (`utils/streaming.js` gives a settling
promise the next free slot, not its own), and `get_data` is called outside the
`page_config.csr` branch — so whether a response streams is decided by the loads
and never by whether the page hydrates, csr = false included.

## Port 5173 was already taken by another worktree

`vp dev --strictPort` printed `Port 5173 is already in use` and exited, and
`lsof -nP -iTCP:5173` then showed a listener that looked like mine and was not.
The dev leg ran on 5199/8138 and the prod leg on 8137, all three killed by pid.
The "listening on" line prints before the bind either way.

## The adapter's fingerprint makes `just generate` part of every adapter edit

Since #47, `example/web/skgo-adapter.js` is written by `skgo generate` out of
`internal/adapter/skgo-adapter.js` and stamped with a fingerprint of what it
wrote (`const SKGO = { version, adapter }`). Any edit to the adapter source
therefore changes two things in the example's copy — the edit and the stamp —
and `TestNothingGeneratedWasWrittenByHand` compares byte for byte, so the copy
cannot be patched by hand at all. Edit `internal/adapter/skgo-adapter.js`, run
`just generate`, commit both. The note above about `internal/newapp/newapp_test.go`
comparing a template copy is stale: that template and that test are gone.

Rebasing this branch past #47 needed no manual conflict resolution — git
followed the rename of `internal/newapp/template/web/skgo-adapter.js` to
`internal/adapter/skgo-adapter.js` and the two hunks are hundreds of lines
apart.

## The fixtures' `final` frame is not written by a scenario that takes its own

`shot` writes `<slug>-final.png` only when a scenario finished having taken no
screenshot (`fixtures.ts`, `if (taken === 0)`). The streaming scenario names
five, so its `-final.png` is a leftover from before those names existed and the
suite does not rewrite it — its todo count is one ahead of the five. A frame in
`ephemeral/screenshots/` that the current suite does not reproduce is not
evidence of the current suite.
