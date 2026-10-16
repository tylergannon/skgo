# SvelteKit tracing

correction: Kit's tracing boundaries are operation wrappers, not every application call. `sveltekit.remote.call` surrounds HTTP dispatch and refresh handling; a query called directly during SSR does not receive another automatic remote span. Live/request scopes finish when the Response is constructed, before an unbounded body completes.

decision: Native Kit development uses an ordinary Node SDK preload and Go uses an ordinary Go SDK provider. One Go provider cannot export native Node spans. The private callback carries existing span contexts and annotations; it is not a custom telemetry provider/export service. Production universal loads use synchronous in-process proxies to Go spans.

friction: Kit rejects recognized `src/instrumentation.server.*` in builds unless the adapter guarantees execution before application code. Keep that support flag unset; the non-reserved development preload survives same-source development/build and stays inert when `SKGO_DEV_ORIGIN` is absent.

correction: Kit's fetch closes over the original untraced event. Development fetch propagation must use the active Node OpenTelemetry context plus the request owner's root span, rather than reading `event.tracing` from that original event.

correction: Per-operation renderer span lookup must replace only the span on the engine's callback context. Returning a saved earlier context loses render-depth limits and cancellation ownership.

friction: macOS fixture roots need realpath normalization before native Kit discovery; otherwise mixed /var and /private/var roots can create /private/private paths. The first fixture run exposed this before any product assertions passed.

correction: Adversarial implementation review found that same-origin production dispatch intentionally clears request values; preserving renderer callback span context was insufficient. Carry only the active OTel span through that isolation boundary. Actual Go-originated and universal-renderer fetch exports now prove parentage, and an explicit incoming trace header can still override the inherited parent.

correction: A sanitized native callback HttpError misclassifies an unexpected Go error as known_error, and a returned endpoint 500 hides a panic from Kit's endpoint wrapper. Carry private original diagnostics separately from the already-handled public body, throw the corresponding native Error/HttpError, and map it back through Kit's normal handleError hook. Actual native/standalone exported spans check load/action/endpoint exceptions; public responses retain the custom Go error body and omit the private diagnostic.

correction: Second independent review found that Kit passes `HttpError.body` to handleError for known app errors, while passing the original Error object for unknown failures. Key the handled-body WeakMap by that actual hook input identity; keying every error by its thrown exception loses custom status/message/fields on known native errors. The real runtime fixture now exercises a known 418 whose Go hook returns public 429 plus custom fields, with a literal single-call counter.
