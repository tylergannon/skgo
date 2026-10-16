# Adversarial review: issue 341 transport types, round 01

Date: 2026-10-16 12:09 local. Reviewer: Claude Fable 5.1. Read-only review; the only file written is this one.

## Review target

Working tree of worktree `skgo-341-transport-types`, branch `codex/issue-341-transport-types`, on top of `860a308`.

- Modified: `internal/gen/types.go` (`transportedNamed` compares `types.TypeString(named, nil)` instead of `named.Obj()` pointer identity).
- Added: `internal/gen/transport_identity_test.go` (`TestTransportTypeIdentityAcrossAnalysis`).
- Added: `internal/gen/transport_retained_test.go` (`TestRetainedTransportDecodedTypes`).
- Added: `ephemeral/worklog/transport-types.md`.

Specification: tylergannon/skgo issue 341 ("Dev regeneration can replace decoded transport types with encoded carrier declarations") and its one comment; repository instructions in `CLAUDE.md`.

Caller constraints honored: read-only except this artifact. No caller instruction narrowed the subject matter, so nothing was ignored. The `agent-protocol` skill was not invoked because its worktree/worklog/checkpoint side effects conflict with the read-only constraint of this review.

## Evidence inspected

- Issue 341 body and comment: expected behaviour is that dev regeneration preserves the decoded transport type (hooks import) in load and remote signatures, including a struct containing a transported field, and never substitutes the Go carrier shape. The issue names `transportedNamed`'s pointer comparison as a lead and asks that distinct identities be confirmed before choosing a repair; it forbids weakening Gimbal's decoder.
- `internal/gen/types.go:655-671` (the change), `:88-104` (named projection appending the visited `*types.Named` to `transported`), `:673-712` (`containsTransported` / `findTransported`), `internal/gen/wire_fields.go:67`, `internal/gen/transport.go:24-118` (marker scan and `checkTransportKeys`, which already keys by `entry.named.String()`), `internal/gen/codecs.go:55-59` (`codecSet.root` keyed by `types.TypeString`), `internal/gen/emit.go:39-118, 325-336, 474-520, 978-1020` (hooks import emission, `fileImports.byPkg` keyed by `*types.Package` pointer, `transportImports` deduped by path).
- `internal/gen/retained.go:1-140, 640-760` (retained Project, `SetMaxUniverses(4)`, `generate` using a shared `goload.View`), `internal/devserver/session.go` call sites of `OpenProject`, `Prepare`, `ObserveRoutes`.
- `go doc -all github.com/tylergannon/polytype/goload`: "Importing a package from export data then requesting its source starts a separate universe: go/types cannot safely promote an imported package into a source-checked package in place" and "Export-to-source promotion needs another universe". This is the mechanism by which the hooks package's `RunSnapshot` can exist as two `*types.TypeName` objects with equal qualified names, which is what the worklog reports was observed in the real Gimbal run.
- Test helpers: `foreignFixture` (`ownership_test.go:36`), `prepareRetained`, `observeRetained`, `observeRetainedWrite` (`retained_test.go`).
- Worklog `ephemeral/worklog/transport-types.md`: records that instrumentation in the real consumer confirmed equal qualified names with distinct `TypeName` and `Package` pointers while `Transport()["RunSnapshot"]` was still generated. That satisfies the issue's precondition ("determine whether the failed invocation supplied distinct Go type identities ... before choosing a repair") on the author's word; it is not reproduced by anything in the repository (see finding 1).

### Commands run

```
go test ./internal/gen -run 'TestTransportTypeIdentityAcrossAnalysis|TestRetainedTransportDecodedTypes' -count=1
  ok (0.78s)

# mutation: original types.go restored through a -overlay (project files untouched)
go test -overlay <scratch>/overlay.json ./internal/gen -run 'TestTransportTypeIdentityAcrossAnalysis|TestRetainedTransportDecodedTypes' -count=1 -v
  --- FAIL: TestTransportTypeIdentityAcrossAnalysis
        same_declaration: decoded result = {expr:RunSnapshot deps:[...] transported:[]}  (carrier declared, not imported)
        containing_struct: decoded result = {expr:Data deps:[...] transported:[]}
        "same instantiated declaration lost transport"
  --- PASS: TestRetainedTransportDecodedTypes (0.53s)      <-- passes WITHOUT the fix

# instrumentation: copy of the fixed types.go that prints when a match has
# equal TypeString but distinct Obj() pointers, applied through -overlay
go test -overlay <scratch>/overlay_instr.json ./internal/gen -run TestRetainedTransportDecodedTypes -v
  --- PASS, zero "SKGO-IDENTITY-SPLIT" lines     <-- the split identity never occurs in this fixture

go test ./internal/gen -count=1      ok (104.7s)
go vet ./internal/gen                clean
gofmt -l <changed and added files>   clean
```

