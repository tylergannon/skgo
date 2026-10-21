# Run skgo workflows on its LAN hosts

- decision: all Linux jobs use Bob and all macOS jobs use Amritanandamayi, with explicit host labels. The user accepted GitHub-hosted npm publication after learning that npm trusted publishing rejects self-hosted runners; both packages are prepared on Bob and uploaded as artifacts for the hosted publish jobs.
- discovery: current main already uses Lightpanda for every CI browser check. The Chromium failures came from old commits pushed across 64 historical/backup branches together. Editing main's workflows does not rewrite those historical commits.
- discovery: Linux ARM64 release packaging executes the CLI and builds/loads an external cgo plugin. Changing its runner to Bob's x86_64 runner alone would change or break the release. An ARM64 runner container under QEMU retains the existing executable and plugin witnesses on Bob.
- friction: actionlint rejects the existing concurrency `queue: max` field supported by GitHub. Ignore only that specific diagnostic when checking these workflows; retain the release queue.
