# Native Kit development: continuation

Updated 2026-10-15. Worktree `/Users/tyler/.codex/worktrees/5f35/skgo`, branch `codex/dev-server-review-fixes`, based on dev868c5dba. Read agent-protocol, [GOAL](GOAL.md), [DESIGN](DESIGN.md), then [PLAN](PLAN.md). Never reset to historical hashes.

## Authorization and endpoint

The user explicitly authorized assessment followed by 3–5 implementation chunks using Sol6.1 subagents. Earlier architecture-only restrictions were superseded. Parent orchestrates/synthesizes; owners implement. The user subsequently authorized shipping on 2026-10-15 after review repairs. Open an integration PR against the existing `dev` baseline and request native squash auto-merge; do not bypass checks or claim a queued merge is complete. No separate release or upstream posting is requested. Do not cancel ongoing subagents. Use `/Users/tyler/src/skgo-token-cache/index/README.md` and `index/dev-native-kit-uds.md`; cached old plans are historical, linked workspace files authoritative.

Five missions in PLAN have implemented the selected topology. Independent validation is complete at f8b6485, with the qualifications below. Preserve bounded scope; remaining failures go to their owners, not a new architecture exercise.

## Current execution state

- Latest product repair `f10c262`: native environment artifact, exact installed-Kit environment alias, canonical private-file denial. Native adapter fixture passes environment reload, route add/change/remove, and direct/raw/symlink private artifact controls.
- Test isolation `74b843f`: share installed dependency entries, keep Kit3 `node_modules/$app` and Vite caches fixture-local. Test independently proves example generated route types unchanged. Example restored via `vp sveltekit sync` and ordinary `just generate`; deterministic generated caller output committed `720e00c`.
- One full Go run failed native lifetime mapping assertion after release. Seven focused executions then passed. Diagnostic-only `f8b6485` adds Mach mapping object offsets/current mapping details, leaving the assertion unchanged. Original intermittent failure remains unexplained; no causal fix claimed.
- Final original-concurrency `just test -v` passed at f8b6485:34packages,958top-level tests,0failures,0skips; log `/tmp/skgo-final-go-test-final.log`. Native lifetime passed10.86s without assertion relaxation. Production artifacts were refreshed for current adapter fingerprint. Do not repeat checks solely because docs are committed.
- `/root/kit_callbacks` and `/root/go_request_bridge` are completed and available for targeted followup; validator completed. Parent owns docs. No owned dev/prod servers remain; validator confirmed ports8087/8088 and dev private session directory cleaned. Inspect live state if newer work started.

The five-mission implementation is delivered locally. Final docs record coverage and the unexplained earlier lifetime failure; no product fix is claimed for that failure. There is no active implementation assignment. If the user requests further work, preserve this evidence and investigate only the specific remaining concern, without reopening architecture or repeating completed checks by default.

## Selected architecture and lean seams

Browser → Go coordinator public listener → Vite UDS for native Kit documents/data/actions/routes/assets/HMR. Reserved remote prefix (respecting base/appDir) → existing Go remote wire handler. Generated Kit callback bodies call Go UDS; Kit wrappers/live graph remain authoritative. No separate dev renderer graph or Go dev renderer bundle.

Each session owns a short unique directory with constant `vite.sock` and `go.sock`. Bind public origin before initializing Kit. Supervise child readiness/replacement; old socket cleanup cannot unlink a new owner. Compatible Go edits use retained native prepare/install; unsupported edits rebuild. No arbitrary initialized-state survival or whole-render code-revision pinning requirement.

Actual app.NewHandler configuration is preserved through NativeDevelopment build carrier. Native NewSSR registers real Go hooks/registries/options without constructing goja; request still traverses app FetchConfig/request boundary. JS HandleFetch preserves Kit's supplied continuation through the Go bridge; Go-origin internal fetch enters Kit before the Go boundary, except configured remote wire prefix. There is no adopted-request header/map protocol. Synchronous Kit ResolveOptions predicates use a small transport-only worker, not a renderer.

One random Go handle owns a logical request. Replacement rejects stale handles with409 and never replays mutations. Kit traced handle events are shallow copies, so shared locals Symbol carries ownership into fetch's original-event closure. Derive Go raw/typed params from original URL and actual Go matchers; native forms reuse production caller binding. HTTP JSON headers must be canonicalized before Go emits them; decoded response adapters strip hop framing.

Production remains one cgo-free Go binary with pooled goja, Node at build time only. Native Node SSR/transport is an explicit dev exception; application behavior stays Go. No custom waiting page, forced navigation, Vite patch or browser-state destruction policy.

## Recorded validation

