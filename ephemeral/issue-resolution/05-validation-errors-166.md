# #166 — application validation-error handling

Complete downloaded issue body and comments: [05-validation-errors-166.json](05-validation-errors-166.json).

## Readiness and mission

Ready to implement; no HITL product decision remains. An application chooses the public result when a remote argument fails its validation, before its Go remote body runs, consistently in native development and production. Preserve pinned Kit's remote wire envelopes and safe defaults. The issue's historical `handleValidationError` name describes the capability; Kit 3.1.0's actual API determines how it is mirrored.

## Read-only cache evidence and important naming correction

Retrieval followed `/Users/tyler/src/skgo-token-cache/index/README.md` → `index/upstream-docs/topics.md` / `index/upstream-docs/kit-advanced.md`, and `index/promise-audit.md` → `promise-audit-2026-10-17/facts/matrix/handle-error.md`.

- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/60-appendix/35-migrating-to-sveltekit-3.md:382-384`: **`handleValidationError` was removed in Kit 3**. Remote schema validation reaches server `handleError` with `kind: 'validation'`; the safe generic error and private issues are separate.
- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/20-core-concepts/60-remote-functions.md:1269-1303`: invalid command/query/prerender arguments produce generic 400 by default. Applications can inspect validation issues privately and choose a public response. This is an argument-schema refusal, distinct from a remote body's business/form-validation result.
- `repos/dev-experience-20261015/frameworks/kit-3.0.0-upstream/documentation/docs/30-advanced/20-hooks.md:157-192`: validation's default is `{status: 400, message: 'Bad Request'}` with separate issues; a hook can return an `App.Error` override, while omitted fields keep defaults. Do not turn decoder internals into the public response by default.
- `promise-audit-2026-10-17/facts/matrix/handle-error.md:5-7` records the older skgo limitation: page error handling has app/unknown kinds but no remote validation boundary. It is a source account, not a current execution verdict or authorization to expand this issue into all missing error categories.

Installed Kit is verified **3.1.0**. Its `src/exports/hooks/public.d.ts:18-69` confirms `kind: 'validation'`, safe error status/message, separate issues, and partial public overrides. There is no separate current `handleValidationError` export to copy. The builder must independently map invocation boundaries and wire behavior from the installed source, not simply wire the historical name to all 400s.

## Current seams to assess

`handle_error.go` exposes Go `HandleError` and `CaughtError` for page errors; the latter currently documents app/unknown kinds. `remote.go:408-433` wraps generated-decoder refusals in `badRequest` while preserving private details and unwrapping to safe HTTP 400. This existing error representation alone is not an application validation hook. An arbitrary application-returned `BadRequest`, a malformed remote transport request, a remote body's thrown `HTTPError`, and form `Invalid`/issues must not all be mislabeled as argument-schema failures.

Installed source mapping starts with `src/runtime/app/server/remote/shared.js` (`create_validator`), `src/runtime/server/errors.js` (validation classification and public merge), `src/runtime/server/remote-functions.js` (carrier envelopes), and form/batch/live paths where relevant. Native bridge and production dispatch must preserve the same intended application result, rather than applying the hook twice or replacing it with raw decoder text.

README still combines `handleValidationError`, reroute and init into an unsupported row; describe the actual Kit3-compatible capability and avoid claiming unrelated init support. Update affected documentation using the existing render contract if needed.

## Winning and proportional proof

Supply a deliberately invalid argument to a representative generated Go remote and an application hook that returns a fixture-chosen public message plus an extra identifying field. Assert the exact expected public result through the actual served handler and that the remote body was called **zero** times; use a counter initialized by the fixture, not a baseline sampled from the running code. A valid-argument control must call the body and return its independent positive receipt.

Cover the invocation boundaries discovered in pinned source, including default/no-override behavior, private issue availability, partial public overrides and safe hook-failure behavior where Kit requires them. Add negative controls demonstrating that ordinary application errors, malformed wire decoding, and form business issues do not accidentally enter the schema hook. Use shared focused contract cases across the remote carriers whose envelopes differ; do not invent a new validation engine or blindly run a whole browser suite for HTTP-only assertions.

Execute the invalid-argument/custom-public-result case in native development and built production. A focused Kit-client scenario is useful where needed to prove the chosen public result survives its remote envelope to the client; assert the visible supplied message/field and a valid control receipt. Ordinary status/body/default/boundary contracts belong in Go handler tests. Reuse the existing build/session across assertions.

Opus verifies the tests would fail if the hook were bypassed or the body were invoked, runs focused proof, and reports acceptance gaps or reproducible regressions only. Repeat affected checks after fixes, without a new proof runner, evidence manifest, raw-schema disclosure, general `handleError` expansion, or changes to unrelated remote business-validation semantics.

## Questions resolved

- **Separate historical hook or current Kit behavior?** Current Kit3 `handleError` validation category; the retained issue's goal stays valid despite stale naming.
- **Can the body validate after invocation instead?** No: acceptance requires rejection and application public customization before the Go body runs.
- **Should detailed decoder issues become public?** No: Kit keeps safe fallback and private issues separate; the application explicitly chooses any public fields.
- **Which transports and boundaries apply?** Installed Kit answers this; mapping and executable qualification are builder work, not a product-design interview.
