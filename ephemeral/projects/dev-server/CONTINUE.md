# Resume here after compaction

Updated 2026-10-15. Workspace `/Users/tyler/.codex/worktrees/5f35/skgo`, branch `codex/dev-architecture-experiments`, based on dev868c5dba. Read agent-protocol, then [GOAL](GOAL.md), [DESIGN](DESIGN.md), [PLAN](PLAN.md). Current task: architecture documentation and bounded research, **not product implementation**.

## Latest user instructions

- Native Vite/SvelteKit SSR during development. Production remains Go+pooled goja. Dev/prod differences are explicitly allowed and qualified separately.
- Both Vite and Go listen on UDS using constant basenames within an instance-private directory. Go supervisor/proxy alone listens on the public network interface.
- SSR fetch delegates Go application work. Direct Kit loads/actions/remotes also require generated callback delegation; fetch alone cannot intercept them.
- Missing automatic SSR response-cookie propagation is an accepted **non-blocking limitation**. Continue architecture docs; investigate a remedy in parallel. Do not turn this into another gate.
- User suggested a tiny non-breaking SvelteKit upstream request might be appropriate. This is permission to investigate, **not to post/send** an issue or message.
- Most recent request: persist everything necessary for post-compaction. This file is that checkpoint.

## Active work / next action

`native_dev_host` (Sol6.1) completed the source-only follow-up under `ephemeral/experiments/20261015-vite-uds-cookies/FINDINGS.md`. Initial finding is saved in commit31a56bc: Kit3 explicitly documents getSetCookie -> cookies.parse -> cookies.set for external response relay. Final follow-up is saved in commit ea9fc81: no documented promise of automatic response-cookie adoption; only the default INTERNAL-route continuation adopts cookies, while the external continuation returns fetch directly. Recommend upstream documentation clarification first; optional opt-in transport continuation is an ergonomics idea, not a requirement. Existing public cookies.parse/set can solve our case without an upstream change. Automatic global adoption risks third-party/synthetic responses, duplicate effects and rewritten-URL path/scope changes. No issue was posted and no runtime test was added. All three delegated lanes have completed; check status before new dispatch, and do not cancel any independently ongoing agents.

The other agents `listener_ownership` and `retained_result_check` completed UDS validation. Their owned POC processes and generated runtime dirs were cleaned. No UDS fixture should be assumed live. Do not restart old experiments to recreate already-recorded evidence.

The cookie follow-up is folded into DESIGN as a non-blocking source finding. Parent has persisted the architecture, decisions, evidence, accepted limitations and handoff boundaries. Resume architecture/documentation work only as needed by the next user instruction; there is no pending agent result to await and no authorized product implementation. Do not infer that unresolved bridge qualification authorizes implementing the product. Answer literal user questions first; frustration is not rhetorical authorization.

## Selected architecture

Browser -> public Go coordinator -> Vite UDS for native Kit documents, __data, actions, +server routing, frontend modules and HMR. Generated callback bodies delegate application logic to Go UDS while retaining Kit's wrappers and live graph. Browser reserved /_app/remote prefix initially goes directly to existing Go wire handler, preserving its complete refresh/update/ignore protocol. Direct SSR query callbacks still use Kit wrapper+Go body bridge. Kit and Go use matching module-hash remote IDs by source mapping; joined SSR/hydration/transport/cache proof is not yet executed.

No separate development renderer bundle, copied Kit graph, custom waiting page, forced reload policy or Vite behavior modification. Existing page/state preservation follows normal Kit behavior; do not choose extra browser destruction without user approval. All business logic and data access stay Go; Node dev SSR/transport is explicitly allowed.

Each session shares one short private socket directory with vite.sock/go.sock; public origin is initialized from actual public bind. UDS eliminates changing private port discovery, not process readiness or ownership. Keep original Go process/request-handle context through callbacks or fail clearly on loss. **No new requirement to freeze native code revision for an entire render.** That unsupported strengthening was withdrawn.

## Executed evidence versus limits

- `ephemeral/experiments/20261015-vite-uds-ssr/REPORT.md`, raw HTML/headers/logs and BROWSER.json: actual KitSSR over Vite UDS, SSR event.fetch to Go UDS, independent ALPHA4101/BETA5202, cookies/auth/marker/publicHost transport, wrong-target HTTP200 control, public HMR101 and actual Svelte update, independent hydration interaction.
- Reopening ALPHA Go socket at same path served7407 to fresh requests; BETA unchanged. Go backend and public proxy were in one Go harness. **Not child crash recovery**. Existing browser needed manual reload; **no automatic Go refresh proof** in this fixture.
- `ephemeral/experiments/20261015-vite-uds-validation/ASSESSMENT.md`: independent browser/checks and explicit limits.
- `ephemeral/experiments/20261015-vite-uds-bridge/BRIDGE.md`: pinned Kit3 call boundaries and reuse of existing prerender bridge. It is build-only, KindPrerender-limited, buffers bodies/deferred values, lacks actions/general remote effects and has build-specific cancellation/synchronous callbacks. Do not claim complete native-dev callback integration.
- Native Go patching/admission proofs remain historical reusable evidence. Earlier507ms warm-Go /7.574s rebuild medians describe the **old** renderer composition, not new topology performance.
- Production/current application source was not changed by this UDS validation; only isolated POC and docs. Older out-of-scope product work remains preserved in Git.

## Cookie precision

Four distinct questions: (1) Set-Cookie headers readable in server handleFetch: yes; (2) updated Kit pending cookies/later same-request fetch; (3) outer SSR response sends cookies to browser; (4) universal-load/client serialization access. Bare hook solved1 but skipped2/3. Normal Kit internal fetch applies2/3; universal loads separately restrict header serialization. Do not whitelist Set-Cookie into page data or expose HttpOnly values.

Pinned Kit `types/index.d.ts:369-391` documents external-cookie relay using response.headers.getSetCookie(), event.cookies.parse(), event.cookies.set(). `runtime/server/fetch.js:152-167` shows native propagation. Public setters have defaults; faithful Go attribute preservation needs care. Later custom outbound UDS fetch copying only original inbound Cookie misses newly set cookies. The accepted limitation concerns this SSR relay; do not silently relax other cookie/auth behavior.

Cookie proposal is source-supported, **not runtime-tested**. The source/docs assessment found no promise of automatic adoption from a replaced handleFetch response, so do not call it an established upstream contract violation. Automatic propagation of arbitrary intercepted responses may change existing scope/security/duplicate behavior. No upstream request sent.

## Files and checkpoints

- Current architecture: GOAL.md, DESIGN.md, PLAN.md, this CONTINUE.md; AGENTS.md has an explicit dev-only SSR exception.
- Previous architecture preserved in DESIGN-go-ssr-historical.md; its imperatives are historical, not instructions.
- Worklog: `ephemeral/worklog/20261015-architecture-experiments.md` (actionable corrections only).
- Commits: cf7b4be validates/documents UDS topology;21200d1/642056f/4c58c32 source bridge decisions;31a56bc cookie API findings;ea9fc81 upstream contract assessment. Later doc checkpoint may follow; inspect Git instead of resetting to these identifiers.
- Token cache entry: `/Users/tyler/src/skgo-token-cache/index/README.md`; new leaf `dev-native-kit-uds.md` links current workspace authority/evidence. It explicitly says new evidence lives outside cache snapshot; older copied plans are historical. No new retrieval benchmark or broad corpus reindex was done.
- Known unrelated untracked old runtime/fixture dirs exist. Do not delete others' data or sweep them into commits. No PR/merge/release task.
