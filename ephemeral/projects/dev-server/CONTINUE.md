# Resume here after compaction

Updated 2026-10-15. Workspace `/Users/tyler/.codex/worktrees/5f35/skgo`, branch `codex/dev-architecture-experiments`, based on dev868c5dba. Read agent-protocol, then GOAL, DESIGN and PLAN.

## Current authorization and active workers

The user explicitly requested assessment followed, if ready, by 3–5 implementation chunks using Sol 6.1 subagents. Parent assessed architecture ready to begin bounded implementation; full joined semantics still require proof. Earlier architecture-only prohibition is superseded. Five chunks and endpoint are in PLAN.

Sol 6.1 workers: `/root/uds_supervisor` owns public coordinator/process/transport, internal/devserver, command wrapper and adapter dev.js; `/root/kit_callbacks` owns adapter callback JS, generation and distinct dev_fetch/dev_remote integration; `/root/go_request_bridge` completed root Go request/service and internal/hotreload generated host (available for followup fixes); `/root/joined_validation` is the fresh independent validator owning e2e/test qualification and the small vp tool-resolution defect. Check live status rather than assuming these remain active. Shared worktree; coordinate interfaces/commits, never revert others. Parent owns planning and scope, not competing implementation.

Initial implementation checkpoints:451052b/2357910 Go lifetime+stream/cancel;31b410a Kit adapter callbacks;af4eeb3 UDS supervisor. Later joined fixes are listed below. Do NOT call product complete from handler/unit tests. Validator must prove actual native Kit app and separately production regressions. No PR merge/release or upstream posting authorized.

Do not cancel independently ongoing agents. Read their status/messages after compaction rather than redispatching duplicate work. They must use token-cache index and pinned Kit source. Existing unrelated untracked experiment artifacts are not owned by this task.

Cookie response propagation from direct SSR subfetch is accepted non-blocking. The public Kit3 cookies.parse/set remedy is source-mapped, not runtime-qualified. No upstream change is required and no issue was posted. Prior POC processes were cleaned; do not assume fixtures live.

## Current joined qualification

Real-session integration passed at f7871d7: actual Kit endpoint callbacks, native edits retaining PID, initializer rebuild, Go/Vite crash recovery, isolation, combined frontend/Go edit and compile-error retention/repair. `/tmp/skgo-uds-core-integration-current.log` is local output.

Current stable core a1737e4 and adapter0020746. Root Go tests passed. Earlier browser failures (auth sign-in, native page fetch404, prerender search getter, typed params begin400, middleware slug,403 custom body, unimported prerender remote404, lost owner on Kit traced copy) have fixes and fresh runtime proof. Native typed params are derived from original URL through actual Go matchers, never stringified Kit values. Go-origin fetch enters Kit before Go RequestBoundary except the configured remote wire prefix; no adopted-request mechanism. Direct remote/public-peer API tests preserve trusted127.0.0.1 and app-configured override, with forged-header controls. Compiler lowering now preserves exact SDK identity-taking calls, fixing auth reconnect lookup.

All 27 selected distinct native browser scenarios passed across recorded runs; final e0d6382 remaining3cases passed2.1s. Exact lowercase JSON header parser regression fails against prior source, passes repaired code. Clean Chrome HMR retained draft through edit/restoration; repaired nested page inspected healthy. No full development-suite pass claimed.

Final broad qualification in progress. Initial GOFLAGS=-v just test was invalid because verbosity polluted child build output; use cf260c0's `just test -v` CLI args instead. Validator fixed real vp resolution inside vp exec by preferring VP_CLI_BIN global executable. Production build passed after env.json restoration. Full production browser run175passed/1failed; the sole failure was validator's over-tightened text/plain equality against correct charset suffix. Original prefix assertion restored067959f, targeted scenario passed1.4s; no uninterrupted full176pass command claimed. Actual Chrome production Item42 inspected healthy, production server stopped, example tracked diff empty.

