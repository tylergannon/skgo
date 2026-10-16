# Independent validation of the Vite listener fixture

The completed fixture demonstrated instance-isolated native updates and backend recovery for two active Vite/Go instances. An uncached Go HTTP assertion passed for the correct ALPHA/BETA targets, failed when ALPHA was deliberately routed to BETA, and passed again after restoring ALPHA's target. The failure was load-bearing: HTTP 200 alone was insufficient; the response also had to carry the expected public origin, instance label, and independently supplied Go SSR value.

## Executed results

The author brought up two isolated Vite roots at public origins `http://127.0.0.1:56411` (ALPHA) and `http://127.0.0.1:56421` (BETA), with separate Go hosts and root-local backend address files. They were started sequentially but remained live together for the controls below. This proves active-instance isolation while both were running; it does not test simultaneous process startup or port collision behavior.

The validator changed ALPHA's watched Go source to return literal4101. The watcher installed revision1 into ALPHA PID18250 in957ms. Public SSR returned ALPHA/4101 while BETA remained3000. The parent independently confirmed ALPHA's browser received its own notification and preserved its seeded draft, count and document ID; BETA received no notification and kept its state. The exact HTTP capture is `alpha-only-http.txt`.

Changing only BETA to5202 installed revision1 into BETA PID18276 in894ms. Public SSR returned ALPHA/4101 and BETA/5202. The parent confirmed BETA received its notification and preserved browser state while ALPHA stayed unchanged. See `both-isolated-http.txt`.

A malformed ALPHA source edit was rejected with a compiler parse error; ALPHA kept last-good4101 while BETA kept5202. The parent saw ALPHA's generic `GO_COMPILE_REJECTED` browser status with state preserved and BETA unchanged. This validates rejection notification scoping and last-good response behavior for this case; the visible message does not establish that a useful compiler diagnostic reached the developer. Exact capture: `alpha-rejected-http.txt`.

Repairing ALPHA to6303 installed in1.056s and returned ALPHA/6303 while BETA stayed5202 (`alpha-repair-http.txt`). The parent confirmed ALPHA's browser recovered with its original state and BETA remained unchanged.

A manually triggered ALPHA restart replaced PID18250/backend `http://127.0.0.1:62554` with PID20664/backend `http://127.0.0.1:62781`. The root-local marker contained the new backend URL. Requests to both the stable public Vite origin and the newly discovered backend returned ALPHA/3000; BETA remained5202. This restart reused the original built binary, so it did not demonstrate rebuilding a changed Go source value. The parent confirmed ALPHA's browser received a restart notice and preserved state. See `alpha-restart-http.txt`.

The integration author then ran a separate crash-recovery fixture with ALPHA at7407 and BETA at8508. The author reports killing ALPHA host PID22532 and observing exact-child recovery launch PID23114, reinstall the last successful native object, and publish its newly bound backend `http://127.0.0.1:63147` instead of `http://127.0.0.1:63032`; BETA PID22552/backend `http://127.0.0.1:63046` stayed unchanged. Parent browser checks are reported in the integration report. This validator did not initiate the crash; the Go test below independently verifies the resulting public identity/value pair.

## Executed Go identity control

`httpcheck/identity_test.go` is a small ordinary Go test. It requires `ALPHA_URL` and `BETA_URL`, requests `/orders`, and asserts HTTP200, exact `X-Application-Origin`, an ALPHA/BETA label, and literal `Go SSR total: 7407` / `8508`. Empty URL variables fail; the test has no skip path. It checks HTTP response bytes only and does not claim browser hydration.

Before the explicit build tag was added, the test source was executed uncached from `httpcheck/` against correct public targets before and after the negative control, and failed uncached during the wrong-target control. The recorded invocation was `GOWORK=off ALPHA_URL=http://127.0.0.1:56411 BETA_URL=http://127.0.0.1:56421 go test -count=1 -v ./...`. The test now has the `skgo_listener_httpcheck` build tag so ordinary repository test discovery cannot run it against stopped fixtures. Reproduction from `httpcheck/` now requires adding `-tags skgo_listener_httpcheck`; no live rerun was performed after that tag-only change.

For the negative control, ALPHA's recovery marker was atomically changed from `http://127.0.0.1:63147` to BETA's `http://127.0.0.1:63046`. The same uncached test failed ALPHA because the response origin was BETA's `:56421`, the body lacked `ALPHA`, and it lacked literal7407; BETA's subtest passed. The original ALPHA marker was atomically restored and verified before the final uncached PASS. The first attempt omitted `-count=1` and got Go's cached PASS without making network requests; that false-positive output is preserved but is explicitly excluded as evidence. Relevant test outputs are `positive-before.txt`, `wrong-target-go-test.txt`, and `positive-after.txt`.

An earlier direct HTTP observation also caught the same misroute as HTTP200 with BETA's origin, label and value (`wrong-target-control.txt`, `wrong-target-headers.txt`, `wrong-target-body.html`). That was an observed mismatch, not a test failure; the later Go assertion is the executed load-bearing failure.

## Limits

The controls establish automatic watcher-to-native-install behavior, per-instance notification and application-response isolation, last-good behavior on compile rejection, and per-instance backend rediscovery/retargeting after restart for the exercised fixture. They do not qualify general application admission, arbitrary initialized Go state, simultaneous instance startup, exhaustive restart races, structural route/remote metadata synchronization, or integrated save-to-visible latency. Browser state claims above are attributed to the parent observer; the Go HTTP test does not assert browser state. The author's separate structural route probe found Go did not acquire Kit's newly added route metadata; see the integration report and structural raw logs.

Initial setup attempts failed before readiness with EOF/HTTP502. They are retained as diagnostics, not used as evidence against the corrected run: `../20261015-vite-listener/failure-missing-client.log` and the earlier records in `../20261015-vite-listener/run.log`. The final integration report and its raw positive/recovery logs are in the token-cache experiment at `experiments/vite-listener-20261015/REPORT.md`.
