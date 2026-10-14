# Adversarial review: dev-server plan, bounded reclamation revision

Reviewer: Claude Fable 5.1, read-only. Date: 2026-10-14.

## Target

`ephemeral/projects/dev-server/PLAN.md` in `/Users/tyler/.codex/worktrees/0879/skgo`, reviewed as the engineering target under the user's stated bounds: maximize useful in-place replacement and actual retirement up to diminishing returns; retention under a finite budget with host recycling is the fallback, not the optimization target; correctness is bounded to an implementable supported capability; no general escape analysis, proof machinery, ledgers or verdict runners. Development remains paused. The plan's "next decision" section (lines 34–44) is proposed work, not demonstrated behavior.

No caller instruction narrowed the defects, files or subject matter, predicted a conclusion or requested a verdict. The read-only boundary and output path were honored.

## Evidence inspected

- `PLAN.md` in full, with the "Next decision" section (34–44), the code replacement and state section (75–97), step 1 (137–161), and validation (254–266) read against each other.
- Preserved implementation at checkpoint `b487dd0`, as selected into this worktree: `internal/hotreload/{classify,lower,toolchain,native_test}.go`. The `30de` copy was compared for what is being removed (SSA storage-address checks, serial `Unload`).
- Earlier consultations in `30de`: `fable-diminishing-returns-raw.json` (tiers: retire / retain / rebuild; object-fingerprint boundary), `FABLE-BOUNDED-CORRECTNESS.md`, `SAFE-RETIREMENT.md`, `SLICE-01.md`.
- GoLoader `2695cfc` at `/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/link/`: `ld.go` `addSymbolMap` (411–441), `buildModule` (469–547), `Load` (561–635), `Unload` (675–683); `module.go` `addModule`/`removeModule`; `init.1.21.go` `doInitialize`; `iface.1.10.go` `removeitabs`; `itablink.1.27.go`.
- skgo consumers: `refresh.go` `lookupFunc`/`codePointer` (300–318), `remote_live.go` `serveLive` (36–147), `remote.go` `callLive` (935), `example/web/src/routes/todos/todos.remote.go`, `example/web/src/routes/(marketing)/pricing/pricing.remote.go`, `example/web/src/routes/stream/page.server.go`.
- Repository plumbing: `Justfile` (`test`, `e2e`, `worktree`), `.github/workflows/{ci,qualification}.yml`, root and example `go.mod`.
- Not verifiable here: the plan's Kit `src/exports/vite/dev/index.js` line citations. Neither worktree has `example/web/node_modules/@sveltejs` installed, so those citations were not checked and nothing below depends on them.

## Findings

### 1. Critical: the plan keeps closures, boxing and new itabs outside the supported class, which empties the retain tier it just introduced

Lines 38 and 42 introduce three outcomes (reclaim, retain, rebuild) and say retention exists for "a compatible synchronous edit whose result persists in host state." Line 42 then says "Retention alone does not expand support to goroutines, callbacks or arbitrary new runtime types," line 93 says "Newly loaded callbacks, goroutines, deferred values and runtime types remain outside the initial scope unless separately demonstrated," and line 153 tests that "a type/global-initializer/new escaping-lifetime change is classified before an unsafe load." Read literally, an implementer rebuilds the host for any edit that introduces a func value or a candidate-resident static.

That contradicts line 44, which names sorting and formatting as representative edits, and it contradicts the user's bound. A `slices.SortFunc` comparator, a `fmt.Sprintf("%d", 3)` boxed constant, a `skgo.Errorf` wrapping a candidate itab, a stored callback: these are exactly the cases the earlier consultation put in the retain tier because, with the module never unmapped, there is no lifetime question left to answer. The loader confirms the premise. `addSymbolMap` (`ld.go:425–427`) always places text in the candidate mapping and (`ld.go:434–438`) resolves data to the host only when a host symbol exists, so a retained mapping is a complete, permanently valid home for every such pointer. Under the plan as written, retention covers only "candidate-backed values persisted into host state" (line 40), which is the narrowest possible slice of that tier, while the common edits that make retention worth having go to a full rebuild.

The consequence is a product that rebuilds on most real edits while carrying all of the retention machinery, which is the opposite of "maximize useful replacement."

