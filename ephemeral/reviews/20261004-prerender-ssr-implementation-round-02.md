# Production SSR prerender artifact reuse — implementation review, round 02

Outcome: **material findings remain**.

Reviewed immutable **c7fb92a85af8415af7bf094a373ca97cae13ba6c** in `/Users/tyler/Codex/2026-10-03/task-8/skgo-prerender-ssr`, using committed source rather than concurrent private proof mutations. I inspected the complete implementation from baseline **7803f6439ad4200c560f8482501f5a19467e51d3**, the changes since round one's **765b58b**, and the focused **07fd..c7fb92a** repair. The earlier immutable reviews remain unchanged. I performed the review myself, with no delegation, implementation edits, commands running tests/builds/servers, commits or network calls; this new artifact is my only write.

Authority remains `AGENTS.md`, agent-protocol and adversarial-review, the committed SSR-artifact worklog, both cleared design rounds, preceding extension boundaries and the original bounded user goal. That goal is production SSR reuse of built artifacts for existing Go prerender declarations, preserving client priority, raw keys, result transport, original hydration `p` entries, opaque unbuilt refusal, development execution, already-handled error bodies and native redirect-as-undefined behavior. Site alignment is R-remote-44 only. The excluded inputs/dynamic/parameter/HTTP-interception/URI-decoding implementation slices were not used to exclude material findings. No desired outcome was adopted.

## Evidence and prior finding

I rechecked the new store and both constructors, complete relevant runtime diff, result and argument transport boundaries, production/dev wrapper selection, error signal across adapter entry/Goja/Go document handling, pooled-runtime cleanup and document hydration assembly. All maintained generator, handler and public-consumer test changes were read. The final formatting changes in the generator test and BDD step do not change their behavior.

The candidate's installed primary Kit package is **3.0.0**. Relevant source authority includes `runtime/app/server/remote/prerender.js:69–125`, production replacement in `exports/vite/build/remote.js:72–89`, native remote caches and client hydration, `exports/internal/shared.js:3–24`, `runtime/server/errors.js:20–24`, required `App.Error` fields in `types/index.d.ts:3983–3986`, and the new failure-path comparison below. The Go devalue parser and result revivers were rechecked; static syntax validation continues to require no application transport decoding.

**Round-one malformed-field finding is closed for this target.** `prerender_artifact.go:117–136` now requires a numeric status and string message, including their presence, without an extra HTTP range rule. `decodePrerenderArtifact:269–276` uses the existing `ssr.Error.UnmarshalJSON` decoder rather than defaults, unchecked assertions or numeric truncation. Valid extras survive. Static accepts numeric wire syntax; SSR refuses fractional or otherwise unrepresentable integer statuses with the artifact identifier. This is an explicit supported-Go representation boundary, not a claim of native integer/range rejection. Native fractional-status document parity remains unproved and unclaimed.

Actual `error-envelope-focused.log` establishes maintained constructor failures, a valid 409 exact-wire/native-marker pass and malformed-client opaque 500/calls zero. Independent `final-corrupt-error-construction.log` replays the exact previously failing body `{"status":"409","message":7,"marker":"corrupt"}`, both missing fields, each missing field and wrong type separately. Both constructors now refuse with the recorded artifact path, and the test exits zero. Its fractional case separately shows static acceptance and SSR integer-decoder refusal. The old failure remains preserved in `candidate-corrupt-error-original-gap.log`. The new guard-removal replay was still pending when this review completed; it was not assumed to pass.

## Finding

### 1. Issue — an uncaught built error loses its original prerender hydration entry during Go error-page recovery

**Requirement:** the authorized worklog and cleared design require original `p` keys/results to survive production SSR reuse, including a built error's already-handled body. The successful component-boundary case does not cover an error escaping an actual universal load.

**Source cause:** `document.go:1441–1443` records the built error under its original `p` key and returns the handled signal correctly. `renderPlan:1254` returns those answers with the render error. But `deliver:1053–1055` discards the answers on error; `failed:976–978` calls `respondWithError` without them. `respondWithError:943` obtains a fresh answer map from a fresh render and assembles only that map at line 955. The earlier answered prerender is consequently absent from the emitted error document. Correct HTTP status and hook bypass do not recover this data.