- Real-session integration at `f7871d7` passed25.79s: native edit samePID, initializer rebuild, Go/Vite crash recovery, two instances, combined frontend/Go edit, syntax failure retains healthy service and repair recovers. Log `/tmp/skgo-uds-core-integration-current.log` includes transient startup/restart errors before finalPASS; no latency benchmark implied.
- All27 selected distinct native browser scenarios passed across recorded runs. Final remaining3cases at `e0d6382` passed2.1s. Covers SSR/hydration/transport/cache, navigation, mutations/refresh, hooks, actions, bare/keyed typed forms and nested/Go fetch destinations. **Not a full development browser-suite pass.** Logs `/tmp/skgo-joined-expanded.log`, `/tmp/skgo-joined-validation.log`.
- Actual Chrome Svelte HMR: Item42→Item42HMR781→Item42, draft native-hmr-draft781 preserved. Screenshot inspected healthy; edits restored. Repaired nested page inspected healthy too.
- Full production browser run175passed/1failed45.3s; sole failure was an over-tightened media-type equality against correct charset suffix. Restored original prefix assertion `067959f`; targeted case passed1.4s. **Not an uninterrupted176pass full command.** Production Item42 inspected in Chrome. Logs `/tmp/skgo-final-prod-e2e.log`, `/tmp/skgo-final-prod-server.log`.
- Handler/transport tests cover streaming/deferred/cancellation, request owner loss without replay, real Go hooks/locals, synchronous predicates, batch/live/noJS form bodies, original typed URL values and trusted/configured client address. Exact lowercase-header Node-parser regression fails against prior source and passes repaired source.
- Initial GOFLAGS=-v verification invocation polluted child Go build output; discard those failures as invocation evidence. `cf260c0` adds optional arguments to the existing `just test`, so `just test -v` affects tests only. Same checkpoint fixes vp global CLI resolution through VP_CLI_BIN. No alternative runner.

## Accepted limitation and evidence boundaries

Automatic response-cookie propagation from a direct SSR UDS subfetch into Kit pending cookies/outer page is accepted nonblocking. Headers are server-readable. Public Kit3 cookies.parse/set remedy is source-mapped, not runtime-qualified; do not expose HttpOnly cookies through page serialization. Other cookie/auth requirements remain. No upstream contract violation or nonbreaking API fix was established, and no issue was posted.

Original POC evidence is linked from DESIGN; it did not prove complete callbacks or child crash recovery. New implementation evidence above supersedes those narrow limits, without rewriting history. Old507ms warm-Go/7.574s fallback medians describe the removed renderer composition, not current topology. No new benchmark claimed.

## Cleanup and source authority

Use installed Kit3.0.0 at `example/web/node_modules/@sveltejs/kit`, following `ephemeral/sveltekit-current/SKILL.md`. `vp` owns frontend tools. Keep experiments/harnesses in Go; JS is framework glue only. Worklog records actionable traps at `ephemeral/worklog/20261015-architecture-experiments.md`.

Unrelated untracked historical directories under ephemeral/experiments (kit-publication-poc, kit-publication-validation, live-metadata/runtime, overlap-answer/runtime, visible-latency app/binaries) are not owned by this task. Do not delete or stage them. All task source is checkpointed; final doc changes may be pending. Inspect Git rather than inferring cleanliness from this file.

## Review repairs authorized — 2026-10-15

The user has now requested implementation of the full review findings, using
subagents in parallel and review of changed code. Worktree remains 5f35 on
`codex/dev-server-review-fixes`, baseline `1d9e23e`. Earlier statements that
there is no active assignment are superseded by this request. The review and
its concrete reproductions are in
`/Users/tyler/.codex/worktrees/9b3a/skgo-project/ephemeral/reviews/20261015-skgo-development-server-review.md`.
Three owners repair configuration publication, request bridge semantics, and
native compilation/application composition. Keep current GOAL boundaries and
accepted temporary limitations. Parent coordinates shared-file boundaries,
joined validation and fresh independent review. Preserve all untracked experiments.


## Shipping handoff — 2026-10-15

Product repairs are checkpointed through `380f943`; `02c1a9a` corrects the real
adapter fixture to await its listener and changed configuration epoch. The
filesystem carrier is replaced by an explicit application development constructor
sharing production setup. Captured compiler/workspace/runtime inputs, Vite
configuration publication, header effects, demand-driven parent data, fetch body
transport/metadata/ownership and cancellation are repaired. Independent review
found and repaired two further continuation-body lifetime defects; both final
full-target reviews report no findings. Review artifacts are in
`ephemeral/reviews/20261015-dev-{repairs,changed-code}-round-*.md`.

The actual two-session recovery/configuration integration passed at the final
product source (75.95s), with all eight owned child PIDs and both session
directories confirmed gone. Production build and `just vet` passed. Full
production browser run: 176 passed, zero skipped, flaky or unexpected cases
(41.6s), including JavaScript-off forms. A separate UI reader used the running
production todo editor and item preview form, restored its edit, and inspected
healthy visible content. Logs: `/tmp/skgo-dev-repairs-final-session.log`,
`/tmp/skgo-dev-repairs-final-build.log`, `/tmp/skgo-dev-repairs-vet.log`,
`/tmp/skgo-dev-repairs-prod-browser.{log,json}`.

The first broad Go run exposed stale readiness assumptions in one adapter test;
those are fixed without relaxing post-readiness HTTP assertions. Final `just test -v` passed: 34 packages, 968 top-level tests, zero failures
or skips (`/tmp/skgo-dev-repairs-final-go.log`). Native UI smoke rendered todos and Widget 42. A temporary Svelte title edit
appeared without manual reload, but the unsent Add draft was lost. This is an
observed limitation; draft preservation is not certified. The edit was restored. The user requested shipment
without another full local browser suite because CI repeats the suites. A full
development browser-suite pass remains unclaimed. Existing accepted cookie,
platform, historical lifetime-flake and benchmark evidence boundaries remain.
