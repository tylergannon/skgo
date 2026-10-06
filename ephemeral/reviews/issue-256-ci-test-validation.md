# Issue 256 CI test repair validation

Reviewed the frozen repair pushed as `a6e2366`, against issue #256 and the user's correction recorded in `ephemeral/worklog/prerender-service.md`. No material assertion gaps found.

The successful declared-input build now requires exactly one anchored native helper start line and one stop line with the same valid PID. On this Darwin host, it then calls the existing process-group assertion immediately after `vp build` returns. That assertion accepts only ESRCH; a live group or another syscall error fails. A logged stop alone therefore cannot satisfy successful cleanup.

The one-second writer and post-build wait are absent. The test retains independently chosen producer receipts, exactly one body call for each atlas/beacon input despite the duplicate atlas declaration, one reused page remote call, transported `$1.25`, no-argument output, literal native artifact payloads and redirect destination, error policy checks, JavaScript-mode artifact checks and the compiled Goja handler check. Missing execution or wrong artifacts still fails the existing test. Dedicated descendant, failure and interruption lifecycle tests retain responsibility for their separate cleanup behavior; this repair does not claim a new run of them.

Ran the changed existing test once, after the push, with a completion watcher started alongside it. Captured command and exit status in `ephemeral/declared-input-ci-fix.log`:

```text
go test -count=1 -v ./internal/adapter -run '^TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts$' -timeout=180s
=== RUN   TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts
--- PASS: TestDeclaredPrerenderInputsBuildAndProduceNativeArtifacts (25.89s)
PASS
ok github.com/tylergannon/skgo/internal/adapter 26.204s
go test declared-input exit status: 0
```

The selected test executed and passed with zero skips. No source edits, extra fixture, repeated build for additional assertions, or performance/stress work was performed by this validator. Windows process-group cleanup was not exercised by this host.
