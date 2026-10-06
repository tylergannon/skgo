# Typed-load evolution fixture slice: independent validation

Validated 2026-10-06 on branch `codex/issue-247-typed-load-fixture` at HEAD
`d034f77`, uncommitted. The changes checked were the edited
`internal/gen/load_params_test.go`, the new
`internal/gen/typed_load_fixture_test.go` and the new
`internal/gen/testdata/typed-load-evolution/`. I checked them against
`ephemeral/plans/20261006-issue-247-typed-load-slice.md` and `AGENTS.md`
(byte-identical to `CLAUDE.md`). Environment: go1.27.1 darwin/arm64, 10 CPUs.

## Verdict: accepted

I found no material gaps. All three things the slice set out to do were shown by
running the code:

- **The fixture is isolated.** A compile-breaking edit to an unrelated example
  route (`about`) did not stop the changed test, which passed (U1).
- **The example still rejects that edit.** The example's own `go tool skgo
  generate` failed on the edit (U2).
- **The old test was blocked by it.** HEAD's version of the test, which copied
  the whole example, failed at its first `Run` with that same edit (U3). This
  slice removes that blockage.

Six separate faults injected into the generator were each caught at the
assertion that guards them (M1–M5, M6b). That includes both literal
served-handler values and the separate initial, refreshed-type and
refresh-ordering checks.

## What the fixture is

`stageTypedLoadEvolutionFixture` copies `testdata/typed-load-evolution` and
then six bootstrap files from `example/`: `go.mod`, `go.sum`, `package.json`,
`vite.config.ts`, `tsconfig.json` and `app.html`. It points the `replace` and
the adapter `link:` at the repository and links the pinned Kit installation.

- **Owned by the fixture:** `src/params.go` (`package hooks`, `OrderNumber`,
  `Label`, `Order`), `src/params.ts` (identical to the example's), the two
  routes cut down to `.Label()` consumers, and `internal/skgo/config.go`.
- **Not copied:** any example route, hook, layout or matcher consumer.
- **Type identities kept:** the module stays `github.com/tylergannon/skgo/example`,
  and the matcher package stays `.../example/web/src`, imported as `hooks`.
  The error paths in M2 and M1 confirm both.

The test still makes three generator attempts:

1. The initial run, which writes `hooks.OrderNumber` for `Number`, `A` and `B`.
2. After the matcher changes to `RevisedOrder`, a run that fails on `Label`
   with both params files already refreshed.
3. After both handlers are repaired, a run that succeeds, followed by a child
   `go test`. That child test serves real `__data.json` requests and checks the
   literals `Revised order #42`, `Revised order #7` and, for `?read=b`,
   `Revised order #42`.

## Experiments

Each experiment ran the single test
`go test -count=1 -run '^TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation$' -v ./internal/gen`
under `/usr/bin/time -l`, except U2, which is listed separately. Each mutation
was restored from a scratch copy straight after its run. Complete logs are in
the appendix.

