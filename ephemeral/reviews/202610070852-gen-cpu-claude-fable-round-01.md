# Adversarial review: generator resource claims (round 01)

Reviewer: Claude Fable 5.1, 2026-10-07. Read-only except this file.

## Target

`ephemeral/gen-cpu-20261007/claims-for-review.md` and the investigation beside
it (`README.md`, profiles, logs, overlay sources, `no-deps.patch`), reviewed
against `CLAUDE.md`, the implementation at `c6906b7` (HEAD `907d9c2` adds only
investigation artifacts), pinned `golang.org/x/tools@v0.50.0`, and the recorded
evidence. The user's question: can intensive work be done once and its result
reused without lowering confidence, and what else reduces test resource usage.

Operating constraints accepted: artifact path, do not edit other files. The
launch prompt did not narrow subject matter; nothing was ignored.

## Evidence inspected

- Sources: `internal/gen/{main_test,load_params,shared_params,package_overlay,
  packages,scan,output,gen,endpoints_test,actions_test,shared_params_test,
  ownership_test,stalegen_test}.go`; `Justfile`; `.github/workflows/*.yml`.
- Pinned library source: `x/tools/go/packages/packages.go` (`refine`,
  lines 798–811), `golist.go` overlay handling; `go/types/package.go`
  (`Imports` doc); `testing.go:478` (`-test.parallel` default);
  `dave/dst/decorator/load.go:19` (default `LoadSyntax`).
- Recorded evidence: `default.{cpu,heap}.pprof` re-queried with
  `go tool pprof -top -cum -focus=…`; `default.cpu-cumulative.txt`,
  `default.alloc-cumulative.txt`, `default.resources.txt`,
  `default.test-durations.txt`, `default.tests.log` (103 PASS, 0 SKIP, 0 FAIL);
  `generation.log`, `no-deps.log`, `no-deps.contracts.log`, `no-deps.patch`,
  `probe_test.go.txt`, `observed_package_overlay.go.txt`; worklog
  `ephemeral/worklog/20261007-gen-cpu.md`.
- One reproduction of my own (scratchpad only, compiler overlay, no project
  file touched): a fresh `foreignFixture(t, "", {server.go: endpointSource})`
  generated once under the investigation's parse-observer hook. Result:

  ```
  allocated=301.3 MiB
  source=application      parses=2    bytes=4835
  source=standard-library parses=1090 bytes=11280051
  source=module-cache     parses=0
  wall 0.60 s, 0.90 s user, 0.29 s sys, 245 MiB peak
  ```

  This fixture has no `web/src/params.go`, so `readGoParamMatchers` returns at
  `load_params.go:220` and the `NeedDeps` configuration at line 249 is never
  constructed. The 1,090 standard-library parses therefore come from a
  different path (see Finding 1).

## Claim-by-claim

| # | Claim | Verdict |
|---|---|---|
| 1 | Suite cost is package analysis + memory churn | **Supported.** 694 CPU-s sampled, 115.6 GiB allocated; `Checker.Files` 59.6% + `parseFile` 37.2% of alloc_space. Arithmetic in README checks out. |
| 2 | Unchanged generation repeats dependency source analysis | **Supported for the example app** (params.go present): 1,182 stdlib + 212 module-cache parses per `Run`, twice. **Not shown for the suite's dominant fixture shape** — see Finding 1. |
| 3 | Flag-only shortcut unsafe; failure doesn't prove full analysis needed | **Supported, and the explanation is correct.** `go/types.Package.Imports()` on an export-data package "includes packages that provide package-level objects referenced by pkg" (package.go:69–71); `var _ params.Value` references nothing exported, so `params` vanishes from the graph. Nitpick on the panic — Finding 5. |
| 4 | Import metadata can preserve the guard | **Plausible, unmeasured; incomplete as the remedy** — it addresses one of two causes (Finding 1). |
| 5 | Identical fixtures can share one generation | **Correct in principle, immaterial in magnitude** — Finding 2. |
| 6 | Invocation-local validation reuse | **True but not a resource lever** — Finding 3. |
| 7 | One graph's result cannot certify another | **Correct**, and the review makes the distinction explicit below. |
| 8 | Child CPU cap does not cap the parent | **Correct** (`testing.go:478` captures `GOMAXPROCS(0)` at init; `os.Setenv` after start is invisible to the running scheduler). Remedy space incomplete — Finding 4. |

