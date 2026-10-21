# Adversarial review, round 01: Date transport wire declaration

## Review target

- Core fix, working tree `skgo-transport-wire-repair` (branch `codex/transport-time-wire-type-20261021`, uncommitted):
  `internal/gen/transport.go` (`transportEntryPayload`), `internal/gen/types.go:371` (call site),
  `internal/gen/transport_declaration_test.go` (`TestTransportedTimePayloadMatchesCodecString`),
  `ephemeral/worklog/date-transport-adoption-repair.md`.
- Companion consumer fix, working tree `skgo-project-hook-repair` (branch `codex/transport-hook-contract-20261021`, base c0807939):
  `site/web/src/hooks.ts` (`transport: Transport` becomes `satisfies Transport`, `encode` takes `value: unknown`),
  `ephemeral/worklog/date-transport-hook-contract.md`.
- Requirements: both repositories' `CLAUDE.md`, and the two worklogs. The worklogs state the failure (adoption run 39656451330, `src/transport.ts` TS2344), the defect (`Transported[devalue.Date]` declared as `Record<string, never>` while the codec sends an RFC3339 string), the recovery claim, and the constraint that no release or publication action is authorized.

The review scope was derived from the repository instructions, the worklogs and the code. The launch prompt did not narrow it.

## Evidence inspected

- Full diffs and `git status` of both trees, plus the surrounding code: `transport.go` (marker scan, `transportPayload`, `transportDeclaration`), `types.go` (`project`, `generateTypes`), `codecs.go` (codec root lowering), polytype `typegrammar` node kinds, and the consumer's `hooks.go` and generated `skgo_gen.go` (`EncodeDate` goes through `dvEncodeTime`, and `devalue.Date` is `type Date time.Time`).
- Kit's `Transporter<T, U>` definition (`encode: (value: T) => false | U`).
- Failure evidence: `gh run view 39656451330 --log-failed` shows `typescript(TS2344): Type '…/hooks'.Transport does not satisfy the constraint '…/src/transport'.Transport` at `src/transport.ts:20:35`. PR 221 is open, branch `adopt/skgo-v0.45.0`, state UNSTABLE, and changes only `site/go.mod`, `site/go.sum` and `_links` (no `transport.ts` is checked in).
- Core tests run with `GOTOOLCHAIN=go1.27.2`:
  - `go test ./internal/gen/ -run Transport`: pass.
  - `go test ./internal/gen/ -count=1` (whole package): pass.
  - Mutation, via `go test -overlay` restoring the original `types.go`: `TestTransportedTimePayloadMatchesCodecString` fails for `Date` and `LocalDate` with `payload=Record<string, never>; want string`. The test guards the change, and the unmodified call path reproduces the defect.
  - `gofmt -l internal/gen` lists only `testdata/typed-load-evolution/internal/app/locals.go`, which is untouched and pre-existing. `go vet ./internal/gen/` is clean.
