# Run skgo workflows on its LAN hosts

- decision: all Linux jobs use Bob and all macOS jobs use Amritanandamayi, with explicit host labels. The user accepted GitHub-hosted npm publication after learning that npm trusted publishing rejects self-hosted runners; both packages are prepared on Bob and uploaded as artifacts for the hosted publish jobs.
- discovery: current main already uses Lightpanda for every CI browser check. The Chromium failures came from old commits pushed across 64 historical/backup branches together. Editing main's workflows does not rewrite those historical commits.
- discovery: Linux ARM64 release packaging executes the CLI and builds/loads an external cgo plugin. Changing its runner to Bob's x86_64 runner alone would change or break the release. An ARM64 runner container under QEMU retains the existing executable and plugin witnesses on Bob.
- friction: actionlint rejects the existing concurrency `queue: max` field supported by GitHub. Ignore only that specific diagnostic when checking these workflows; retain the release queue.
- correction: the user explicitly accepted hosted publication rather than introducing an npm token. Do not replace npm OIDC with a new credential.
- decision: the user requested more than three workers. Bob has 16 logical CPUs and about 58 GiB RAM; use four x86_64 workers at four CPUs/eight GiB each plus the ARM64 worker. Amritanandamayi has ten cores and 24 GiB RAM with 59% free at observation; add a second independent worker, and cap macOS Go/Zig/Swift compilation at four-way concurrency per job.
- discovery: cancelled the fifteen unfinished runs from the exact 2026-10-21 21:55:39–49 UTC historical push batch after explicit user authorization. New PR runs were excluded.
