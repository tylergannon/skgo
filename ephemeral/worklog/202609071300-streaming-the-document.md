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