Native Kit retains the same request state: `runtime/server/page/index.js:272–300` passes it into the load-error response; `page/respond_with_error.js` likewise carries state through error recovery; `page/render.js:510–514` collects remote data. `remote-functions.js:434–494` serializes the settled implicit prerender rejection under its original key and invokes the already-handled error fast path to preserve the body.

**Actual reproduction:** the independently authored and compiled fixture completed a real `vp build` in `uncaught-real-build.log`. Its ordinary Go handler and native Kit renderer then request `/uncaught`, whose universal load awaits the supplied valid built error. `uncaught-original-oracles.log` records:

- Go: HTTP **409**, hook calls **0**, remote Go calls **0**, but the literal assertion for `"1qtjfmp/builtError/":{e:` fails.
- Native: the corresponding uncaught, unrelated-error and ordinary-runtime cases pass; `/uncaught` is **409**.
- The later Go `/unrelated` is **500** with hook calls **1**, followed by ordinary `/runtime` **200** without another hook call. These observations do not erase the first response's payload failure.

I read the actual saved response files. `uncaught-native_uncaught.html:34` contains:

```js
__sveltekit_19f0kmy.data = {p:{"1qtjfmp/builtError/":{e:{status:409,message:"built handled message",marker:"receipt17"}}}};
```

`uncaught-go_uncaught.html` has the correct visible 409/message and boot `error` marker, but **no remote data/p assignment** before `kit.start`. The independent Go test fails while the native control passes. This is an observed payload mismatch on the newly supported built-error path, not a hypothetical failure or missing-toolchain skip.

**Impact and bounded correction obligation:** preserve the answered built prerender entry across recovery and merge it into the error document under the original key, with the same already-handled body. Keep the existing body-counter zero and hook bypass; do not replace the body, rerun Go as fallback, normalize the key or turn the repair into general unrelated error architecture. Add a maintained contract at the real compiled handler that distinguishes this uncaught-load path from a Svelte component boundary. The corrected original must pass before its corresponding isolated removal controls can establish that this recovery behavior is load-bearing.

## Proof disposition and remaining limits

Raw receipts are retained under `/Users/tyler/Codex/2026-10-03/task-8/prerender-ssr-logs`. The original successful literal/transport/client-priority/raw-key controls and viewed scripting-on/off pages remain valid evidence for the unchanged success paths described in round one, rather than claims about every new error path.

I read the actual saved removal sources and outputs. Removing artifact lookup, normalizing the supplied raw key, dropping result transport, reversing client priority, disabling SSR semantic decoding and disabling shared wire validation each causes the corresponding independent assertions to fail. Removing hydration `p` data also fails the compiled document's literal-key assertion. These are materially stronger than a batch process exit zero.

Conversely, `remove-built-error-classification.log` and `remove-handled-error-hook-fast-path.log` both show the **component** oracle passing after their respective removals. They are retained unsuccessful proof attempts, not proof of those mechanisms. The new uncaught case establishes correct original status/body/hook observations but is itself red on the missing `p` entry; a mutant's aggregate exit one cannot by itself make that already-failing combined original load-bearing. Corrected original-plus-negative controls, explicit pool reuse evidence and the actual hydration-browser removal remain pending, not inferred future passes.

The earlier candidate production native report was independently parsed in round one: 175 expected/passed, no skipped/unexpected/flaky tests, no retries and no report errors. Its actual browser screenshots were opened and show `build:atlas`, `$23.00` and `build:noarg`; it is not a substitute for new error-path proof or repaired-head development/main/CI qualification.

Current `repaired-vet.exit` is zero. **`repaired-test.exit` is one**: `repaired-test.log:122–123` reports the existing freshness oracle, because `web/src/app.html` was modified at 05:09:54 after the embedded build at 05:04:16 and instructs `just build`. This is an actual qualification precondition failure, not a new demonstrated runtime defect. No reviewer mutation or weaker oracle was used to bypass it, and the earlier candidate test pass was not substituted for a current pass. A fresh intended build and ordinary check remain required.

No additional material defect was established beyond the demonstrated uncaught-error hydration loss. The broader explicitly unclaimed capabilities and retained URI-decoding limit remain outside this release claim. The old malformed-field issue is repaired, but the new observed payload failure leaves **material findings remain** against immutable c7fb92a. Subsequent corrections and completed qualification require a new immutable review; this report does not predict their result.
