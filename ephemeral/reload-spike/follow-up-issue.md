## Outcome

Make SKGO's development experience fast enough that choosing Go does not impose a noticeably worse edit-to-browser loop than ordinary SvelteKit. SKGO should own Vite, code generation, and Go application replacement in one development session; Gimbal should consume the same capability through ordinary Go hooks.

This is follow-up work. Land the bounded, independently verified cache/replay and staging gains first so other development lanes can proceed. Do not make those releases wait for a language-server fork or native hot reload.

Related: #307 (standalone generation cost), #277 (new-route readiness), #243 (development diagnostics), #113 (edits during pending Go I/O). Current delivery: SKGO #310, [Polytype #167](https://github.com/tylergannon/polytype/pull/167), [Gimbal #528](https://github.com/tylergannon/gimbal/pull/528). Polytype's earlier persistent cache is released as v1.7.0.

## What we learned

These are observations from development experiments on Go 1.27.2, macOS arm64, with existing tool caches. They are not controlled application latency guarantees.

- Replay of formatted generated output can restore missing output in well under one second without package loading or formatting. Generation must also work when prior generated output is missing, stale, or invalid; authored inputs and configuration are authoritative.
- One SKGO changed-input profile took 6.852s. It made 96 Go subprocess calls: 30 dependency/export discovery calls, 30 release/build-tag probes, 29 architecture/compiler probes, four environment queries, two package-directory queries, and one module query. Aggregate subprocess wall time was 3.467s. Repeated overlay materialization and removal also consumed substantial time. Keeping this exact loop alive is insufficient.
- A shared invocation session improved one SKGO example from 9.89s to 6.31s cold and 9.83s to 6.32s after a field edit. Full Gimbal runs exposed overlay/fallback regressions; fixes restored matching output, but a controlled full-pipeline speed win is not established. Do not extrapolate the example result.
- A small real Gimbal package experiment reduced retained-metadata/import parse+check work to roughly 0.17–0.28ms, compared with about 118ms through the current loader. This excludes capture, generation, publication, compilation, and browser delivery.
- One Gimbal body-edit build took 3.178s, of which 2.870s was linking. A matched exploratory `-compressdwarf=false` run took 2.884s with 2.631s linking. This is insufficient evidence to change defaults.
- Gimbal's existing development launcher starts Vite and Go but does not rebuild Go after startup. The SKGO supervisor has the reusable lifecycle foundation. Warm generation and application reload are distinct costs, even when one supervisor owns both.

## Direction to evaluate

One dev-owned, long-lived source snapshot and dependency graph should feed all generators and reload planning. Each generator retains its own persistent replay cache and consults analysis only on a miss. Retain captured bytes and compatible parsed/type-checked packages; avoid repeated filesystem reads, package discovery, and reconstruction of identical overlays. LRU controls memory retention, never semantic validity.

Evaluate selective reuse of a pinned gopls implementation against the bounded `polytype/goload` session. Useful candidates include package metadata/reverse dependency graphs and parse-cache machinery. The full gopls Snapshot/TypeCheck implementation is coupled to workspace, protocol, settings, indexes, and persistent-cache internals; it is not a small public library. Ordinary LSP does not supply the complete AST/type graphs needed by these generators. A pinned extraction needs upstream provenance, license notices, relevant tests, and deliberate Go-version compatibility maintenance. A minimally modified fork may be preferable if an extraction pulls in most of the engine. No choice has been made yet.

Preserve build tags, module/workspace/replacement selection, overlays, generated declarations, source and embed membership, effective build settings, and coherent type identity. gopls's own TypeCheck contract permits types from separate importers, so it cannot simply replace our shared-type contract unchanged. Structural or unsupported changes must trigger authoritative discovery or an ordinary rebuild.

## Native reload experiment and its limits

A separate proof of concept uses [pkujhd/goloader](https://github.com/pkujhd/goloader) at `2695cfc5350631cc3becc1409162b12a885471b5`. It compiles one admitted handler source file and loads native object code into a retained host. Local tests exercise literal changed HTTP responses, retained host state, active requests during replacement, compile failure and repair, reverted edits, rejection of incompatible changes, repeated GC, and unloading. A 60-replacement stress run observed 47,928 concurrent HTTP requests, 61 modules loaded/unloaded, at most two resident revisions, and independent OS confirmation that retired code/data mappings disappeared. Independent review is pending; these claims remain fixture-scoped.

The narrow compile/load/HTTP experiment measured about 19–27ms. Complete watched edits with conservative dependency/toolchain content verification measured about 254–451ms; hashing dominated. Neither is SKGO/Gimbal application performance. The watcher currently watches the admitted handler only: dependency/configuration changes are rejected when another handler reload is attempted, not proactively restarted.

An immutable compiler/dependency snapshot tied to the actual host build may remove repeated verification. This ownership integration is not implemented. Native replacement also needs compiled dependency identities and a replacement boundary: type-analysis invalidation alone misses inlining/generic-code effects, and host-linked callers may still hold old addresses. Layouts, initializers, globals, callbacks, and active calls need explicit compatibility/lifetime handling. Incompatible edits must rebuild/restart successfully. The macOS experiment uses cgo and compiler/runtime internals; it has not changed SKGO's pure-Go production runtime.

## Evidence required before adoption

- Compare complete cold startup, unchanged replay, Go function-body edits, API/type edits, dependency edits, new/deleted routes, and frontend-only edits in the real SKGO example and Gimbal. Measure source save through the correct visible browser result, plus phase timings; compare frontend edits with the pinned SvelteKit/Vite baseline under matched conditions.
- Ordinary compatible edits should avoid repeated Go discovery and unnecessary generation. A function-body edit that leaves a generator's semantic input unchanged should reuse that generator's result while publishing any required fresh executable source.
- Show broken edits then repair, missing/invalid generated output, changes during active requests, and tag/module/configuration changes. Compare output and diagnostics with a fresh authoritative run. No stale results or manual cache deletion as the recovery mechanism.
- Show memory and retained native modules remain bounded over sustained edits. Measure full verification costs rather than reporting only compiler or parser microbenchmarks.
- Keep the same reusable technology available to both SKGO and Gimbal. Prefer the fastest correct, maintainable approach; native hot reload and a gopls fork are experiments, not predetermined requirements.

## Resume references

- Research branch: `codex/generation-cost-research`, POC checkpoint `82801ae` (local at issue creation).
- Research worktree: `/Users/tyler/.codex/worktrees/skgo-generation-costs/skgo`.
- Detailed experiment notes: `ephemeral/reload-spike/experiment-notes.md`; runnable POC: `ephemeral/reload-spike/poc/README.md`.
- The private GOROOT and cloned loader are disposable local prerequisites, not committed project dependencies. Never modify the installed Go toolchain to run this experiment.
- [Gopls incremental analysis](https://go.dev/blog/gopls-scalability), [gopls implementation](https://go.dev/gopls/design/implementation), [current type-checker contract](https://github.com/golang/tools/blob/master/gopls/internal/cache/check.go).
