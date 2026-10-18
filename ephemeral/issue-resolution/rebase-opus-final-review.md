# Final rebase validation: PASS

Both repaired tests pass on one run, with zero skips (rebase-opus-final.log). The only production-file diff is the earlier gofmt of remote_live.go.

- TestBuildRouteEntriesBeforeRequestOwnership: the fixture now supplies `ctx: t.Context()`, as the real constructors do. All four subtests (catalog, empty, failure, panic) still assert their exact body and status.
- TestRemoteSchemaValidationDefaultsAndClassification:
  - The schema-refusal subtests are unchanged.
  - `BadRequest` and `Errorf` errors now reach the hook as kind `app` with their public status and message. A plain Go error reaches it as kind `unknown` / 500 / Internal Error with the original error attached.
  - The hook's `reference` field reaches the response envelope, and private text never does.
  - Form issues bypass the hook.
  - This matches pinned Kit 3.1.0 `handle_error_and_jsonify` and #354.
- The test is load-bearing. A /tmp `-overlay` that reverts `callError` to main's validation-only `remoteError` fails at remote_validation_test.go:139 with `hooks=0, want 1` (rebase-opus-final-mutation.log). No source was edited.

The earlier real-build, handler and entries results (rebase-opus-gen-production-options.log, rebase-opus-adapter-entries.log, rebase-opus-root-remaining.log) are reused, not rerun.

Non-blocking note, outside #354's scope: the new `?payload=malformed` assertion locks in skgo's existing answer, a 400 Bad Request that skips the hook. Main already behaved this way and production did not change. Pinned Kit parses the payload inside the `try` that calls `handle_error_and_jsonify`, so Kit would give kind `unknown` / 500 and run the hook. The test comment's claim that malformed transport "never reaches ... its policy" describes skgo, not Kit.
