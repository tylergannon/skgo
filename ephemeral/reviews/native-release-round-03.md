# Adversarial review: native release automation (round 03)

Outcome: **only nitpicks remain**

## Target

- Commits `0328c57c ci: verify generated consumer recipe and native calls` and
  `5a3218e3 ci: recover pending native drafts through authoritative lookup` on
  top of `77cf98d6` (worktree `native-release-public`; tree clean apart from
  session logs and this review).
- Authoritative requirements: `ephemeral/native-release-review-prompt.md`
  (unchanged since round 02), `CLAUDE.md`, agent protocol.
- The launch arguments again end with `--output
  ephemeral/reviews/native-release-agent-round-03.json`; treated as a harness
  flag, nothing written there. No instruction narrowed scope.

## Evidence inspected

- `git diff 77cf98d6 HEAD` for `.github/workflows/release.yml`,
  `cmd/skgo-native-release/{main.go,main_test.go,notes.go}`, and the proof
  directory; the new `releaseByTag` (`main.go:1026-1058`) and its three call
  sites (`ownedReservation`, `promote` ×3), the draft skip in `publishedPair`
  (`main.go:870`), the release.yml comment tying the job display name to the
  eligibility gate.
- `go test -count=1`, `go vet`, `gofmt -l` on the package: 18 tests pass,
  clean.
- Test fixture semantics re-checked against GitHub's contract: in
  `TestPromotionPublishesOriginalBytesAndRecoversNotification` the
  `/releases/tags/...` mock now 404s while the release is a draft and only the
  `/releases` list serves it (`main_test.go:267-280`); the create mock returns
  an error after a partial (one-asset) draft, and the retry path uploads only
  the four missing assets with byte comparison, then publishes, then survives a
  503 on notification. `TestDraftLookupRejectsDuplicatePendingTags` covers the
  ambiguous-draft refusal.
- Proof: `recipe.sh` matches the `notes.go` template line for line after
  placeholder substitution (diff shows only `@BASE@/@DOWNLOAD@/@ARTIFACT@/@SHA@`
  substitutions); `consumer.log` from a second fresh directory ends in
  `** BUILD SUCCEEDED **`; `window-process.txt`, `window-accessibility.txt`
  (window text `initial`) and `query-command.log` (`initial` →
  `public-release-recipe-proof`) record the running application and the
  generated Swift query/command path.
- Round-02 findings re-checked: #1 (draft lookup) resolved by
  `releaseByTag` with list fallback and duplicate refusal; #2 resolved by
  skipping drafts in the pairing scan; #3 resolved by the release.yml
  comment; #4 resolved by the added proof captures.

## Findings

No material findings remain. The remaining items are genuine nitpicks.

### 1. nitpick — Proof log carries an unexplained error line from the example's shutdown

`ephemeral/native-release-proof/query-command.log` line 3 is
`context deadline exceeded`. It comes from
`plugins/native-shell/example/native/host/main.go:46-48` (`SKGoHostStop` logs
`host.Stop()`'s error) because `loopback/host.go:84` allows 2 s for graceful
shutdown while the Swift client's shutdown runs asynchronously
(`ApplicationView.swift:11`). Both query results printed first and the
process exited through `NSApplication.terminate`, so the proof stands, but the
README describes the log as "the actual before/after results" without noting
the trailing error. This is pre-existing example behaviour, outside the
release automation; either note it in the proof README or make the example's
stop path tolerate a timed-out graceful shutdown.

### 2. nitpick — Pairing scan cost grows linearly with published native releases

`publishedPair` (`main.go:862-935`) runs `gh release download … candidate.json`
for every non-draft native release that has a manifest, on every automatic
candidate run, before comparing `source`/`core`. With one release today this
is negligible; with dozens it becomes dozens of downloads per core release.
The release title already encodes the core version
(`native-shell <plugin> for skgo <core>`), and the asset list is in the
listing, so a cheap pre-filter on title or on a `core_version`-bearing asset
name would bound the work without weakening verification.

### 3. nitpick — Blank-version reservation probes every native tag

`reserve` (`main.go:357-369`) calls `ownedReservation` for each
`plugins/native-shell/*` tag (2–3 API calls each) before computing the next
patch. Same linear growth as above; filtering to annotated tags whose message
prefix matches this run id would need the same calls, so the practical fix is
to stop at the first owned match (already done) and to skip lightweight tags
early (`ref.object.type != "tag"` is already the first exit). Noted for
awareness rather than action.

## Verified as sound (not findings)

- Draft recovery: `releaseByTag` finds pending drafts through the paginated
  releases list, refuses duplicates, and `promote` enters `stageDraft` on any
  draft, uploads only missing original assets after byte-comparing existing
  ones, verifies the complete draft (`allowDraft=true`), publishes it, then
  re-verifies the published release and compares toolchain/receipt bytes
  before notifying. Published releases are only ever read and compared.
- `ownedReservation` uses `releaseByTag`, so an in-flight draft for a tag is
  no longer mistaken for "unpublished and resumable".
- `publishedPair` ignores drafts, so a promotion in progress does not fail a
  concurrent automatic candidate as a mismatch.
- Everything verified in rounds 01–02 (identity resolution, run-owned
  reservations, dispatch-only delivery and fail-visible config, authoritative
  run/attempt/artifact validation, receipt-based recovery after artifact
  expiry, dry-run gating, scoped secrets, no minimum-macOS claim, pinned
  recipe) is unchanged by these commits.