## Findings

### 1. Issue — suite-level attribution to `load_params.go:249` is inferred, not measured, and a second independent cause is live in most tests

README: "The direct cause is `internal/gen/load_params.go:249`." That sentence
is demonstrated only by the paired probe on the example app, which has a
`params.go`. The suite is mostly `foreignFixture` tests (42 call sites across
ten files) with no `params.go`, where line 249 is never reached. My probe shows
one such `Run` still parses 1,090 standard-library files and allocates ~300 MiB.

Mechanism (verified in source): `Run` sets `cfg.generation` (`gen.go`), so
`scan.go:225`, `packages.go:21` and `load_params.go:251` all take
`generation.overlay()`. For a fresh tree that overlay contains link-directory
mirrors (`output.go:221ff`) that do not yet exist on disk. `preserveDependencyExports`
bails at `package_overlay.go:53–55` (`os.IsNotExist → return noop`), leaving
`Config.Overlay` set, and go/packages then does
`exportDataInvalid := len(ld.Overlay) > 0` (`packages.go:802`) for **every**
package — so `needsrc` is true for all of stdlib even with no `NeedDeps`
anywhere. The README names this as "a second source-loading path worth
investigating… did not quantify that fallback separately". It is now
quantified, and it is the only path the majority of tests exercise.

The CPU profile cannot settle attribution by itself: 70.45% of samples sit under
`errgroup.(*Group).Go.func1` (goroutines spawned inside go/packages), so a
`-focus` on any skgo frame shows ≤1.65% (`gen.Run`), `readGoParamMatchers`
1.40 s. The investigation's suite-level attribution is therefore inference from
one fixture shape. Claim 4's candidate (metadata import graph in place of
`NeedDeps`) leaves Finding 1's path untouched.

**Evidence needed:** pprof labels (as `probe_test.go.txt` already does) keyed by
load site *and* by whether `preserveDependencyExports` returned noop, across one
full suite run; or the parse-observer with a caller tag. One run, many
assertions against its output — consistent with `CLAUDE.md`'s "an expensive
command runs the minimum number of times".

### 2. Issue — "do the test work once" is the smaller lever by two orders of magnitude, and the doc's framing inverts that

The user asked whether intensive work can be done once and reused. The honest
answer the investigation reaches only in its last paragraph: **the Go toolchain
already does the intensive work once** — compiler export data in `GOCACHE` is
the shared result — and skgo defeats that reuse twice (the `NeedDeps` request
and the blanket overlay invalidation). Restoring it needs no new machinery and
benefits real generation, not just tests.

By contrast, claim 5's concrete duplicate (`TestAServerRouteBecomesTheModuleKitCompiles`
/ `TestAppWithoutPageActionsStillHasAnActionRegistry`, `endpoints_test.go:31`,
`actions_test.go:8`) costs ~0.6 s and ~300 MiB uncontended (my probe is exactly
that fixture). The 5.93 s / 2.56 s figures quoted are contended wall times under
10-way parallelism; the README caveats this but still presents the pair as the
headline reuse opportunity. Sharing it is fine and safe (precedent:
`sync.OnceValues` + `packageTemp` in `main_test.go`), but it does not move the
suite.

Also unstated and worth stating: the suite-level "once" mechanism Go offers —
the test result cache — is deliberately disabled by `-count=1` in `Justfile:57`,
and correctly so: these tests exec `go`, `node` and read `node_modules` through
children, inputs the cache does not track. Re-enabling it would be a silent
confidence loss of exactly the kind `CLAUDE.md` forbids.

