# Adversarial review: remote command/form context-first API, round 01

## Target

- Branch `codex/remote-function-params`, HEAD `3d21533` ("Revise remote command
  and form APIs to explicit context and readable params"). The API revision
  starts at `4669720`.
- Spec: `ephemeral/plans/20261005-remote-function-params.md` (whole document),
  `AGENTS.md`/`CLAUDE.md`.
- Latest user execution direction (plan §"Current continuation instructions"):
  fix concrete, demonstrable correctness and material completeness problems.
  Do not chase exhaustive polish. Stop at roughly 90–95% once the main
  capability is implemented and independently checked.
- Baseline proof for the previous API is in
  `ephemeral/tmp/remote-params-independent-qa-final.log`.

The launch prompt did not narrow the review scope, so I ignored nothing from it.

## Evidence inspected

- Full diff `4669720..3d21533`, including:
  - generator: `internal/gen/scan.go` (`readMarker`, `remoteSignature`, `shapeOf`), `emit.go`, `shared_params.go`
  - runtime: `request_event.go`, `event.go`, `loadevent.go`, `remote.go`, `remote_form.go`, `document_form.go`, `remote_caller.go`, `middleware.go`
  - advice analyzer and its catalog and testdata
  - example consumers, generated bindings and `params_gen.go`
  - feature files and test fixtures
- Builder logs in `ephemeral/tmp/context-api-*.log`, and the worklog
  `ephemeral/worklog/20261006-context-first-remotes.md`.
- Searched every remaining `skgo.Command`/`skgo.Form` author site: `internal/newapp` templates, README, `doc.go`, `skills/skgo/SKILL.md`, and docs.

### Checks I ran

| Check | Result |
|---|---|
| `just test` | exit 0, 25 packages ok. This includes an untracked `ephemeral/tmp/union-api-probe` package that `./...` sweeps up; it is not committed. |
| `skgo check --root example` (read-only, local staticcheck) | exit 0; all 9 checks complete. |
| `just serve` + `just e2e prod` | 185 passed; 0 skipped, flaky or failed. |
| `just dev` + `just e2e dev` | 185 passed; 0 skipped, flaky or failed. The worktree was clean afterwards. |

The builder had run neither browser suite on this revision. Its logs show only build, generate and focused Go tests.

### Mutations

I ran these only in a scratch copy of the repository, never in the worktree.

- **Generated call passes the dispatcher `ctx` instead of `event.Context()`** (`emit.go:902`): `TestGeneratedCommandFormCallerEvents` fails. `check()` panics with "raw/context caller access allowed", and the overlap subtests fail. The ctx-restriction assertion is load-bearing.
- **Package-qualification stage removed from `nameSharedAlternatives`**: `TestSharedParamsReadableVariantNames` fails. It reports `NumberParam_OrderNumber_9471…` where `NumberParam_SalesOrderNumber` was expected. The naming assertion is load-bearing.

### Looking at the page

Production was already covered by the suite. In the dev server I:

- opened `/typed-load/42` and previewed a message. The page showed `number:params.NumberParam_OrderNumber:{42}` with the body. I looked at the screenshot myself.
- signed in, which writes a cookie and refreshes queries through the new restricted ctx. The page showed "Signed in as Reviewer".
- signed out again.

The only console error was a favicon 404.

### Runtime reasoning I confirmed

- `RequestEvent.Context()` (`request_event.go:17–28`) copies the event and clears `caller/hook/load/params`. It sets `query` while leaving `mutable`, `jar` and `refreshes` shared. As a result:
  - cookie writes and `Refresh*` work through ctx;
  - `URL`/`SearchParam`/`RouteID`/`Param`/`Params`/`RemoteCallerValues` panic through ctx.
- The base context is `e.Request().Context()`. Every command/form dispatcher (`remote.go:832/881`, `remote_form.go:183`, `document_form.go:199`) builds its ctx from that same `r.Context()`, so values, deadlines and cancellation are preserved.
- The caller value left in the request context is overwritten by `matchCaller` on every remote request (`remote.go:766–771`).
- `readMarker` indexes `Params().At(1)` only after `remoteSignature` has validated the arity, so obsolete shapes cannot panic it.

## Findings

### 1. critical — `skgo new` example scaffold still declares the obsolete ctx-only command, so new projects fail generation

Type: verifiable bug, and incomplete requirement.

**Requirement.** Plan §"Kit mapping and runtime seams": "Update authored consumers, diagnostics, generation fixtures and marker documentation together. Event-only and ctx-only authored command/form shapes should receive the new source-located signature guidance."

**Evidence.**
- `internal/newapp/gofiles/examples/web/src/routes/example.remote.go.tmpl:27` declares `func record(ctx context.Context, name string) (Status, error)`, and line 44 registers it with `skgo.Command(record)`.
- Neither this revision nor the earlier event-first revision on this branch touched the template. Its last change was `a14ceed`.
- `newapp.go:292` runs `go generate ./...` on the scaffolded project whenever the sv demo is chosen (`p.Examples = chosen.demo`).
- The newapp unit tests use a fake command runner. They only check that `example.remote.go` exists (`newapp_test.go:409`), so `just test` stays green.

**Reproduction.**
1. Copy the repository to a scratch directory, with `node_modules` symlinked.
2. Place the template as `example/web/src/routes/scaffold/scaffold.remote.go`, changing only `package routes` to `package scaffold`.
3. Run `go generate ./...` in `example/internal/skgo`. It fails with:

```
scaffold.remote.go:44:6: record is declared as a command, but its signature is
func(ctx context.Context, name string) (…Status, error). A command is
func(context.Context, skgo.RequestEvent[params.Params]) (Out, error), or
func(context.Context, skgo.RequestEvent[params.Params], In) (Out, error).
```

**Impact.**
- Once released, the same binary that ships this template rejects it. `skgo new` with the demo starter would stop at "generating the Go bindings failed".
- The scaffold is the first command/form a new developer sees, and it shows the superseded shape.

**Fixing it needs:**
- the template moved to `ctx, event skgo.RequestEvent[params.Params], name`;
- an import of the project's generated params leaf, which depends on the module path;
- a test that actually generates and compiles the scaffold. `TestCatalogRepairExamplesCompileAgainstThisSkgo` is a ready pattern for that.

### 2. nitpick — readable package qualification is proved only on synthetic types; compiled fixtures exercise only the digest fallback

Type: incomplete proof.

**Requirement.** Plan §"Accepted variant naming" and Development sequence step 3: "Naming fixtures exercise the settled collision policy."

**Evidence.**
- `TestSharedParamsReadableVariantNames` (`shared_params_identity_test.go:87–130`) builds `types.NewNamed` values in memory and calls the naming functions directly.
- The fixtures that are actually generated and compiled (`remote_params_test.go:91`, `shared_params_test.go:240`) collide two packages that are both named `domain`. They therefore only ever compile `IDParam_DomainNumber_<64-hex>`.
- No generated, compiled consumer switches on a qualified name like `NumberParam_SalesOrderNumber`.

**Impact.** Low. The emission path (`sharedAlternativeName`) is shared, and my mutation showed the unit test is load-bearing for the naming function. What is unproven is whether the qualified name reaches generated declarations, constructors and an application `switch`.

**Fix.** Add a differently named colliding package to one existing generated fixture.

### 3. nitpick — the diagnostic still recommends the hidden-argument alias

Type: incorrect guidance.

**Requirement.** Plan §"Revised public API": do not hide the type argument "in the canonical authored examples through `params.RequestEvent`."

**Evidence.**
- `internal/gen/scan.go:577` says "must receive skgo.RequestEvent[%s.Params] or its generated RequestEvent alias".
- `params_gen.go:28` still emits `type RequestEvent = skgo.RequestEvent[Params]`.

Accepting the alias is correct, since the scanner checks type identity. Advertising it in the one-line repair text steers authors back toward the spelling the user rejected.

**Fix.** Drop the alias clause from the message.

## Observations, not findings

- The untracked `ephemeral/tmp/union-api-probe` Go package runs as part of `just test` in this worktree. It is harmless here, but remove it before relying on package counts.
- Nested direct query calls still get a `mutable` event, so they can write cookies where Kit would refuse. This predates the revision and is inherent to direct Go calls. The advice analyzer's QueryCookie rule is the existing mitigation.

## Outcome

material findings remain
