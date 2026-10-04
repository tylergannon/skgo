# Declared prerender Inputs adapter review — round 04

Outcome: **material findings remain**.

Review target: immutable checkpoint `80e05ea273423be6b482dcb45452c7115edf38f4`, the locked Inputs worklog/design and its seven acceptance groups. Line references below are to that checkpoint, not subsequent edits. This is not a completed-feature or merge verdict.

## Evidence and limits

Applied the repository agent-protocol/adversarial-review instructions already read in this session. Inspected the complete new process-owner/helper implementation and adapter wiring, the Go body dispatcher and panic classification, generated TS/JS callback typing, package declarations/exports, inert engine resolution, checkpoint tests, and corrections to the round-03 generator findings. Revisited the installed Kit 3.0.0 and VitePlus 1.0.0 sources mapped in earlier rounds, especially native enqueue, error classification/crawler policy and application build hooks. Checked Node's documented child exit/close ordering.

Read the independent native test source and actual retained `native-final.log`, `native-error-origin.log`, initialized/stock/bootstrap-removal receipts, and `native-fixtures/error-origin/{origins,http,policy}.log` under `../prerender-inputs-proof`. These establish reference behavior, not correctness of the Go bridge. No additional test/build was run by this reviewer; the reproductions below are detailed causal cases from source. No product, test, dependency, worklog or previous artifact was changed. Only this immutable review was written. No requested focus or implementation-stage boundary was treated as a restriction on material defects.

## 1. Issue: a terminal owner failure can still finish as a successful build

**Requirement:** malformed IPC and producer/infrastructure failure must fail the build regardless of native policy for ordinary body HTTP errors (acceptance groups 5–6).

`internal/adapter/skgo-adapter/prerender.js:234–237` latches session.failed and drains. Worker-side errors then become ordinary thrown Errors in Kit. The application wrappers at lines 443–449 only consult the owner in their catch branch; they return a successful native handler result without checking the latched failure. `skgo-adapter.js:242–247` likewise only awaits cleanup in adapt's finally. Cleanup itself does not throw session.failed.

**Causal reproduction:** use an argument-taking remote with one declared input, a native handleHttpError policy that ignores 500, and a Go child fixture that returns malformed remote output. The owner detects the bad envelope, closes/drains, and rejects the worker invocation. Kit transforms that thrown Error into a 500 and the permissive crawler policy accepts it. There need be no later Go request. The native worker can return successfully; compile/finalise/adapt then return successfully despite the latched infrastructure failure. The same hole covers worker-reported devalue decoding failure.

**Required correction:** successful application completion must also observe a latched owner failure, await its cleanup and reject. Preserve native policy for structured application HTTPError/Redirect responses, which must not latch owner failure. The distinguishing proof uses permissive native HTTP-error policy: a structured 500 may complete according to that policy, while malformed IPC still rejects the outer build after cleanup.

## 2. Issue: draining descendants only after stdio close can hang forever

**Requirement:** the owner must terminate its process trees and settle without hanging, including normal completion with zero live children and no private compile directory.

`prerender.js:184–190` starts descendant termination inside the child's close handler. There is no child exit handler. Node emits close only after the process has ended **and its stdio streams have closed**; descendants may share those streams, while exit occurs earlier. See [Node 24 child close/exit semantics](https://nodejs.org/download/release/v24.21.0/docs/api/child_process.html#event-close).

**Causal reproduction:** a Go invocation starts a long-lived child with `Stdout = os.Stdout` and `Stderr = os.Stderr`, writes a valid answer and exits zero. Its descendant remains in the owned process group holding the pipes open. The direct child's exit event occurs, but close does not; stopGroup is never reached, job.done remains pending and the build hangs. No failure or signal need arrive to activate the separate cleanup escalation path.

**Required correction:** begin descendant termination after direct-child exit, independently of pipe close; completion still joins stream close, tree drain, validated output and the original exit status exactly once. A Go-owned fixture must leave inherited pipes open after the leader exits, prove the literal answer/exit behavior, and verify descendant count zero before job/build completion.

## 3. Issue: valid devalue with the wrong response shape is accepted as success

**Requirement:** malformed IPC must never turn into a successful build or missing declared artifacts (group 5).

