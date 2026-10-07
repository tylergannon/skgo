# Adversarial review: remote command/form context-first API, round 02

## Target

- Branch `codex/remote-function-params`, HEAD `6b96208` ("Repair demo starter command API and generation bootstrap"). The API revision starts at `4669720`; round 01 reviewed `3d21533`.
- The authoritative sources are unchanged:
  - `ephemeral/plans/20261005-remote-function-params.md` (whole document), including its latest execution direction: fix concrete correctness and material completeness gaps, and stop at about 90–95% without chasing polish;
  - `AGENTS.md`/`CLAUDE.md`.

The launch prompt did not narrow the scope. I re-reviewed the whole revision, not only the repair.

## Evidence inspected

### Code

- The repair diff `3d21533..6b96208`:
  - `internal/newapp/gofiles/examples/web/src/routes/example.remote.go.tmpl` now uses `record(ctx, event skgo.RequestEvent[params.Params], name)` and imports `{{.BindingsImport}}/params`.
  - `internal/newapp/newapp.go:310–329` adds `initializeGo`. For the demo it runs `go get -tool …/cmd/skgo@<version>`, then `go generate ./...`, then `go mod tidy`. Without the demo the order stays tidy, then generate.
  - `TestDemoStarterGeneratesAndCompiles` (`newapp_test.go:877–908`) renders the real templates and runs `initializeGo` with real Go commands. It then runs `go build ./...`.
  - One worklog line.
- `gofiles/go.mod.tmpl` already declares `require` and `tool github.com/tylergannon/skgo/cmd/skgo`, so the `go get -tool` step only resolves the tool's module graph and sums before generation.
- Starter README and other example templates: neither mentions the old signature.
- Round-01 material is otherwise unchanged and still applies:
  - the generator signature and dispatch path;
  - the restricted `RequestEvent.Context()`;
  - readable naming;
  - advice analyzer, example consumers and feature files.

### Checks run this round

| Check | Result |
|---|---|
| `go test -count=1 -v ./internal/newapp` | ok. `TestDemoStarterGeneratesAndCompiles` ran and passed (about 3 s). |
| `go test -count=1 ./cmd/skgo ./internal/newapp` | ok |
| `go vet ./internal/newapp`, `gofmt -l internal/newapp` | clean |

### Repair mutations

These ran in a scratch copy of the repository, never in the worktree.

- **Template reverted to `record(ctx context.Context, name string)`, without the params import:** the new test fails with the source-located "record is declared as a command, but its signature is …" diagnostic.
- **The demo's first step reverted to `go mod tidy`:** the new test fails with "cannot find module providing package example.net/customer/greetings/internal/skgo/params".

Both parts of the repair are therefore load-bearing.

### Carried over from round 01 on `3d21533`

The repair touches only `internal/newapp`, so these results still apply:

- `just test`: 25 packages ok.
- `skgo check --root example`: all 9 checks complete.
- `just e2e prod` and `just e2e dev`: 185 passed each, with 0 skipped, flaky or failed.
- Mutations showing the ctx-restriction and naming assertions are load-bearing.
- A dev-server look at the numeric caller preview and at sign-in/out through the new ctx.

I did not repeat the expensive suites for a change confined to the project scaffolder.

## Findings

Round-01 finding 1 is resolved. The demo starter now uses the explicit generic context-first signature. A real-Go regression test generates and compiles it starting with no generated output, and fails if either the template or the bootstrap order regresses.

No material findings remain. Two nitpicks from round 01 are still open.

### 1. nitpick — readable package qualification is proved only on synthetic types

- `TestSharedParamsReadableVariantNames` (`internal/gen/shared_params_identity_test.go:87–130`) checks `NumberParam_SalesOrderNumber`/`NumberParam_LegacyOrderNumber` against in-memory `types.NewNamed` values.
- The fixtures that are generated and compiled (`remote_params_test.go:91`, `shared_params_test.go:240`) collide two packages that are both named `domain`. They only compile the digest fallback `IDParam_DomainNumber_<64-hex>`.
- The emission path is shared, and the unit test is load-bearing (round-01 mutation), so the risk is low. Still, no compiled consumer switches on a package-qualified name.
- Plan step 3 asks that "naming fixtures exercise the settled collision policy". Adding one differently named colliding package to an existing generated fixture would close this.

### 2. nitpick — the diagnostic still advertises the hidden-argument alias

- `internal/gen/scan.go:577` says "must receive skgo.RequestEvent[%s.Params] or its generated RequestEvent alias".
- The plan's "Revised public API" section rejects `params.RequestEvent` as the canonical spelling.
- Accepting the alias is correct, because the scanner checks type identity. Recommending it in the one-line repair text is not.

## Outcome

only nitpicks remain
