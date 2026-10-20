# Replace the Gherkin suite (#420)

decision: Tyler deleted example/e2e and `just e2e`. Tests let agents check that a change broke nothing; they are not end-to-end, proof of work or release qualification. The one browser check (Kit's client adopting Go's page on /todos) is a Playwright spec in example/web/tests run by Go tests that own their server. Mid-task Tyler asked whether Playwright should go entirely; he said finish #420 as written and revisit after merge.

implementation: The pending-edit dev test is plain HTTP now. Its Go load still blocks on a small httptest gate (the only way to hold a load in the native child); the browser control endpoints, the subprocess and SKGO_E2E_PENDING_EDIT are gone.

mutation: Serializing the dev router's admitted requests (a mutex in router.ServeHTTP) fails TestRealSessionRouteEditDuringPendingLoad with "independent endpoint: ... Client.Timeout exceeded"; reverted.

fix: The dev client-handoff test copies the example to a temp dir whose app module, github.com/tylergannon/skgo/example, sits under skgo's own module path. internal/gen/cache_source.go took the first module whose path prefixed an import, so skgo claimed the app's `_links` packages and the session exited with "symlinked import directory is not cached". It now picks the longest matching module path, as Go does. Reverting the fix fails TestRealSessionClientHandoff at startup.

mutations: built handoff: moving getTodos's embed key fails with "remote requests: GET …/watchCount, GET …/getTodos". Dev handoff: disabling the published notify fails the Go edit step (expected 1 document, received 0) while the log still says "published revision 3".

trap: Dev sessions run by hand need `--go` pointing at go1.27.2; GOTOOLCHAIN alone gives "version go1.27.2 does not match go tool version go1.27.3".

looked: /todos in production (built, port 8080) and dev (skgo dev, 18490) both render four todos, hydrate, add "looked at by hand" to count 5, with no console errors.

decision: Tyler wanted no full Chromium, and the check in Vitest. example/web/tests/client-handoff.test.ts is a Vitest test (`vp test run --config tests/vitest.config.ts`) in Node that drives Lightpanda 1.0.1, a headless browser without layout, over CDP with playwright-core. internal/clienthandoff starts `lightpanda serve` on a free port and runs it for both Go tests.

tried, best first: (1) playwright-core over CDP to `lightpanda serve`, chosen. (2) `lightpanda fetch --inject-script-file` from Go also passed 15/15 and saw the dev edit, but the assertions live in an injected script. (3) Vitest browser mode's Playwright provider connects with browserType.connect, not CDP, and runs tests in an iframe on Vitest's server, so it cannot use Lightpanda. (4) happy-dom: its ESM rewrite leaves a bare import.meta.resolve, it lacks SVGAElement, and addTodo got 403.

trap: Lightpanda lays nothing out (boxes are 5x5), so Playwright's click() waits forever; dispatchEvent("click") works. page.route script holding left about half the runs unhydrated on Lightpanda; an init-script MutationObserver keeps the parser's nodes instead and DOMContentLoaded snapshots Go's text. Vitest sets process.env.BASE_URL to Vite's base "/", so the origin variable is HANDOFF_ORIGIN.

ci: qualification.yml downloads lightpanda-x86_64-linux 1.0.1, checks its sha256 and puts it on PATH. bob's skgo runners are Docker containers (skgo-release-runner, Ubuntu 24.04), so a host install would not reach them; the pinned binary ran inside skgo-release-runner-1.
