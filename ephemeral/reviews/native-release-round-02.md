# Adversarial review: native release automation (round 02)

Outcome: **material findings remain**

## Target

- Commit `77cf98d6 ci: harden native release recovery and consumer installation`
  on top of `3b3c7c94` (worktree `native-release-public`, tree clean apart
  from session logs and this review).
- Authoritative requirements: `ephemeral/native-release-review-prompt.md`
  (round-01 list plus the two added paragraphs: artifact-expiry-proof recovery
  of the published notification; no unproven minimum macOS claim; partial
  draft recovery that adds only missing original assets, rejects mismatches,
  publishes only after the complete draft verifies; published releases stay
  immutable), `CLAUDE.md`, agent protocol.
- The launch arguments end with `--output
  ephemeral/reviews/native-release-agent-round-02.json`. That is a harness
  flag, not a review artifact path under this skill; nothing was written
  there. No instruction narrowed scope.

## Evidence inspected

- `git diff 3b3c7c94 HEAD` for `.github/workflows/native-plugin.yml`,
  `cmd/skgo-native-release/{main.go,main_test.go,notes.go}`; the full current
  `main.go` (1027 lines) around `ownedReservation`, `reserve`, `promote`,
  `stageDraft`, `verifyPublished`, `publishedPair`, `eligible`.
- `go test`, `go vet`, `gofmt -l` on the package: 17 tests pass, clean.
- Proof: `ephemeral/native-release-proof/{README.md,recipe.sh,consumer.log}`.
  `recipe.sh` is line-for-line the `notes.go` template with v0.1.0/v0.46.0
  values substituted; `consumer.log` ends in `** BUILD SUCCEEDED **` and the
  native build products path, with only benign warnings.
- Round-01 findings re-checked: #1 (unproven recipe) resolved by the proof and
  the reworked recipe; #2 (stranded explicit version) resolved by annotated
  run-owned reservations (`TestUnbuiltReservationCanResumeOnlyItsOriginalRun`);
  #3 (dry-run red runs) resolved by the Linux `eligible` gate
  (`TestCoreDryRunIsSkippedBeforeNativeBuild`); #4 (token scope) resolved by
  per-step secrets; #5 kept by recorded decision.
- GitHub REST documentation for "Get a release by tag name" and the gh CLI
  source (`pkg/cmd/release/shared/fetch.go`): the REST endpoint returns only a
  *published* release and 404s for drafts; gh works on drafts only because it
  falls back to a GraphQL lookup by pending tag name. The repository currently
  has no draft release, so an empirical read-only demonstration was not
  possible and creating one would be a mutation outside this review's
  boundary.
- Release-job display name coupling: `release.yml:333` vs `main.go:980`.

## Findings

### 1. critical — Promotion can never leave the draft stage; every rerun creates another draft

`main.go:737-760` (`promote`):

```
err = c.api("GET", "releases/tags/"+url.PathEscape(tag), nil, &rel)
if missing(err) {
    runGH("release", "create", tag, ..., "--draft", ...)
    if err = c.api("GET", "releases/tags/"+url.PathEscape(tag), nil, &rel); err != nil {
        return err
    }
}
...
if rel.Draft { stageDraft(...); gh release edit --draft=false ... }
```

`GET /repos/{owner}/{repo}/releases/tags/{tag}` is documented as "Get a
*published* release with the specified tag" and returns 404 for drafts; a
draft has no tag until it is published. Consequences on the real API:

1. First promotion: `gh release create --draft` succeeds, the immediate
   re-lookup at `main.go:742` gets 404 and `promote` fails. The draft exists
   with all assets but `rel.Draft` is never observed, so the edit at
   `main.go:752` never runs. Nothing is published, no notification is sent.
2. Rerun: the lookup at `main.go:737` is 404 again, so `promote` creates a
   *second* draft for the same tag (GitHub permits multiple drafts sharing a
   `tag_name`), and fails the same way. Each rerun adds a draft; none is ever
   published.
3. The partial-draft recovery path (`stageDraft`, `main.go:993-1026`) is
   unreachable for the same reason: it is only entered when the tag lookup
   returns a draft, which it cannot.

