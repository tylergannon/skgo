# Making skgo public: LICENSE, README, secret scan

Added `LICENSE` (MIT, copyright 2026 Tyler Gannon — matches the template used
by tractor, mcp-go, etc.) and replaced the one-line `README.md` with real
content drawn from `AGENTS.md`.

Full-history secret scan (`git log -p --all`, ~151 commits) and a scan of the
409 tracked files under `ephemeral/`: no live credentials, API tokens, private
keys, `.env` content, or connection strings with embedded credentials. Two
things that looked like hits and were not:

- `export const load = (): { secret: string } => unimplemented();` — a
  generated stub whose type field is literally named `secret`; no value.
- `throw new Error('SECRET-INTERNAL-DETAIL: database password is hunter2')` in
  `ephemeral/semantic-index/sources/junkyard/app/guestbook-app.md` — a fixture
  documenting kit's error-scrubbing behavior, not a real credential.

One low-severity finding worth knowing about before going public: a worklog
(`ephemeral/worklog/202609061500-skgo-new-scaffold.md`) mentions
`GOPRIVATE=github.com/pagerguild/*,github.com/tylergannon/*` from the
developer's local `go env`. It names a second private GitHub org
(`pagerguild`) the owner is affiliated with. Not a credential and not worth
history rewriting, but it is a fact about the owner that a public repo would
otherwise not surface — flagged in the PR/report rather than scrubbed, per
"do not rewrite history."

CI (`.github/workflows/ci.yml`) references no `secrets.*` at all — it only
needs `actions/checkout`, `setup-go`, `mise-action`, `pnpm/action-setup`, and
`setup-just`, none of which need repo secrets. Confirmed no `GOPRIVATE`/
`GONOPROXY` set anywhere in the repo (Justfile, CI, mise.toml), so nothing in
the checked-in config forces private-module resolution — that env var lives
only on the developer's machine.

All go.mod / example/go.mod dependencies resolve to public GitHub repos
(dave/dst, dlclark/regexp2, dop251/goja, go-sourcemap/sourcemap, google/pprof,
stretchr/testify, tylergannon/polytype, tylergannon/structtag, golang.org/x/*).
