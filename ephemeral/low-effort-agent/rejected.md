# Original batch exclusion

- [#201 — Match Kit HEAD fallback Content-Length behavior](https://github.com/tylergannon/skgo/issues/201): the earlier real-server probe and closed PR #202 exposed inferred Content-Type loss after explicit WriteHeader. A correct fix needs deferred response status plus flush/panic behavior, so this is not a simple mechanical change. The current endpoint fallback still directly invokes GET. Keep open.

Correction after PR #377 merged: the user challenged this judgment. A present nil Content-Length header lets net/http suppress inference while preserving its normal writer behavior, avoiding the assumed buffering work. Luna implemented the six-line fix on `codex/issue-201-head-fallback`; its real HTTP/1.1 and HTTP/2 regressions and Sonnet review pass. The original rejection is superseded by that follow-up.