### 3. Nitpick — claim 6 is true but not a resource claim

`validateSharedType` (`shared_params.go:226`) is a pointer walk over an
already-built `types.Package` graph; its only real cost is
`sharedForbiddenPackages → findSourceFiles` (a filesystem walk) per route
parameter. Nothing in either profile points at it. Listing it beside the
1.1 GiB `NeedDeps` cost as a "candidate" invites building a memo layer that
saves microseconds. Drop it from the resource plan; if it is done at all it is
a tidy-up, not an optimisation.

### 4. Issue — the two largest unmentioned levers are GC and process-level parallelism, both one measurement away

- `default.resources.txt`: **645 s sys vs 486 s user** over the whole process
  tree. More kernel time than user time is the signature `main_test.go:24–28`
  itself describes for children at default GOMAXPROCS — and the parent runs at
  10, with `-test.parallel` 10, inside `go test ./... ./example/...` which runs
  packages concurrently at `-p` = 10. Three multiplicative concurrency knobs,
  none of them set by the thing that can set them (the invocation). Remedy
  belongs in the `Justfile`/env (`GOMAXPROCS=N go test`, `-parallel`, `-p`),
  not in `TestMain`.
- In-process flat CPU is dominated by the collector: `tryDeferToSpanScan`
  24.8%, `spanInlineMarkBits.init` 16.3%, `memclrNoHeapPointers` 13.5%, sweep
  18.8%, `gcAssistAlloc` 13.9% (assist = allocators throttled, GC running
  back-to-back). `GOGC` is never mentioned. Raising it trades RSS (already
  8.2 GiB peak, 24 GiB machine shared with other agents) for CPU, so it is a
  hypothesis for *after* Finding 1's allocation drop, not before.

Both are hypotheses. Each needs exactly one paired run of the existing probe
or the suite with `/usr/bin/time -l`, asserted many ways from the saved output.

### 5. Nitpick — the no-deps experiment established one failure, not "only one"

`TestSharedParamsSourceDiagnostics` calls `prepareLoadParams(&cfg, …)` directly
with a `Config` whose `Logf` is nil (`Run` installs the default at `gen.go:65`,
the test bypasses `Run`). When the guard was removed the case proceeded to write
and panicked on `Logf`, which aborted the process and left
`TestTypedLoadActualParameterDependencies` paused. The README says so; the
claims document's phrasing ("the regression case runs once per full suite")
reads as if that case were the only thing the shortcut breaks. Nothing in the
record shows the rest of the suite under `no-deps`. Any real change must run
the whole package. Separately, the test's final section asserts the **Go
compiler's** `import cycle` message is surfaced with the `params.go` location —
that is the evidence that the forbidden-prefix guard is a diagnostic layer with
the compiler as backstop, which answers the "unnecessary policy?" question
below.

## Answers to the questions asked

**What survives.** Claims 1, 3, 7, 8 as stated. Claim 2 for the example app
only. Claim 4 as a candidate for one of two causes. Claims 5 and 6 as true
statements that are not where the resources go.

**What "once" safely means.**

