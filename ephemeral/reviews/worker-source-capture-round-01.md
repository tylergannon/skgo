# Adversarial review: Worker source-capture optimization (round 01)

## Target

- Branch `codex/worker-source-capture`, commit `dc681c8` "perf(generate): retain unchanged captured source bytes" (the working tree was uncommitted at session start and became this commit during the review; `git diff HEAD` is empty, so the reviewed content is exactly the commit).
- Files: `internal/gen/cache_source.go`, `internal/gen/worker.go`, `internal/gen/worker_capture.go`, `internal/gen/worker_capture_{darwin,linux,other}.go`, `internal/gen/worker_capture_test.go`, `ephemeral/worker-capture-probe/{main.go,logs/*}`, `ephemeral/worklog/worker-source-capture.md`.
- Specification: issue #311 ("Make SKGO development edits feel as fast as SvelteKit..."), repository `CLAUDE.md`.
- Out of scope but on the same branch: `d69891c` "perf: load dev SSR route modules on demand" was not reviewed here and will ship in the same PR unless split.

Caller narrowing: none ignored. The launch prompt named the artifacts to inspect and the read-only boundary; scope was derived from the issue and `CLAUDE.md`.

## Evidence inspected

- Full diff of `dc681c8` and surrounding code: `cacheSources.capture`/`captureWithRead`, `cacheOwned`, `materialize`, `acquireGenerationSources`, the second validation capture in `internal/gen/cache.go:263`, `Worker.Run`/`Close`.
- Worklog and all six probe logs. Recomputed medians from the logs: body-edit medians 239.056 / 224.242 / 202.705 ms and 37.45 / 31.16 / 11.87 MB allocated for baseline / predicate / retained. These match the worklog's stated numbers. Every log has 12 `MEASURE` lines and no error.
- `go vet ./internal/gen`: clean. `gofmt -l`: only pre-existing testdata fixtures.
- `go test ./internal/gen -run 'TestWorkerCapture|TestWorkerSourceCapture' -race -count=1` on macOS 27 / APFS: pass.
- `go test ./internal/gen -count=1` on macOS: `ok` in 106 s.
- Cross-compiled `internal/gen` test binary (`GOOS=linux GOARCH=arm64 CGO_ENABLED=0`) and ran it in `debian:bookworm-slim` under Docker (kernel `6.12.76-linuxkit`, `CLK_TCK=100`):
  - `TestWorkerCaptureReusesOnlyCurrentFileVersions`: **FAIL 5 of 5 runs** at `worker_capture_test.go:54` with `capture="first" ... want "other"`.
  - `TestWorkerSourceCaptureRetainsBytesWithoutLosingMembership`: pass 3 of 3.
- Load-bearing check of the tests (by reasoning against the code, not by mutation, since the tree is read-only):
  - Removing retention makes `reads != 1` fail (on darwin/linux). Dropping the ctime term makes the preserved-mtime edit return stale bytes and fail. Dropping the post-read re-stat makes the "read racing a write was retained" assertion fail. The authoritative-generation comparison test fails if a retained capture ever diverges from a fresh `Run`. The tests are load-bearing on macOS, and on Linux they are load-bearing enough to expose the defect below.

## Findings

### 1. Critical — verifiable bug: stale source bytes are served on Linux when a same-size in-place edit lands within one kernel timestamp tick

`internal/gen/worker_capture.go:23-25` treats a file as unchanged when ctime, size, mtime and inode identity all match. The header comment (`worker_capture.go:8-10`) says size and mtime are insufficient and relies on ctime as the kernel's own revision stamp. On Linux without multigrain timestamps (everything before 6.13, including `ubuntu-latest` runners and this Docker kernel), ctime and mtime are coarse-clock values with jiffy granularity (10 ms at `CLK_TCK=100`). Two in-place writes of the same length inside one tick produce identical `Stat_t`, so `read` returns the previous bytes from `r.previous` and never calls `readFile`.