| ID | Temporary mutation | Expected | Actual outcome | Real time |
|---|---|---|---|---|
| U1 | `example/web/src/routes/about/page.server.go`: appended `var _ int = "unrelated demo consumer is broken"` | changed test unaffected | **PASS** (2.47s test) | 4.09s |
| U2 | same as U1; ran `go tool skgo generate --web ../../web` in `example/internal/skgo` | example generation rejects it | **exit 1**: `page.server.go:33:13: cannot use "unrelated demo consumer is broken" … as int value` | 2.41s |
| U3 | same as U1 + `load_params_test.go` replaced by `git show HEAD:` version | old test is blocked | **FAIL** at `load_params_test.go:49` (first `Run`) with the same compile error | 3.23s |
| M1 | `writeLoadParams`: matcher type replaced by `.Underlying()` (named type erased) | caught | **FAIL** at `:49`: `event.Params.B().Label undefined (type int64 …)`. The first `Run` compiles the fixture's `.Label()` calls | 1.85s |
| M2 | `writeLoadParams`: return without writing if the params file exists (no refresh) | caught | **FAIL** at `:87`: the error was `undefined: hooks.OrderNumber`, not `Label` | 2.33s |
| M3 | `Run`: snapshot `skgo_params_gen.go` files and write them back when the run fails (refresh not kept past the compile failure) | caught at stale-type check | **FAIL** at `:95` "stale matcher type remains after handler compilation failed". The `Label` check at `:86` passed first | 2.41s |
| M4 | `writeLoadParams`: struct matcher results written as their structural type | caught at refreshed-type check | **FAIL** at `:100` "new matcher type was not written…" (`struct{ Number int64 }`). Both `:86` and `:94` passed first | 2.97s |
| M5 | `writeLoadParams`: `valueB` filled from route param `a` | caught by `?read=b` literal | **FAIL** in the child test: `?read=b` returned `"Revised order #7"`, status 200, `uses.params:["b"]` | 3.91s |
| M6 | `writeLoadParams`: accessors filled with `*new(T)` | — | **Invalid mutation**: build failed (`declared and not used: helper`). Replaced by M6b | 0.33s |
| M6b | M6 plus `_ = helper` | caught by literals | **FAIL** in the child test: `/typed-load/42/__data.json` returned `"Revised order #0"`, status 200 | 3.78s |
| M7 | `writeLoadParams`: private storage prefix `value`→`stored` | — | **FAIL** at `:68` "initial named matcher type missing" | 1.95s |

No test was skipped: every log shows `=== RUN` for the target test, and each
failing log names the failing line. After all experiments:

- `git status --porcelain --untracked-files=all` matched the pre-experiment
  capture.
- The SHA-256 of `git diff` matched.
- Checksums of all 12 sources touched or inspected matched.
- `grep VALIDATION-ONLY` over `internal` and `example/web/src` found nothing.
- `gofmt -l` on both changed test files printed nothing.

## How much each assertion protects

- **Refresh before the compile failure** (`:94`, `:99` over both routes) is
  load-bearing. M3 is the exact regression the test is named for: refreshed
  types rolled back when compilation fails. Only `:95` caught it. M4 shows
  `:99` separately catches a refresh that drops the named type, even when `:94`
  passes.
- **The second attempt must fail on `Label`** (`:86`) is load-bearing. M2 shows
  a run that never refreshes fails for the wrong reason, and `:86` rejects it.
- **Literal served values** are load-bearing and do not come from what the
  generator wrote. M6b's zero values fail the `#42` check. M5's B/A mix-up fails
  only the `?read=b` → `#42` literal: tracking still recorded `b`, so only the
  value check could catch it.
- **The initial named-type check** (`:60–71`) is a check on the generated text.
  Semantic erasure of the named type (M1) is caught first by the initial `Run`,
  because the fixture's own `.Label()` calls must compile. The text check fails
  only on changes to the generated shape (M7). This is not a gap: it fixes the
  starting point that `:94` and `:99` compare against, and it covers `A`/`B` as
  well as `Number`, which HEAD's version did not.

## Timing (reusing the existing logs, not re-run)

`ephemeral/issue-247-typed-load-baseline.log` (HEAD, whole-example copy):

- test 16.42s, process 18.86s real
- 18.50s user, 14.96s sys
- max RSS 305,561,600 B

`ephemeral/issue-247-typed-load-after.log` (this slice):

- test 2.41s, process 4.03s real
- 3.83s user, 2.95s sys
- max RSS 304,627,712 B

Both cover all three generation attempts and the child `go test`. Neither has
a per-phase breakdown or records the exact command line. The test time in my
U1 run (2.47s) agrees with the post-change log.

## Encountered, not in this slice, not repaired