- *Within a test process:* once per `(fixture inputs, starting state)`, shared
  through `sync.Once` into `packageTemp` — the pattern `main_test.go`,
  `evolution_test.go`, `check_test.go` and
  `prerender_production_fixture_test.go` already use. The boundary is exactly
  the one the investigation draws: any test whose claim *is* a transition
  (language switch, deletion, stale syntax/symbols, idempotence, edited matcher
  type) keeps its own invocation, because the second `Run` is the assertion.
  Confidence is preserved because each test still reads real generated bytes
  and asserts its own fixture-anchored expectation (`CLAUDE.md`: "never derive
  what you expect from the thing you are testing").
- *Across processes:* never, without tracked inputs. Keep `-count=1`. Do not
  build a persistent skgo cache of type-checked dependencies: that cache
  already exists (export data in `GOCACHE`), it is keyed by the go command on
  real inputs, and skgo's job is to stop defeating it.
- *For real generation:* once per dependency per toolchain/build-cache state —
  i.e. export data — for everything below the application; source analysis
  once per invocation for the application's own packages. The import-ownership
  guard needs the *import graph*, which `go list -deps` metadata gives for
  roughly the cost of one subprocess; it never needed dependency type-checking.

**Preserving checks vs paying for policy.** The policy is justified and cheap;
the enforcement mechanism is what is expensive. The forbidden-prefix rule keeps
generated `params` and route files acyclic and matcher domains leaf-shaped, and
the compiler's own `import cycle` error is the backstop the test already pins.
Keep every diagnostic and its `params.go:` location. Stop computing it from
`types.Package.Imports()` under `NeedDeps`.

One unverified observation while reading `writeLoadParams` (`load_params.go:355`):
the generated per-route file imports the *matcher package*, so a matcher package
that transitively (via a non-type helper) reaches a route directory also forms a
cycle; the current walk starts from the result type's package and would not see
it, whereas a metadata closure over the matcher package would. I have not
reproduced this; treat it as a question for whoever maps the guard against the
metadata graph, not as a finding.

## Resource-reduction recommendations

Demonstrated (by the recorded evidence or my probe), in order of expected yield:

1. **Stop requesting dependency syntax/type-info in `readGoParamMatchers`.**
   Paired probe: 1,504 → 388 MiB and 12.1 → 4.3 s per real example generation,
   zero stdlib/module-cache source parses. Replace the guard's graph source with
   `NeedName|NeedImports|NeedDeps` metadata (one cheap load; `Package.Imports`
   are real packages with `PkgPath`, errors preserved via `-e`). Acceptance:
   every case of `TestSharedParamsSourceDiagnostics` including the final
   compiler-cycle assertion, `TestTypedLoadActualParameterDependencies`, and
   the whole `./internal/gen` package green; the paired probe numbers re-recorded.
2. **Stop the blanket overlay invalidation on fresh trees.** My probe: 1,090
   stdlib parses and ~300 MiB per `Run` with no `params.go` at all; this is the
   path every `foreignFixture` test and every first-time real generation takes.
   The fine-grained check is an upstream TODO (`packages.go:801`); locally the
   question is why not-yet-existing files are in the overlay at `Run` time when
   `Run` writes generated files to disk anyway (`writeSharedParams` /
   `writeLoadParams` non-ReadOnly branches) and `a.links.sync()` can
   materialise link mirrors (as it does when `generation == nil`,
   `scan.go:234`). Ordering writes before loads, or excluding
   disk-backed paths from the overlay once written, would let
   `preserveDependencyExports` keep export data. Verify with the parse observer:
   stdlib parses must go to zero for the `foreignFixture` shape, and
   `TestReadOnlyCheckReportsAuthoredSourceErrorsWithoutWrites` / the `check_test`
   lane must stay green because ReadOnly mode genuinely needs the overlay.
3. **Measure before touching either.** One full-suite run with pprof labels per
   load site and per overlay-fallback decision, asserted many times from the
   saved profile. That turns Finding 1 from inference into a number and ranks 1
   against 2.

Hypotheses, one paired measurement each, after 1–2 land (their effect sizes
change once allocation drops):

4. Parallelism from the invocation: `GOMAXPROCS`/`-parallel`/`-p` in the
   `Justfile` recipe, not `TestMain`. Watch sys time in `/usr/bin/time -l`.
5. `GOGC` for `gen.test` once peak RSS is lower. Assert both CPU and max RSS.
6. Share the endpoint/action fixture pair and audit the remaining
   `foreignFixture` callers for identical `(extra files, remote, cfg)` tuples;
   expect single-digit seconds total. Do it because it is tidy, not because it
   is the fix.

Do not: re-enable the Go test cache; add a reduced "fast" suite beside `just
test`; build a persistent skgo analysis cache; memoise `validateSharedType` as
a performance measure; apply `no-deps.patch`.

## Outcome

`material findings remain`