This defeats "Qualification explicitly gates publication; original candidate
bytes must be promoted" (nothing is promoted) and the added requirement
"Publication must recover a partial draft upload" (the recovery branch cannot
execute). It also leaves `publishedPair` (`main.go:880-935`) seeing an orphan
draft in `GET /releases` for the same source/core and failing the next
automatic candidate with "existing release identity mismatch".

The passing test `TestPromotionPublishesOriginalBytesAndRecoversNotification`
hides this because its mock (`main_test.go:267-271`, `320`) serves the draft
from `/releases/tags/...` after `create`, i.e. the expectation was derived from
the implementation rather than from GitHub's contract (`CLAUDE.md`: "Never
derive an expectation from the thing under test"). `gh release download` and
`gh release edit` do work on drafts, but only through gh's own GraphQL
fallback, which the Go code does not use.

Fix direction: locate the draft the way gh does — list `GET /releases`
(`per_page=100`, paginate) and select `draft == true && tag_name == tag`
(there must be exactly one; more than one is a hard failure), or query GraphQL
by pending tag name; use the REST tag endpoint only for the published check.
The fixture must then serve drafts only from the list endpoint and keep
returning 404 from `/releases/tags/...` until `--draft=false`.

### 2. nitpick — Pairing scan treats a legitimate in-flight draft as a mismatch

`main.go:900` calls `verifyPublished(rel, m, dir, false)` for every listed
native release whose `candidate.json` matches the current source and core.
Once finding 1 is fixed, a draft will legitimately exist between a failed and
a successful promotion attempt; an automatic candidate run in that window
(e.g. a Release rerun for the same core) will fail on the draft instead of
either ignoring it or reporting "promotion in progress". Skip `rel.Draft`
entries in `publishedPair` or name the state in the error.

### 3. nitpick — Eligibility gate is coupled to another workflow's job display name

`main.go:980` matches the literal `"Publish GitHub Release and Homebrew cask"`
from `release.yml:333`. A rename fails closed ("core Release has no recognized
publishing job"), which is visible, but the coupling lives in Go source rather
than next to the job it describes. A comment on the `release.yml` job naming
the consumer, or matching on the job's `id`-derived name via the identity
artifact, would make the dependency discoverable.

### 4. nitpick — Proof directory records the window observation only as prose

`ephemeral/native-release-proof/README.md` states the launched `TinyNative`
window showed `initial` and names a PID; `consumer.log` ends at the build.
The claim is credible and the recipe is proven to build, but the proof folder
keeps no capture (screenshot or `lsappinfo`/`ps` excerpt) of the final step
the repository rule asks for ("someone looks at the running app").

## Verified as sound (not findings)

- Reservation ownership: annotated tag message carries run id and source;
  only the same run with no published release may resume; lightweight or
  foreign tags are never replaced (`TestUnbuiltReservationCanResumeOnlyItsOriginalRun`).
- Receipt-based recovery: `release.json` carries the full original request and
  the toolchain SHA256; `verifyPublished` checks manifest equality, plugin
  bytes, checksum file, toolchain hash and receipt identity; `publishedPair`
  additionally checks tag source and core identity without touching Actions
  artifacts (`TestPublishedPairRecoveryUsesPermanentReceiptAfterArchiveExpiry`).
- Incoming promotion still requires the original artifact and a passed
  `candidate` job (`authorize`, unchanged).
- Dry-run gating runs on Linux before any macOS work; a purported publishing
  success without identity still fails closed in `prepare`.
- Secrets are scoped to the three steps that use them.
- Notes no longer advertise a minimum macOS version; links include the
  publication receipt; pins are frozen from the selected core and native
  sources; the generated recipe was executed verbatim and built the app.

## Note on concurrent edits

While this review was being written, uncommitted edits appeared in
`cmd/skgo-native-release/notes.go` and `ephemeral/native-release-proof/recipe.sh`
(splitting `export PATH="..."` into an assignment plus `export PATH`, a
shellcheck-style change). They were not made by this review, do not affect any
finding above, and were not inspected further; the reviewed commit is
`77cf98d6`.