Concrete change. Replace the exclusion sentences in lines 42 and 93 with an explicit assignment rule for the three outcomes:

- Reclaim: the candidate object, after host resolution, defines only function text, generic dictionaries, heap-copied strings and compiler funcdata, and its source contains no `go` statement. Retire after the revision's consumers finish.
- Retain: the object carries any func-value symbol (closure, method value, `·f`), any `..stmp` static, any `type:` or `go:itab.` symbol absent from the host, or the source contains `go`. Install in place, never unmap, charge the budget. This is where sorting, formatting, error wrapping and stored callbacks land.
- Rebuild: binding incompatibility only, per finding 2.

If the plan wants to keep "synchronous" as product scope, say that a `go` statement is excluded as a scope choice, not a safety one, so the exclusion can be lifted without re-analysis. Update line 153 so "new escaping-lifetime" is tested as a retain outcome, not a rebuild, and keep "type/global-initializer" as rebuild.

### 2. Issue: the preserved starting point admits method-body edits and dead-stripped globals as compatible, and the checkpoint's test list does not catch either

Line 36 says to preserve `b487dd0` as the starting point. Line 89 requires that no partial update "leave two callers observing different revisions of the edited helper." The preserved classifier cannot enforce that for methods, and the loader makes the split real:

- `internal/hotreload/classify.go:112–116` nils the body of every `FuncDecl` except `init`, including methods, so a method-body edit compares equal and passes as a body edit.
- `internal/hotreload/lower.go:96–99` copies method declarations unchanged into both the host and the candidate, so the candidate object contains the new method text.
- `ld.go:425–427` places that text in the candidate mapping. Candidate direct callers reach the new body. Host callers outside the package and every host itab keep the old text. Line 93's "unchanged signatures alone do not establish replacement compatibility" is right, but nothing in the checkpoint operationalizes it for this case.

A second silent case: `ld.go:434–438` falls through to candidate data storage for any data or BSS symbol absent from `symPtr`. A package variable the host linker dead-stripped, or one not registered, becomes a second copy and forks state with no error. The plan's text (line 87) forbids this, but the checkpoint tests (line 40) and step-1 proofs (147–157) have no object-level rejection for it. The current `native_test.go` only checks `UnresolvedSymbols`, which does not cover either case because the data symbol is defined in the object.

Both apply equally to the retain outcome, since retention settles lifetime only. Finding 1's assignment rule is unsafe without this gate.

Concrete change. Add to the checkpoint (line 40) a pre-`Load` binding gate that both reclaim and retain must pass, and the tests that prove it:

- Method-body edit is a rebuild, with a test that edits a method, asserts the rebuild classification, and asserts the host is untouched. Fix `declarations()` to keep method bodies, or move method bodies into the executable closure the plan already requires.
- Object inspection after `ReadObjs`: any defined data/BSS symbol for a package variable that is not in `symPtr`, any named `type:` symbol not in `symPtr`, and any `..inittask` whose record has executable functions, each rejects before `Load`. Distinguish a bare inittask record from one with functions, as the earlier caller assessment noted.
- A test where the fixture's host has a package variable that the candidate references but the host did not link, asserting rejection rather than a forked value.

### 3. Issue: the lifetime holder is specified by the handler, but candidate text runs past the handler, and the plan's non-drain outcome skips the fallback

Line 95 is correct that a lease ending when a load returns is too early. But the plan still describes the holder in terms of requests and treats only "unrelated host-only live streams" (line 97, line 152). The consumer that matters is in the same package as the synchronous bodies being edited:

- `todos.remote.go` declares `watchCount` (a live query) beside `getTodos`, `addTodo` and the rest. A body edit to `getTodos` recompiles the whole package; the open stream keeps executing the old revision's `watchCount` text for as long as the tab is open.
- `remote_live.go:81–90` runs the producer in its own goroutine via `callLive` (`remote.go:935`). The HTTP handler can return while the producer is still inside candidate text. The producer has no `go` statement in its own source, so the finding-1 fingerprint does not move it to retain; it is a reclaim-tier revision that cannot drain.
- `stream/page.server.go` uses `skgo.Deferred[T]` with values produced after the load returns, the same shape for loads.