1. **A failed example generation leaves `example/internal/skgo/links` dirty.**
   U2's failed run left 49 tracked files under `example/internal/skgo/links/`
   modified:
   - `skgo_remotes_gen.go` mirrors blanked down to their header
   - the broken `about/page.server.go` copied into the mirror

   `Run`'s `restore()` (`gen.go`, `resetStaleGeneratedFiles`) brings back the
   blanked originals under `web/src`, but not the copies in the link mirror. So
   the comment "a failed run leaves the tree exactly as it found it" does not
   hold for `internal/skgo/links`. The generator is unchanged in this slice, so
   this predates it. I restored the mirror with
   `git checkout -- example/internal/skgo/links` and confirmed the status
   matched its pre-U2 state.
2. **The child `go test` would pass silently if its test were renamed.** The
   child run's success output is not asserted, so renaming `TestRepairedTypedLoad`
   without updating `-run` would print `no tests to run` and exit 0. This
   predates the slice (that code is unchanged), and the risk is small because
   the name and the pattern sit in the same function. Not material.
3. **Minor bootstrap coupling.** The fixture also copies `vite.config.ts` and
   `package.json`, and rewrites the adapter `link:` with `replaceOnce`. That call
   errors if the example ever respells that link. I did not check whether
   generation in this test needs these two files. This bootstrap coupling is
   what the plan allows. Not material.

## Appendix: complete command output

Whitespace normalized for Markdown: tabs expanded and trailing spaces removed.

### U1-new-test-broken-demo

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
--- PASS: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (2.47s)
PASS
ok      github.com/tylergannon/skgo/internal/gen    2.893s
        4.09 real         2.44 user         2.56 sys
           303693824  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              152460  page reclaims
               18238  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1473  signals received
                5922  voluntary context switches
               36616  involuntary context switches
          4416117333  instructions retired
          2872033925  cycles elapsed
            29803192  peak memory footprint