Reproduction: the project's own test. Cross-compile and run `TestWorkerCaptureReusesOnlyCurrentFileVersions` on Linux:

```
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/gen.test ./internal/gen
docker run --rm -v /tmp/gen.test:/gen.test:ro debian:bookworm-slim /gen.test -test.run TestWorkerCaptureReusesOnlyCurrentFileVersions -test.v
```

Output, five of five runs:

```
worker_capture_test.go:54: capture="first" error=<nil>; want "other"
--- FAIL: TestWorkerCaptureReusesOnlyCurrentFileVersions (0.00s)
```

Impact:

- CI (`.github/workflows/ci.yml` job on `ubuntu-latest`) will fail on this branch; the change cannot merge as is.
- Production: the development Worker on Linux can generate from stale source for tool-driven edit sequences (format-on-save rewrites, scripted rewrites, tests that write and rerun) and, more seriously, the post-generation validation at `internal/gen/cache.go:263` now revalidates through the same stat path, so a write that lands during generation inside the same tick as the pre-read stat is not detected. The post-read re-stat guard at `worker_capture.go:41-45` fails for the same reason. Issue #311 states "No stale results or manual cache deletion as the recovery mechanism"; this violates it on the platform CI and most deployments run.
- The implementer evidently only ran the suite on macOS/APFS, whose nanosecond ctime hides the problem.

Direction, not prescription: Git's "racy git" rule is the established precedent for exactly this trap (an entry whose stamp is not older than the capture time is treated as suspect and re-read). Any fix must keep both tests green on Linux and macOS, and CI must actually exercise the Linux path rather than skipping it.

### 2. Issue — cost/benefit of the retention layer is thin against the issue's priorities and now carries platform-dependent correctness

Issue #311 says avoiding generation whose inputs did not change is the priority and that "making a whole-project pass faster is secondary", and asks to "measure the complete visible reload". The worklog is candid that the measurements exclude compilation, startup, rendering and browser reload. In the same issue's own data, one Gimbal body-edit build is about 3.2 s with 2.9 s linking; the retained-bytes layer saves about 22 ms per warm `Worker.Run` beyond the predicate reorder (224 ms to 203 ms). The predicate reorder (`cache_source.go:410`, one-line, semantics-preserving: `generatedArtifact` is a prefix test, `withinTree` does two `EvalSymlinks`) delivers 15 ms with no new invariants. The retention layer buys 22 ms and 19 MB less allocation at the price of three platform files, a kernel-identity contract, and the Linux defect in finding 1. The worklog's own decision rule was to keep the byte cache only if paired measurements showed useful improvement beyond the reorder; 9.6 % of a 224 ms stage inside a multi-second loop is a judgment call the reviewer would resolve the other way unless finding 1 is fixed without reintroducing reads. Report this to the decision owner rather than treating it as settled.

### 3. Nitpick — a silent platform fallback can turn the retention test into a no-op

`worker_capture_test.go:26` computes `retains` from `captureChangeTime` and guards the only "unchanged input was not re-read" assertion with it (`worker_capture_test.go:47`). On darwin/linux that is currently true, but if the `*syscall.Stat_t` assertion ever stops holding (for example a future `os.FileInfo` implementation), the optimization silently switches off and the test still passes. `CLAUDE.md`: "skips are failures". Prefer asserting `retains` unconditionally on the two platforms that ship a ctime accessor.

### 4. Nitpick — the Gimbal-specific probe lives inside skgo's main module

`ephemeral/worker-capture-probe/main.go` hardcodes Gimbal's `LocalsPackage`/`HookPackage` and reads `native/skgo-native.json`, yet sits in the skgo module without its own `go.mod`, so `go build ./...`, `go vet ./...` and `just test` (`./...`) compile it in CI. The prior experiment under `ephemeral/experiments/20261004-declared-inputs-consumers/deliveryproof/` used a separate `go.mod`; follow that precedent or exclude the directory.

## Outcome

material findings remain
