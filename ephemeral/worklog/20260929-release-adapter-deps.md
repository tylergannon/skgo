# Release adapter test dependencies

The adapter publication job is a fresh checkout after browser qualification. Its `go test ./internal/adapter/` now includes the server-only import test, which builds a disposable Kit app using `example/web/node_modules`. Passing qualification does not install those dependencies in the publication job. Keep its example install before the adapter test; a passing qualification alone does not prove the package can publish.