```

### U2-example-generate-broken-demo

```text
skgo: copied internal/skgo/links/onzggl3sn52xizlt/skgo_remotes_gen.go from web/src/routes/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf4ug2ylsnnsxi2lom4us64dsnfrws3th/skgo_remotes_gen.go from web/src/routes/(marketing)/pricing/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf52g6zdpom/skgo_remotes_gen.go from web/src/routes/todos/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf52g6zdpomxwoylumu/skgo_remotes_gen.go from web/src/routes/todos/gate/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_remotes_gen.go from web/src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwy33bmqxvw3tvnvrgk4r5j5zgizlslu/skgo_remotes_gen.go from web/src/routes/typed-load/[number=Order]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwe33voq/page.server.go from web/src/routes/about/page.server.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwe33voq/skgo_remotes_gen.go from web/src/routes/about/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhg/skgo_remotes_gen.go from web/src/routes/actions/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3dojxxg4zpojswgzljozsq/skgo_remotes_gen.go from web/src/routes/actions/cross/receive/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3emvtgc5lmoq/skgo_remotes_gen.go from web/src/routes/actions/default/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3emvtgc5lmoqxxgylwmvsa/skgo_remotes_gen.go from web/src/routes/actions/default/saved/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3pob2gs33oomxw43znmnwgszlooq/skgo_remotes_gen.go from web/src/routes/actions/options/no-client/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3pob2gs33oomxw43znonzxe/skgo_remotes_gen.go from web/src/routes/actions/options/no-ssr/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3qojxwm2lmmvzs6w3qojxwm2lmmvoq/skgo_remotes_gen.go from web/src/routes/actions/profiles/[profile]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3tnftw4zlefvuw4/skgo_remotes_gen.go from web/src/routes/actions/signed-in/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhi/skgo_remotes_gen.go from web/src/routes/account/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3pojsgk4tt/skgo_remotes_gen.go from web/src/routes/account/orders/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3smvzhk3ttf5nxg3dvm5oq/skgo_remotes_gen.go from web/src/routes/account/reruns/[slug]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3torqxizlnmvxhi/skgo_remotes_gen.go from web/src/routes/account/statement/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxa2jpmzqxiylm/skgo_remotes_gen.go from web/src/routes/api/fatal/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxa3dbpewwg33vnz2a/skgo_remotes_gen.go from web/src/routes/api/replay-count/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxa3dbpexvw23jnzsf2/skgo_remotes_gen.go from web/src/routes/api/replay/[kind]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxc5lfon2c2ztforrwq/skgo_remotes_gen.go from web/src/routes/api/request-fetch/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxa2jporxwi33t/skgo_remotes_gen.go from web/src/routes/api/todos/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxg6lommwxg43sf5nw233emvos6w3hojxxk4c5/skgo_remotes_gen.go from web/src/routes/async-ssr/[mode]/[group]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5qxg6lommwxg43sf5rw63tuojxwyl23m5zg65lqlu/skgo_remotes_gen.go from web/src/routes/async-ssr/control/[group]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5rgc5ddna/skgo_remotes_gen.go from web/src/routes/batch/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5rw63tumfrxi/skgo_remotes_gen.go from web/src/routes/contact/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sg6y3tf5ns4lroojsxg5c5/skgo_remotes_gen.go from web/src/routes/docs/[...rest]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sgk43unfxgc5djn5xhg/skgo_remotes_gen.go from web/src/routes/destinations/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sw24dupe/skgo_remotes_gen.go from web/src/routes/empty/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwe33vnzsgc4tz/skgo_remotes_gen.go from web/src/routes/error/boundary/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwg33nnvqw4za/skgo_remotes_gen.go from web/src/routes/error/command/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwk6dqmvrxizle/skgo_remotes_gen.go from web/src/routes/error/expected/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixxezlenfzgky3u/skgo_remotes_gen.go from web/src/routes/error/redirect/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixxk3tfpbygky3umvsa/skgo_remotes_gen.go from web/src/routes/error/unexpected/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5tw6llemv3a/skgo_remotes_gen.go from web/src/routes/go-dev/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5tw6llemv3c6zlomryg62looq/skgo_remotes_gen.go from web/src/routes/go-dev/endpoint/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5uw45tpnfrwk4y/skgo_remotes_gen.go from web/src/routes/invoices/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5uxizlnomxvw2lelu/skgo_remotes_gen.go from web/src/routes/items/[id]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5wgs5tf/skgo_remotes_gen.go from web/src/routes/live/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5wwszdenrsxoylsmu/skgo_remotes_gen.go from web/src/routes/middleware/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5wwszdenrsxoylsmuxvw43movtv2/skgo_remotes_gen.go from web/src/routes/middleware/[slug]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a/skgo_remotes_gen.go from web/src/routes/optional/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5yhezlsmvxgizlsf5nxg3dvm5oq/skgo_remotes_gen.go from web/src/routes/prerender/[slug]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5zgk4lvmvzxillgmv2gg2a/skgo_remotes_gen.go from web/src/routes/request-fetch/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5zwqylen53s6wzofyxhezltoroq/skgo_remotes_gen.go from web/src/routes/shadow/[...rest]/skgo_remotes_gen.go
skgo: copied internal/skgo/links/onzggl3sn52xizltf5zxi4tfmfwq/skgo_remotes_gen.go from web/src/routes/stream/skgo_remotes_gen.go
skgo: github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq: -: # github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq
internal/skgo/links/onzggl3sn52xizltf5qwe33voq/page.server.go:33:13: cannot use "unrelated demo consumer is broken" (untyped string constant) as int value in variable declaration
        2.41 real         4.50 user         3.18 sys
           329302016  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              244591  page reclaims
                2756  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1952  signals received
                2520  voluntary context switches
               39191  involuntary context switches
          3048770622  instructions retired
          2970463695  cycles elapsed
            32064184  peak memory footprint