Callback checkpointf10c262 is frozen. Native-equivalent adapter pair passed2.970s with literal static/dynamic/Go values, dotenv reload, route add/change/remove and ordinary/raw/symlink denial for generated private inputs/current/old snapshots. Product fixes stay dev-only: env.json publication, exact installed-Kit internal/env alias, canonical private file denial. Package expected ordering corrected. Validator independently checked load-bearing literals/path claims. Final just test -v failed four tests, zero skips: TestGeneratedNativeLifetime retains a retired mapping after release; three example tests show Kit generated-type contamination from native fixture routes (/ and /z-existing). Fixture isolation repaired74b843f: dependencies shared entry-by-entry, $app/caches local; focused native test proves real example route types unchanged. Validator restored example via vp sveltekit sync/just generate and will retain deterministic generated params changes. `/root/go_request_bridge` remains active for lifetime diagnosis; its isolated test passed but broad failure is not yet explained. `/root/joined_validation` coordinates rerun only after focused fixes. Do not weaken assertions or broaden unloadability obligations; no full-suite success yet. Production browser evidence remains valid.

Workers `/root/kit_callbacks`, `/root/go_request_bridge` and `/root/joined_validation` own this remaining work. Parent owns docs. Temporary socket relay was restored and diagnostics removed; validator stopped dev app and controls future app lifecycle. All unrelated untracked experiment dirs remain untouched. No new architecture campaign or cookie gate.

## Selected architecture

Browser -> public Go coordinator -> Vite UDS for native Kit documents, __data, actions, +server routing, frontend modules and HMR. Generated callback bodies delegate application logic to Go UDS while retaining Kit's wrappers and live graph. Browser reserved /_app/remote prefix initially goes directly to existing Go wire handler, preserving its complete refresh/update/ignore protocol. Direct SSR query callbacks still use Kit wrapper+Go body bridge. Kit and Go use matching module-hash remote IDs by source mapping; joined ordinary-query SSR/hydration/transport/cache and browser mutation checks have passed.

No separate development renderer bundle, copied Kit graph, custom waiting page, forced reload policy or Vite behavior modification. Existing page/state preservation follows normal Kit behavior; do not choose extra browser destruction without user approval. All business logic and data access stay Go; Node dev SSR/transport is explicitly allowed.

Each session shares one short private socket directory with vite.sock/go.sock; public origin is initialized from actual public bind. UDS eliminates changing private port discovery, not process readiness or ownership. Keep original Go process/request-handle context through callbacks or fail clearly on loss. **No new requirement to freeze native code revision for an entire render.** That unsupported strengthening was withdrawn.

## Executed evidence versus limits

- `ephemeral/experiments/20261015-vite-uds-ssr/REPORT.md`, raw HTML/headers/logs and BROWSER.json: actual KitSSR over Vite UDS, SSR event.fetch to Go UDS, independent ALPHA4101/BETA5202, cookies/auth/marker/publicHost transport, wrong-target HTTP200 control, public HMR101 and actual Svelte update, independent hydration interaction.
- Reopening ALPHA Go socket at same path served7407 to fresh requests; BETA unchanged. Go backend and public proxy were in one Go harness. **Not child crash recovery**. Existing browser needed manual reload; **no automatic Go refresh proof** in this fixture.
- `ephemeral/experiments/20261015-vite-uds-validation/ASSESSMENT.md`: independent browser/checks and explicit limits.
- `ephemeral/experiments/20261015-vite-uds-bridge/BRIDGE.md`: pinned Kit3 call boundaries and reuse of existing prerender bridge. It is build-only, KindPrerender-limited, buffers bodies/deferred values, lacks actions/general remote effects and has build-specific cancellation/synchronous callbacks. Do not claim complete native-dev callback integration.
- Native Go patching/admission proofs remain historical reusable evidence. Earlier507ms warm-Go /7.574s rebuild medians describe the **old** renderer composition, not new topology performance.
- The original UDS POC changed only isolated fixtures/docs. The subsequently authorized implementation changes current product source; retain the distinction when citing historical evidence.

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
