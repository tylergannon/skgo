# Development Content-Length

correction: Two layers dropped the length in `skgo dev`, not one. The bridge
deleted it to keep public completion after Go cleanup (#348), and the traced
callback writer deleted it to declare its tracing trailer. A trailer also
disables net/http's measured length, so an unsized handler (`/api/todos`) lost
its length on the Go hop before Kit ever saw it. Kit's tracing is on in the
example app, so testing without `tracing.server` misses most of the bug.

decision: Keep completion ordering by holding the last body chunk of a sized
bridge response until cleanup returns, instead of removing the length. Unsized
responses are not held, so streams still stream.

decision: A traced callback is held the way net/http holds an unflushed
response (2048 bytes). Held to the end, its annotations fit in a header and
net/http measures it. Committed early, an explicit length travels as
`X-Skgo-Content-Length` and the Node bridge restores it. The `/response` hop
always flushes after its headers, so it always uses the private header.

trap: `TestNativeResponseWaitsForLogicalRequestEnd` is the guard for the #348
ordering. It now also asserts that Kit's `setResponse` sends the length; without
the chunk hold it fails with "response completed while end was held".
