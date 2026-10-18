# #201 follow-up investigation

The earlier rejection was too conservative: it assumed suppressing the body in a new writer was necessary. The user has asked us to implement this issue now, delegating to Luna if a small fix is available.

The installed Kit pin remains 3.1.0. Map its current paths independently: `src/runtime/server/endpoint.js` selects GET for a missing HEAD; `src/runtime/server/instance.js` cancels the HEAD body and returns `new Response(null, response)`; `src/exports/node/index.js` sends a bodyless response without inferring a length.

A promising standard-library mechanism deserves testing before introducing body/status buffering. In the installed Go 1.27.2 source (`net/http/server.go`, `chunkWriter.writeHeader`), automatic Content-Length generation requires the header key to be absent. `net/http/header.go` explicitly defines presence to include a nil or empty slice. The HTTP/2 implementation also checks key presence (`net/http/internal/http2/server.go`). A nil header entry can therefore suppress inference without emitting a wire header. Verify this on real servers rather than assuming it works. Preserve explicit lengths and account for a handler deleting the header before writing, as `http.Error` does.

The earlier PR #202 dropped writes and lost normal Content-Type sniffing after `WriteHeader(200)`. Do not repeat that failure. The desired proof is literal real-server responses: HEAD has no body and no inferred length; an explicit length survives; Content-Type matches normal GET for both implicit and early explicit status. Explicit HEAD still wins; GET, flushing and panic behavior stay as they are. Existing endpoint tests already cover dispatch, flush and panic behavior.

This is an ordinary endpoint compatibility fix with Go tests, not a new general-purpose response layer. If the standard writer can keep doing its existing work, let it.
