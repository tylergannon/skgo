# Rejected from this batch

- [#201 — Match Kit HEAD fallback Content-Length behavior](https://github.com/tylergannon/skgo/issues/201): the earlier real-server probe and closed PR #202 exposed inferred Content-Type loss after explicit WriteHeader. A correct fix needs deferred response status plus flush/panic behavior, so this is not a simple mechanical change. The current endpoint fallback still directly invokes GET. Keep open.
