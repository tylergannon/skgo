# Replace the Gherkin suite (#420)

decision: Tyler deleted example/e2e and `just e2e`. Tests let agents check that a change broke nothing; they are not end-to-end, proof of work or release qualification. The one browser check (Kit's client adopting Go's page on /todos) is a Playwright spec in example/web/tests run by Go tests that own their server. Mid-task Tyler asked whether Playwright should go entirely; he said finish #420 as written and revisit after merge.

implementation: The pending-edit dev test is plain HTTP now. Its Go load still blocks on a small httptest gate (the only way to hold a load in the native child); the browser control endpoints, the subprocess and SKGO_E2E_PENDING_EDIT are gone.

mutation: Serializing the dev router's admitted requests (a mutex in router.ServeHTTP) fails TestRealSessionRouteEditDuringPendingLoad with "independent endpoint: ... Client.Timeout exceeded"; reverted.

kept: internal/webbuild skips a user project's `e2e/` directory from build freshness, and internal/sv tests the Playwright add-on sv generates (`*.e2e.{ts,js}`). Both are about user projects, not the deleted suite, so the issue's final grep is not empty there.