The main envelope check at `prerender.js:46–50` only requires serialized data to be a string. `remoteInputs` at lines 583–586 returns the parsed value without requiring an array. A response `{"inputs":"[null]"}` is valid JSON and devalue; it parses to null. Native Kit `core/postbuild/prerender.js:713` replaces that null with an empty list, so an argument-taking remote silently produces no declared artifacts and can finish successfully. A noarg declaration can similarly ignore a malformed decoded collection.

Likewise, `remoteFunction` at lines 658–664 accepts `{"type":"result","data":"[{}]"}` and reads a missing `_` as undefined. It therefore publishes a successful undefined value even though the required result member was absent. These values are malformed protocol responses, not valid empty producer output or an explicit encoded undefined result.

**Required correction:** require the revived Inputs value to be an array; require a revived result envelope with its own `_` member. Invalid shapes must use the fatal-owner drain path and remain fatal through the application boundary from finding 1. Add distinct wrong-shape controls alongside syntax/devalue-reference corruption controls; JSON.parse/devalue.parse success alone is insufficient.

## 4. Issue: unknown Go error diagnostics are lost before native handleError

**Requirement:** body errors must retain native error origin and native handleError/crawler semantics, while unexpected errors retain the opaque default public 500.

`prerender_remote.go:135–145` correctly distinguishes app versus unknown, but both are serialized only through asHTTPError. An ordinary `errors.New("private detail")` becomes `{status:500,message:"Internal Error"}` before it reaches the worker. `prerender.js:660` then constructs `new Error(answer.error.message)`, so the native hook/operator receives "Internal Error" instead of the original error message. The build-specific panic path at `prerender_remote.go:161–166` similarly preserves origin but converts the value to the sanitized recovered error before any private diagnostic is captured.

The inspected independent reference is decisive: `native-fixtures/error-origin/origins.log` contains `unknown:none:native private failure`, while its HTTP receipt contains the hook's explicitly selected public message. Native `runtime/server/errors.js` supplies the raw unknown Error to hooks.handleError and independently chooses opaque 500/Internal Error as fallback. The checkpoint test asserts only the collapsed kind/status/public-message fields, so it cannot detect lost hook information.

**Adjudicated bounded correction:** an unknown-only private IPC diagnostic field is appropriate and within this slice. Capture the original ordinary error text (and original recovered panic diagnostic before sanitization), retain the separate public fallback fields, and reconstruct the worker's generic Error from the diagnostic. Do not copy the diagnostic into the HTTP envelope automatically. Tests must separately assert the native hook's original private message, its deliberate transformation, and the unchanged opaque public fallback when the hook does not override it. This preserves native behavior without adding a public error API or changing runtime Go error handling.

## Boundaries confirmed and remaining proof

The physical control-class hypothesis is resolved by evidence: actual compiled native output imports @sveltejs/kit/internal externally, and the independent physical-HttpError reference reaches kind app and native transformation. Fresh HttpError and native Redirect in this checkpoint are appropriate; HandledHttpError would wrongly bypass the hook. Public redirect adds external-location permission behavior and is not required for this bounded repair.

The worker-side native transport bootstrap remains source-backed and supported by the retained stock-negative, initialized-positive and bootstrap-removal controls. Canonical remote keys remain Kit's; body IPC now uses ordinary transport serialization. Main carries serialized bytes, with app transport revival/encoding in the worker. The round-03 Go generator corrections use transported input reachability, omit fmt for noarg-only producers and allocate unique Inputs names. The TS helper is generic and generated JavaScript supplies its concrete callback return type; those source changes do not establish real JS/TS frontend qualification on their own. Engine resolution maps the helper to the inert module rather than importing its Node dependencies.

The checkpoint's adapter smoke explicitly adds page calls for both values and checks aggregate artifacts by substrings. It cannot establish the no-page-use/removal acceptance: removing declared Inputs may leave page discovery producing the same data. This is a limit of the smoke, not a completed-feature claim. Required independent declared-only literals, duplicate call counts, Money through the real Go helper, producer/generation/runtime counts, permissive/default native policy, programmatic failure-before-adapt, native cancellation/process-tree/pipe/compiler controls, generated TS/JS builds and existing SSR/static/load regressions remain necessary. None of the native-only reference receipts substitutes for those implementation proofs.