- Independent type-level reproduction (scratch directory, Kit's real types from `example/web/node_modules`, `tsc --strict`) of the consumer hook against the generated shape:
  - `Transporter<Date, string>` with the new `satisfies Transport` hook: zero errors.
  - `Transporter<Date, Record<string, never>>`: TS2344, `string | false` is not assignable to `false | Record<string, never>`.
  - This confirms the claimed mechanism and the claimed fix pair at the type level. It does not substitute for the real consumer run.

## Assessment

The core change is correct for the reported defect and consistent with the codec. `transportEntryPayload` asks polytype's `Lower` for the same grammar the codec generator uses (`codecs.go`, same `grammar.Root{Type, Position}` shape), follows `Ref` chains, and returns `string` only when the leaf is `*typegrammar.Time`. Every other transported struct still takes the unchanged `transportPayload` path, so existing object payloads are unaffected. The `Ref` loop cannot cycle for Go defined types. The consumer change is minimal, preserves the runtime behaviour, and is type-sound on the Kit version I checked. I found no verifiable bug in either diff.

## Findings

### 1. Issue: the recorded "actual proof" is prose only, and the publication gate the worklog promises is never named

- Evidence:
  - `ephemeral/worklog/date-transport-adoption-repair.md:7` asserts that the real consumer "passes … zero errors" after generating with the local corrected compiler. It also says setup failures are "retained separately".
  - Neither working tree contains a transcript, command, run id, path or SHA for that passing run. There is also no retained record of the setup failures. No consumer-side file shows the corrected `Transporter<Date, string>` output.
  - Only the failure is externally checkable, and I confirmed it from the CI log. The recovery is checkable only by the type-level reproduction above.
  - The same worklog (line 9) says to "identify the precise publication gate before any automatic source merge/promotion or new release action". It never names it. `skgo-project/CLAUDE.md` places that gate in qualification (`qualify.yml` / by-SHA request) followed by an explicit release dispatch naming a qualified candidate. The worklog states none of this.
  - The consumer worklog (`date-transport-hook-contract.md`) likewise says "retain … all gates until a corrected published core is available" without naming the gates or the order.
- Impact: the next agent or reviewer must redo the real-consumer proof to trust it, and nobody is told what must pass, or in what order, before this fix reaches PR 221. That is the incomplete-requirement shape the worklog sets for itself.
- Fix: record, in the worklogs, the consumer checkout and SHA, the commands and their results, and the setup failures that were set aside. Then name the gate sequence: core merge, skgo-project qualification of the exact SHA, explicit Release dispatch, then PR 221's pin bump.

### 2. Nitpick: the consumer hook does not check against Go until the pin moves, and nothing records the follow-up

- Evidence: at the current v0.40.0 pin no generated `src/transport.ts` exists, so `satisfies Transport` (imported from `@sveltejs/kit/hooks`, `site/web/src/hooks.ts`) only checks Kit's `Record<string, Transporter>` shape. The ManualDate key and payload agreement with Go appears only after PR 221's bump generates `src/transport.ts` and its `Agreement` type.
- The `ephemeral/worklog/date-transport-hook-contract.md` rationale (compatible with the pin because it imports Kit's type) is true. But the file the generator's header tells apps to import (`import type { Transport } from "./transport"`, as in `example/web/src/hooks.ts:1`) is not used, and the worklog does not say whether the hook should move to it after the bump. As written, the type-level coverage of this PR in the consumer's own CI is limited to a shape check that previously passed.
- Impact: low. The behaviour is fine and the follow-up is optional, but it should be stated.

### 3. Nitpick: stray `package.json` and `pnpm-lock.yaml` at the consumer root

- Evidence: `git status` in `skgo-project-hook-repair` shows untracked `package.json` and `pnpm-lock.yaml`. They are not ignored, and they sit in the repository root with no `node_modules` rule that excludes them. `package.json` is the unmodified `npm init` default (`"name": "skgo-project-hook-repair"`, `"license": "ISC"`, `"test": "echo \"Error: no test specified\" && exit 1"`). `pnpm-lock.yaml` is an empty importer.
- Impact: they are untracked noise from running a package manager in the wrong directory, and a blanket `git add` would put them in the PR. Remove them before commit. For the project, the zero-npm-calls rule means the frontend toolchain runs only through `vp` inside `site/web`.

### 4. Nitpick: the regression test stops at `entry.payload.expr`, and some surrounding prose is now stale

- Evidence:
  - `TestTransportedTimePayloadMatchesCodecString` asserts only `entry.payload.expr` after `generateTypes`. The user-visible artifact is `transportDeclaration` (the `.ts` `Transporter<Date, string>` line and the `.js` `@typedef` member). Both derive from `payload.expr`, so risk is low, but no test pins the emitted text. The real `time.Time` root (`Transported[time.Time]`) is also not exercised: the test uses a stand-in `time` package.
  - The doc comment on `transportPayload` (`transport.go:181`) and the marker error ("a transported type has to be a struct, because its encoding is what the browser's decode receives", `transport.go:106`) still describe an object encoding as the only wire shape. That is no longer true for a time-defined struct.
  - Because `Lower` now runs in `generateTypes` before the codec phase, a transported type polytype cannot lower now fails there with the `cannot be transported under` wrapper instead of the codec message. This is a harmless change, but it is a behaviour difference not covered by a test.
- Impact: documentation and coverage polish only. No functional defect found.

## Outcome

`material findings remain` — the code is sound. The remaining issue is the missing, non-reproducible recovery evidence and the unnamed publication gate (finding 1). Findings 2 to 4 are nitpicks.