For such a revision, line 97 offers "explicit bounded drain/cancellation or application-host replacement." That jumps from reclaim directly to the last resort. Under the user's ordering, the fallback between them is retention: a reclaimable revision that does not drain within the bound is demoted to retained and charged to the budget. Host replacement is reserved for budget exhaustion and an uncertain host (`Load` returning an error after `buildModule` has already run `addModule` at `ld.go:539`, with no rollback at `ld.go:624–634`). As written, a developer with a live-query tab open would get a host recycle, and therefore a state reset, on every edit to that package.

Concrete change. In lines 95–97 and the step-1 proof list:

- Acquire the revision where candidate text is entered (the stable wrapper), not at the HTTP handler. The executor that actually finishes releases it: `callLive`'s goroutine for live queries, the deferred producer for streamed loads, the renderer for SSR consumption.
- "A reclaim-tier revision that cannot drain within the bound is retained and charged to the budget. Host replacement is selected only at budget exhaustion or for an uncertain host."
- Add a test: open a live stream on revision N, publish N+1 as a reclaim candidate, assert N is not unmapped while the stream is open, assert it is retired after the stream closes, or demoted to retained after the bound. Add the same for a held `Deferred` value.

### 4. Issue: the root `just test` acquires a hard toolchain prerequisite and a per-run host build without a stated budget or a Go 1.28 policy

Line 143 says "root `just test` stays an ordinary Go test invocation and exercises code replacement through the real host on Ubuntu," and line 155 plus `toolchain.go:41–57` make missing prerequisites fail rather than skip. Line 226 requires the suite to stay in the repository's seconds tier.

The preserved test does all of this inside one test function: `PrepareToolchain` walks and rewrites `GOROOT/src/cmd/internal` (`toolchain.go:69–146`), runs `go mod download`, runs `go mod tidy` twice, links an unstripped host that includes skgo and the loader, and compiles three candidates, under a three-minute timeout (`native_test.go:20`). That is not seconds, and it runs on every developer `go test ./...`.

Separately, `toolchain.go:41` refuses anything but `go1.27.*`. With `go.mod` at `go 1.27.1` and no toolchain directive, a developer or CI image on Go 1.28 runs the root suite under 1.28 and fails it by design. The plan speaks to patch versions (line 79) but not to the next minor. CI (`ci.yml:29–31`) selects from `go.mod`, which will keep 1.27 for now; local machines will not.

Concrete change. In step 1 and line 226:

- State the wall-time budget this test may add to `just test` and where preparation is shared: once per test binary, keyed by `Toolchain.Identity`, reusing the prepared view and built host across runs under that key. The current test rebuilds everything per run.
- Decide the Go minor policy in one sentence: either qualifying the next Go minor is ordinary maintenance that must land before the repository's `go.mod` advances, or the series check is a diagnostic that fails only the hotreload package. Do not leave the root suite's pass/fail dependent on an unstated future decision.

### 5. Nitpick: "observe actual mapping reclamation" needs an oracle that is not the loader

Line 40 requires observing actual reclamation; line 151 requires it "independently." The only evidence so far came from the loader's own module list. `Unload` (`ld.go:675–683`) calls `removeModule`, `modulesinit` and `Munmap`; a probe that asks the runtime or the loader whether the module is gone is reading the thing under test, which the repository's rules forbid. The plan should say the oracle is the OS mapping table (for example `/proc/self/maps` on Linux and the Mach region API on darwin) checked at the retired address range, and that the budget counts are taken at `Load` from segment lengths, not from the loader's bookkeeping after the fact. This is a mission statement, not a prescribed method.

## Scope notes

- The "next decision" section is internally consistent with the user's bounds except for finding 1, and it correctly refuses another loader survey or an escape-analysis project.
- Step 2's attributed ports of gopls snapshot, metadata-graph, parse-cache and memoize internals (lines 109–115) were not re-reviewed in depth here. They remain the largest body of unbounded work outside the checkpoint, and line 178's own prohibition on "a generic revision platform" is in tension with them. No finding is recorded because that step is not what changed.
- The plan's claim that `b487dd0` demonstrated the serial Mac fixture matches `native_test.go`; the plan does not overclaim concurrency or Linux.

## Outcome

material findings remain
