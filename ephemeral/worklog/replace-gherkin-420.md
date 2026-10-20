# Replace the Gherkin suite (#420)

decision: Tyler deleted example/e2e and `just e2e`. Tests let agents check that a change broke nothing; they are not end-to-end, proof of work or release qualification. The one browser check (Kit's client adopting Go's page on /todos) is a Playwright spec in example/web/tests run by Go tests that own their server. Mid-task Tyler asked whether Playwright should go entirely; he said finish #420 as written and revisit after merge.

implementation: The pending-edit dev test is plain HTTP now. Its Go load still blocks on a small httptest gate (the only way to hold a load in the native child); the browser control endpoints, the subprocess and SKGO_E2E_PENDING_EDIT are gone.

mutation: Serializing the dev router's admitted requests (a mutex in router.ServeHTTP) fails TestRealSessionRouteEditDuringPendingLoad with "independent endpoint: ... Client.Timeout exceeded"; reverted.

kept: internal/webbuild skips a user project's `e2e/` directory from build freshness, and internal/sv tests the Playwright add-on sv generates (`*.e2e.{ts,js}`). Both are about user projects, not the deleted suite, so the issue's final grep is not empty there.

fix: The dev client-handoff test copies the example to a temp dir whose app module, github.com/tylergannon/skgo/example, sits under skgo's own module path. internal/gen/cache_source.go took the first module whose path prefixed an import, so skgo claimed the app's `_links` packages and the session exited with "symlinked import directory is not cached". It now picks the longest matching module path, as Go does. Reverting the fix fails TestRealSessionClientHandoff at startup.

mutations: built handoff: moving getTodos's embed key fails with "remote requests: GET …/watchCount, GET …/getTodos". Dev handoff: disabling the published notify fails the Go edit step (expected 1 document, received 0) while the log still says "published revision 3".

trap: `skgo build` runs `vp fmt` over example/web (internal/webbuild/build.go:81), so authored specs come out reformatted. Commit the formatter's output. Dev sessions run by hand need `--go` pointing at go1.27.2; GOTOOLCHAIN alone gives "version go1.27.2 does not match go tool version go1.27.3".

looked: /todos in production (built, port 8080) and dev (skgo dev, 18490) both render four todos, hydrate, add "looked at by hand" to count 5, with no console errors.
