# Let a release rerun finish while npm stages the version

- discovery: Release run 39503501969 attempt 1 published all of v0.43.3 (CLI
  archives, both npm packages, GitHub Release, cask, receipt
  `release-39503501969-1` published). Attempt 2 was a manual rerun of only
  "Publish @skgo/sv" 6 minutes later; its 409 posted receipt
  `release-39503501969-2` failed. Nothing needed recovering: @skgo/sv@0.43.3
  serves the tagged sv-package bytes.
- discovery: after `vp pm publish` succeeds, npm stages the version. The
  registry answers 404 for it and 409 "Cannot publish over previously staged
  version" to another publish. Lag observed 2026-10-20: 51 s to about 8 min.
- decision: the step waits (10 s polls, 15 min cap) only after a 409, then
  requires the served integrity to match the tarball. A successful fresh
  publish still returns at once: no downstream job failed on the lag
  (skgo-project's installation confirmation passed for v0.43.3). Any other
  publish failure fails at once.
- friction: skgo-project qualification for 49e3a218 and a4807cee failed in
  `scripts/qualify/run.sh` after ~5 s with no attempt.json; 577ec776 and
  db74b3a8 qualified between them. run.sh's output stays on the core runner,
  so the Actions log shows nothing. skgo-project's to fix; left alone.
