# Rebase validation: codex/reliability-fixes on origin/main ea1d75a4

Verdict: two verified failures, both in tests that came from main's ea1d75a4 and that no rebased commit touches. The real builds pass.

## F1. TestBuildRouteEntriesBeforeRequestOwnership panics (nil context)

- Reproduce: `go test -count=1 -run '^TestBuildRouteEntriesBeforeRequestOwnership$' .`
- Expected: four subtests pass (main's Entries contract).
- Actual: `context.AfterFunc` panics with a nil pointer at prerender_service.go:104. The panic kills the package test binary (rebase-opus-root.log), so `just test` is red.
- Cause: the branch's prerender fetch work (#272) now calls `context.AfterFunc(sessions[0].ctx, …)`. Main's entries_test.go:29 builds `&prerenderRequests{options: …}` with no ctx. Both production constructors (prerender_service.go:50, dev_bridge.go:60) set ctx, so this is a fixture conflict, not a runtime bug.
- Capability check: running the same test with a /tmp `-overlay` that only adds `ctx: context.Background()` passes all four subtests (rebase-opus-entries-overlay-diagnostic.log). Entries survives.

## F2. TestRemoteSchemaValidationDefaultsAndClassification fails (remote_validation_test.go:118)

- Reproduce: `go test -count=1 -run '^TestRemoteSchemaValidationDefaultsAndClassification$' .`
- Expected (main's test): `BadRequest(...)`, `Errorf(400, ...)` and a malformed payload must not call `RemoteConfig.HandleError`.
- Actual: `{"error":{"message":"WRONG classification","status":400},"type":"error"}`. The hook ran.
- Cause: the branch's #354 change sends every production remote error through `handleErrorAndJSONify` (remote.go `callError`). Main's validation-only `remoteError` assumed the hook ran only for schema refusals.
- Spec: pinned Kit 3.1.0 `runtime/server/errors.js` `handle_error_and_jsonify` maps `HttpError` to `kind:'app'` and runs the hook, and `remote-functions.js` uses it on remote errors. #354's accepted goal requires the same. The branch's behaviour is Kit's. Main's assertion `hooks != 0` (and the WRONG check on non-schema errors) is stale and needs reconciling. Validation-kind classification itself survives: the absent/nil/partial/panic subtests, TestRemoteSchemaValidationHookCarriers and TestNativeSchemaValidationHandledOnce pass.
- Development is unaffected: dev_remote.go still sends non-validation errors through `loads.dataError`.

## Executed, all passing, zero skips

- rebase-opus-root-remaining.log: 23 top-level root tests, including the remote error policy, component boundary policy, schema validation (except F2), all three Reroute tests, prerender config and fetch destinations, native error hooks and DocumentError.
- rebase-opus-gen-production-options.log: TestProductionOptionsApplication, a real Kit build with both subtests.
- rebase-opus-adapter-entries.log: TestGoRouteEntriesDuringKitAnalyse, with success, producer and matcher subtests.

Worktree changes (gofmt of remote_live.go and three tests, worklog rename) are formatting only.
