# Bake Lightpanda and mise's tools into bob's runner image

- friction: jdx/mise-action's default `cache: true` restored a 353 MB tarball
  from GitHub's cache on every self-hosted job (25–156 s per job on bob, ~87 s
  on the Mac runner), then `mise install` reported every tool already
  installed. On the Mac runner (HOME=/Users/tyler) that restore unpacked over
  Tyler's personal `~/.local/share/mise`. -> `cache: ${{ runner.environment ==
  'github-hosted' }}` on every mise-action that can run self-hosted.
- discovery: mise-action uses `<mise data dir>/bin/mise` without downloading
  when it exists and no `version` input is given, so the image installs mise at
  `/home/runner/.local/share/mise/bin/mise`.
- discovery: a bob runner's registration (`.runner`, `.credentials`) lives in
  the container's writable layer, so recreating a container unregisters it. The
  compose entrypoint waits for `.skgo-start`; `config.sh --replace` under the
  old name keeps the runner id (22, 23 kept theirs on 2026-10-20).
- discovery: `just tools` took 35 s in one production run, but staticcheck plus
  goimports reinstall in ~1.6 s with bob's warm caches. That was one slow run,
  not a repeated cost; left alone.
- discovery: `~/.cache/ms-playwright` (681 MB of Chromium) sat in the shared
  cache volume after #421 removed its last user; deleted.
- decision: image and workflows can be deployed in either order. An old
  workflow on the new image restores the cache as before; a new workflow on the
  old image installs Lightpanda itself. Deployed from this branch before merge
  so the PR's own CI run on bob shows the result.
- doc_bug: docs/index.html still says "a complete development browser-suite
  pass ... not established"; no browser suite exists since #421. Needs Tyler's
  permission to edit docs/.
- friction: Release v0.43.3 failed in "Publish @skgo/sv" (npm 409, "Cannot
  publish over previously staged version") on attempt 2; unrelated to this
  work and left alone.