## Assessment of the fix

The change is small, placed at the single site the issue named, and consistent with the two places the generator already identifies Go types by qualified string (`codecSet.root`, `checkTransportKeys`). Including type arguments through `TypeString` is correct: `Box[int]` and `Box[string]` share a `TypeName` and the old pointer check would have conflated them. Every caller of `transportedNamed` (`projectType`, `findTransported`, `wire_fields.go`) benefits, so the struct-containing-a-transported-field case from the issue (`Data{Snapshot RunSnapshot}`) is covered by the same comparison. `TypeString(named, nil)` fully qualifies by package path, so a same-named type in another package is not matched, which the unit test checks. Gimbal's decoder and component types are not touched, as the issue requires.

## Findings

### 1. issue: `TestRetainedTransportDecodedTypes` is not load-bearing for issue 341

Evidence: `internal/gen/transport_retained_test.go:1-55`. With the fix reverted through a build overlay the test passes on all three iterations, and an instrumented `transportedNamed` reports zero cases where the matched entry has a distinct `Obj()` pointer. The fixture's hooks root (`example.com/app/web/src`) and its consumer (`example.com/app/runview`) are checked from source inside one goload universe on every `Prepare`, so the export-data-then-source promotion that produced the bug never happens. The test therefore asserts a property that was already true before the change and cannot detect the regression it is named for.

Repository rule: "Validate by dispatching an agent that checks they are load-bearing — would each one actually fail if the feature were broken or removed". This one would not. The issue's own research boundary already warned that the isolated comparisons "did not reproduce the divergence"; this test is another such comparison.

Impact: the only guard on the dev regeneration path is the synthetic unit test, which constructs the two analyses by hand. If a future change to retained analysis or to `projectType` reintroduces pointer-identity reliance anywhere upstream of `transportedNamed` (for example a cache keyed on `*types.Named`), nothing at the Project level will notice.

What would make it load-bearing: drive the fixture into the state goload documents (import the hooks package from export data first, then request its source, e.g. by loading the consumer root before the hooks root, or by exhausting `SetMaxUniverses`), and fail the test as a precondition when the two `RunSnapshot` objects turn out to share a pointer, the way `transport_identity_test.go:40-42` already does with "fixture did not create independent analyses". If that state cannot be produced deterministically from the Project API, the test should be removed rather than kept as a passing non-check.

### 2. nitpick: the qualified id is recomputed for every entry on every lookup

Evidence: `internal/gen/types.go:664-667`. `transportedNamed` is reached from `findTransported` for every named type in every struct field of every load, remote, action and wire type, and `types.TypeString` allocates and walks type arguments each time. Computing `TypeString(entry.named, nil)` once when the marker is read (`transport.go:46`, where `checkTransportKeys` already builds the same string) and storing it on `transportedType` removes the repeated work and keeps one definition of "transport identity" instead of two (`entry.named.String()` in `checkTransportKeys`, `TypeString(entry.named, nil)` here; they are equal today, but they are spelled differently).

### 3. nitpick: `TypeString` collapses function-local named types

Evidence: `internal/gen/types.go:664`. `types.TypeString` renders a named type declared inside a function body as `pkg.Name`, identical to a package-level type of the same name. A load or remote that returns a function-local type whose name equals a transported type's name in the same package would now be treated as transported, where the pointer comparison would not. Transport markers are package-level `var _ =` declarations, so the transported side cannot be local, and returning a locally declared type from a load is unusual; this is a theoretical regression, noted for completeness.

### 4. nitpick: other identity sites still assume one universe

Evidence: `internal/gen/emit.go:474-500` keys import aliases on `*types.Package` pointers. Under the same export-data/source split that motivated this change, a package reached through both objects receives two aliases (`runview`, `runview2`) and two import specs for one path in the generated bindings. Go accepts a package imported twice under different names, so this is cosmetic rather than a compile failure, and it is outside the issue's stated scope. It is recorded as a lead because the mechanism is identical and the worklog confirms distinct `Package` pointers were observed.

## Outcome

material findings remain