```

### U3-head-test-broken-demo

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:49: skgo: github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq: -: # github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq
        internal/skgo/links/onzggl3sn52xizltf5qwe33voq/page.server.go:33:13: cannot use "unrelated demo consumer is broken" (untyped string constant) as int value in variable declaration
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (1.81s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    2.159s
FAIL
        3.23 real         4.25 user         2.99 sys
           298975232  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              222265  page reclaims
                1308  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1579  signals received
                1940  voluntary context switches
               27832  involuntary context switches
          4134848882  instructions retired
          5082135116  cycles elapsed
            32375504  peak memory footprint
```

### M1-underlying-type

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:49: skgo: github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi: -: # github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/page.server.go:12:43: event.Params.B().Label undefined (type int64 has no field or method Label)
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/page.server.go:14:42: event.Params.A().Label undefined (type int64 has no field or method Label)
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (0.43s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    0.787s
FAIL
        1.85 real         2.32 user         2.08 sys
           298975232  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
               79425  page reclaims
                1291  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1181  signals received
                 708  voluntary context switches
               15191  involuntary context switches
          4131731432  instructions retired
          4861860048  cycles elapsed
            30278328  peak memory footprint
```

### M2-no-refresh

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:87: stale handler must fail compilation after params refresh: skgo: github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi: -: # github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:15:21: undefined: hooks.OrderNumber
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:16:21: undefined: hooks.OrderNumber
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:20:32: undefined: hooks.OrderNumber
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:22:32: undefined: hooks.OrderNumber
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:30:43: undefined: hooks.OrderNumber
        internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_params_gen.go:31:43: undefined: hooks.OrderNumber
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (0.90s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    1.273s
FAIL
        2.33 real         2.68 user         2.39 sys
           304644096  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              109153  page reclaims
                1378  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1268  signals received
                 795  voluntary context switches
               18424  involuntary context switches
          4105100769  instructions retired
          4699586602  cycles elapsed
            31343312  peak memory footprint
```

### M3-refresh-rolled-back

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:95: stale matcher type remains after handler compilation failed: // Code generated by skgo. DO NOT EDIT.

        package typedload

        import skgo "github.com/tylergannon/skgo"

        import (
            hooks "github.com/tylergannon/skgo/example/web/src"
        )

        // RouteParams stores converted values. Only accessor reads record dependencies
        // on the load invocation that constructed these params.
        type RouteParams struct {
            event       *skgo.Event
            valueNumber hooks.OrderNumber
        }

        func (p RouteParams) Number() hooks.OrderNumber {
            skgo.TrackLoadParam(p.event, "number")
            return p.valueNumber
        }

        type RequestEvent = skgo.RequestEvent[RouteParams]

        func SkgoRequestEvent(event *skgo.Event) RequestEvent {
            return RequestEvent{Event: event, Params: RouteParams{event: event,
                valueNumber: skgo.LoadParamValue[hooks.OrderNumber](event, "number"),
            }}
        }

        func SkgoParamMatchers() map[string]skgo.ParamMatcher {
            return map[string]skgo.ParamMatcher{
                "Order": func(value string) (any, bool) { return hooks.Order(value) },
            }
        }
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (0.90s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    1.324s
FAIL
        2.41 real         2.70 user         2.60 sys
           305561600  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              109050  page reclaims
                1377  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1299  signals received
                 727  voluntary context switches
               19016  involuntary context switches
          4126182650  instructions retired
          5315729829  cycles elapsed
            29917904  peak memory footprint
```

### M4-structural-type

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:100: /var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation4138470603/001/web/src/routes/typed-load/[number=Order]/skgo_params_gen.go: new matcher type was not written before stale handler failed: // Code generated by skgo. DO NOT EDIT.

        package typedload

        import skgo "github.com/tylergannon/skgo"

        import (
            hooks "github.com/tylergannon/skgo/example/web/src"
        )

        // RouteParams stores converted values. Only accessor reads record dependencies
        // on the load invocation that constructed these params.
        type RouteParams struct {
            event       *skgo.Event
            valueNumber struct{ Number int64 }
        }

        func (p RouteParams) Number() struct{ Number int64 } {
            skgo.TrackLoadParam(p.event, "number")
            return p.valueNumber
        }

        type RequestEvent = skgo.RequestEvent[RouteParams]

        func SkgoRequestEvent(event *skgo.Event) RequestEvent {
            return RequestEvent{Event: event, Params: RouteParams{event: event,
                valueNumber: skgo.LoadParamValue[struct{ Number int64 }](event, "number"),
            }}
        }

        func SkgoParamMatchers() map[string]skgo.ParamMatcher {
            return map[string]skgo.ParamMatcher{
                "Order": func(value string) (any, bool) { return hooks.Order(value) },
            }
        }
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (0.90s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    1.876s
FAIL
        2.97 real         2.69 user         2.52 sys
           305561600  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              110390  page reclaims
                1453  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1397  signals received
                 745  voluntary context switches
               19206  involuntary context switches
          4103745161  instructions retired
          5046812611  cycles elapsed
            30769848  peak memory footprint
```

### M5-b-reads-a

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:163: repaired load compile/behavior: exit status 1
        --- FAIL: TestRepairedTypedLoad (0.00s)
            typed_load_repaired_test.go:28: /typed-dependencies/7/42/first/__data.json?read=b: status 200 body {"type":"data","nodes":[{"type":"data","data":[{"label":1},"Revised order #7"],"uses":{"search_params":["read"],"params":["b"]}}]}
        FAIL
        FAIL    github.com/tylergannon/skgo/example/internal/skgo   0.413s
        FAIL
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (2.43s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    2.840s
FAIL
        3.91 real         3.72 user         3.17 sys
           304742400  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              186814  page reclaims
                2472  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1607  signals received
                 913  voluntary context switches
               25065  involuntary context switches
          4112666052  instructions retired
          4776565633  cycles elapsed
            30376656  peak memory footprint
```

### M6-zero-values

```text
# github.com/tylergannon/skgo/internal/gen [github.com/tylergannon/skgo/internal/gen.test]
internal/gen/load_params.go:301:3: declared and not used: helper
FAIL    github.com/tylergannon/skgo/internal/gen [build failed]
FAIL
        0.33 real         0.26 user         1.54 sys
            46841856  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
               12015  page reclaims
                   0  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                 468  signals received
                 262  voluntary context switches
                6631  involuntary context switches
          3850619577  instructions retired
          4527918189  cycles elapsed
            29393592  peak memory footprint
```

### M6b-zero-values

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:163: repaired load compile/behavior: exit status 1
        --- FAIL: TestRepairedTypedLoad (0.00s)
            typed_load_repaired_test.go:28: /typed-load/42/__data.json: status 200 body {"type":"data","nodes":[{"type":"data","data":[{"label":1},"Revised order #0"],"uses":{"params":["number"]}}]}
        FAIL
        FAIL    github.com/tylergannon/skgo/example/internal/skgo   0.312s
        FAIL
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (2.34s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    2.705s
FAIL
        3.78 real         3.71 user         3.21 sys
           302579712  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
              188178  page reclaims
                2418  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1530  signals received
                 897  voluntary context switches
               27926  involuntary context switches
          4105486036  instructions retired
          4894525410  cycles elapsed
            32670416  peak memory footprint
```

### M7-storage-renamed

```text
=== RUN   TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== PAUSE TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
=== CONT  TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation
    load_params_test.go:68: /var/folders/lt/09rsy64x65s_0fp2b8zq3n7m0000gn/T/TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation4249369491/001/web/src/routes/typed-load/[number=Order]/skgo_params_gen.go: initial named matcher type missing: // Code generated by skgo. DO NOT EDIT.

        package typedload

        import skgo "github.com/tylergannon/skgo"

        import (
            hooks "github.com/tylergannon/skgo/example/web/src"
        )

        // RouteParams stores converted values. Only accessor reads record dependencies
        // on the load invocation that constructed these params.
        type RouteParams struct {
            event        *skgo.Event
            storedNumber hooks.OrderNumber
        }

        func (p RouteParams) Number() hooks.OrderNumber {
            skgo.TrackLoadParam(p.event, "number")
            return p.storedNumber
        }

        type RequestEvent = skgo.RequestEvent[RouteParams]

        func SkgoRequestEvent(event *skgo.Event) RequestEvent {
            return RequestEvent{Event: event, Params: RouteParams{event: event,
                storedNumber: skgo.LoadParamValue[hooks.OrderNumber](event, "number"),
            }}
        }

        func SkgoParamMatchers() map[string]skgo.ParamMatcher {
            return map[string]skgo.ParamMatcher{
                "Order": func(value string) (any, bool) { return hooks.Order(value) },
            }
        }
--- FAIL: TestTypedLoadParamsRefreshBeforeStaleHandlerCompilation (0.47s)
FAIL
FAIL    github.com/tylergannon/skgo/internal/gen    0.849s
FAIL
        1.95 real         2.42 user         2.31 sys
           300990464  maximum resident set size
                   0  average shared memory size
                   0  average unshared data size
                   0  average unshared stack size
               81370  page reclaims
                1291  page faults
                   0  swaps
                   0  block input operations
                   0  block output operations
                   0  messages sent
                   0  messages received
                1167  signals received
                 690  voluntary context switches
               15557  involuntary context switches
          4153659970  instructions retired
          5444173902  cycles elapsed
            29557432  peak memory footprint
```

### U2 link-mirror files left modified (restored)

```text
example/internal/skgo/links/onzggl3sn52xizlt/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf4ug2ylsnnsxi2lom4us64dsnfrws3th/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf52g6zdpom/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf52g6zdpomxwoylumu/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwizlqmvxgizlomnuwk4zplnqt2t3smrsxexjplnrd2t3smrsxexjplnuwo3tpojswixi/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf52hs4dfmqwwy33bmqxvw3tvnvrgk4r5j5zgizlslu/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq/page.server.go
example/internal/skgo/links/onzggl3sn52xizltf5qwe33voq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhg/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3dojxxg4zpojswgzljozsq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3emvtgc5lmoq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3emvtgc5lmoqxxgylwmvsa/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3pob2gs33oomxw43znmnwgszlooq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3pob2gs33oomxw43znonzxe/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3qojxwm2lmmvzs6w3qojxwm2lmmvoq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhgl3tnftw4zlefvuw4/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhi/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3pojsgk4tt/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3smvzhk3ttf5nxg3dvm5oq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qwgy3povxhil3torqxizlnmvxhi/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxa2jpmzqxiylm/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxa3dbpewwg33vnz2a/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxa3dbpexvw23jnzsf2/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxa2jpojsxc5lfon2c2ztforrwq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxa2jporxwi33t/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxg6lommwxg43sf5nw233emvos6w3hojxxk4c5/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5qxg6lommwxg43sf5rw63tuojxwyl23m5zg65lqlu/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5rgc5ddna/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5rw63tumfrxi/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sg6y3tf5ns4lroojsxg5c5/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sgk43unfxgc5djn5xhg/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sw24dupe/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwe33vnzsgc4tz/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwg33nnvqw4za/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixwk6dqmvrxizle/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixxezlenfzgky3u/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5sxe4tpoixxk3tfpbygky3umvsa/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5tw6llemv3a/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5tw6llemv3c6zlomryg62looq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5uw45tpnfrwk4y/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5uxizlnomxvw2lelu/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5wgs5tf/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5wwszdenrsxoylsmu/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5wwszdenrsxoylsmuxvw43movtv2/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5yhezlsmvxgizlsf5nxg3dvm5oq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5zgk4lvmvzxillgmv2gg2a/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5zwqylen53s6wzofyxhezltoroq/skgo_remotes_gen.go
example/internal/skgo/links/onzggl3sn52xizltf5zxi4tfmfwq/skgo_remotes_gen.go
```
