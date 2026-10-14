# Rebuild SKGO development on borrowed upstream mechanisms

## WITHDRAWN — premature outline, do not execute

The user corrected this draft on 2026-10-14: the replacement is substantial functionality and requires deliberate design work together. This outline named upstream mechanisms and desired outcomes without establishing how their application boundaries compose into a working system. Its responsibility table and proposed milestones do not establish that design or its feasibility.

The text below is preserved as an unaccepted outline, not an implementation plan or agent handoff. No replacement implementation is authorized by this document. The independently requested deletion in issue #321 remains valid. Develop the actual replacement design with the user from the authoritative sources and demonstrated application behavior before proposing an executable sequence.

## TL;DR

**Delete first, then build a new Go development server.** [Issue #321](https://github.com/tylergannon/skgo/issues/321) owns complete removal of the existing development implementation. A period with no development command is intentional. The replacement starts from the post-deletion repository; there is no migration, compatibility phase, or reuse of the deleted development control flow.

The result is ordinary `skgo dev`: Vite's frontend edit experience, gopls-style retained Go project state, native implementation replacement through the selected `pkujhd/goloader`, and Kit's own client refreshing the values Go serves. Meaningful compatible Go edits preserve initialized application state and the browser document. Structural edits are compiled and installed by this same new server.

The deletion requirement supersedes earlier instructions to extend the existing supervisor; this withdrawn outline is not its replacement execution instruction. The failed worktree `/Users/tyler/.codex/worktrees/73b6/skgo` remains preserved and unmodified; no failed implementation is a starting point.

- [What the developer gets](#what-the-developer-gets)
- [Sources and what we borrow](#sources-and-what-we-borrow)
- [Every responsibility has an owner](#every-responsibility-has-an-owner)
- [Build order and accepted behavior](#build-order-and-accepted-behavior)
- [Integration limits to resolve early](#integration-limits-to-resolve-early)
- [Completion](#completion)

## What the developer gets

One command starts a session at one stable public address. Saving source starts useful work without a browser request. Files, package metadata, analysis, generated outputs and native dependency artifacts stay warm where their inputs are unchanged. Rapid saves converge on the latest source; an older completion cannot overwrite a newer edit or its diagnostics.

The replacement covers component/CSS/dependency edits; Go loads, remotes, actions and endpoints; added, renamed and removed routes; changed wire types and signatures; layout/page options; templates; embeds; local module replacements; configuration changes; compiler failures and repair; process shutdown and Vite restart. These capabilities come from the application and Kit's semantics, not an obligation to preserve old mechanisms or their tests.

For compatible Go implementation edits, generation does no irrelevant work, the whole application is not linked or restarted, initialized host state survives, and the browser refreshes through Kit without replacing its document or losing unsent input. This must include real bodies that use request arguments, a helper and initialized host state. Evaluating a constant expression into a new string is insufficient.

For structural or otherwise incompatible edits, the new server builds the necessary complete application and installs it at the same address. This operation may reset process state; it does not select a second server implementation. The terminal explains the concrete incompatibility. Missing integration or an unavailable loader is an error, not an edit classification that hides missing work.

Compilation failures leave the last successfully installed application revision intact while the browser and terminal show current diagnostics. Initial startup failure has a usable diagnostic response. Repair clears the diagnostic and updates the page. Do not preserve the old policy that kills the application or globally blocks requests during every build. A request arriving before the watcher observes a save may see the current installed revision; request-time project scans are not a requirement.

Healthy compatible updates preserve the document. Kit-authorized route/config changes, `csr = false` documents, and demonstrated error-boundary recovery may require a document reload. That is a browser update operation with a specific reason, never a return to the deleted development architecture.

## Sources and what we borrow

Use the [research index](/Users/tyler/src/skgo-token-cache/index/README.md) to find source, then read the implementation. Research summaries and historical plans are evidence routes, not instructions overriding this plan. Each borrowed mechanism must remain traceable to its source functions and revision, with required license notices. An independently designed system described as “inspired by Vite/gopls” does not satisfy the assignment.

Source identities verified for planning: SKGO main `53179fc`; Kit `3.0.0` installed through the worktree's frozen lockfile; VitePlus Core `1.0.0` containing Vite `8.3.1`; readable Vite `8.3.4` at `c2c9ef4`; x/tools `v0.52.0` at `16517d9`; selected goloader `2695cfc`. The replacement builder rechecks the actual post-deletion baseline and frontend pin. Reading newer Vite source does not silently upgrade the installed frontend.

| Responsibility | Concrete upstream implementation | Adoption boundary |
|---|---|---|
| Observe Go changes, directory additions, renames and removals | gopls [`fsnotify_watcher.go`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/filewatcher/fsnotify_watcher.go:26): drain events separately from potentially blocking registration; preserve event order; recursively register populated directories | Port its machinery into Go. Adapt its Go-only filters for routes/embeds and its 500 ms diagnostics batching policy for a low-latency dev loop. Do not copy that delay into every save. |
| Retain source and invalidate work | gopls [`invalidateViewLocked`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/view.go:785), [`Snapshot.clone`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:1504), [`ReadFile`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:1077) | Retained immutable file handles, changed-file updates, shared snapshot state and leases. Initial discovery is allowed; rebuilding a custom whole-project capture/tree on every save is not the algorithm. |
| Share pending work and reject stale results | Vite [`transformRequest`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/transformRequest.ts:78), gopls [`future.go`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/future.go:12) | Port admission/reuse rules and cancellation ownership. Vite's `abort` evicts a pending entry; it does not cancel computation. Apply admission before external output writes and native publication too. |
| Limit analysis and generation to affected inputs | gopls [`metadata.Graph`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/metadata/graph.go:20), [`metadataChanges`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/snapshot.go:2032), [`parseCache`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/parse_cache.go:149), [`packageHandle`](/Users/tyler/src/skgo-token-cache/repos/go-tools-v0.52.0/gopls/internal/cache/check.go:817) | Port reverse dependencies, content-keyed parse reuse and staged validity into the existing production/shared Go analysis owners. Keep domain emitters. gopls internals are not public importable APIs. |
| Own frontend transforms and hot-update ordering | Vite [`onFileChange`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/index.ts:966), [`hotUpdate` dispatch](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/node/server/hmr.ts:601), Environment API/HotChannel | Use Vite directly in the necessary adapter boundary. Its graph invalidation precedes `hotUpdate`, including custom environments in installed Vite 8.3.1. No raw watcher bypass or `watcher.emit` interception. |
| Evaluate transformed modules in pooled goja runtimes | Vite [`EvaluatedModules`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/evaluatedModules.ts:11), [`cachedRequest`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/runner.ts:165), [`getModuleInformation`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/runner.ts:267), [`HMR handler`](/Users/tyler/src/skgo-token-cache/repos/vite/packages/vite/src/module-runner/hmrHandler.ts:7) | Replace the deleted runner with a focused Go semantic port: canonical aliases, shared evaluation, cycles, reverse edges, cache/invalidate replies and ordered update payloads. Retain production goja execution/Go bridges; do not rebuild a JavaScript engine or execute app I/O in it. |
| Compile and install Go code | Stock `cmd/go`/compiler and [`pkujhd/goloader`](/Users/tyler/src/skgo-token-cache/repos/reload-native/pkujhd-goloader-current/README.md:10); [native compilation inputs](/Users/tyler/src/skgo-token-cache/index/compiler-retention.md) | Reuse real compiler artifacts/importcfg and the selected loader. Analysis cache keys do not prove native ABI/dependency compatibility. No JIT compiler branch, patched compiler/runtime binary, alternate interpreter, or new loader. |
| Publish Go values to the browser | Vite custom HMR delivery and installed Kit [`refreshAll`](/Users/tyler/.codex/worktrees/dev-server-replacement-plan/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/client.js:2716), [`_invalidate`](/Users/tyler/.codex/worktrees/dev-server-replacement-plan/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/client.js:569) | Send readiness only after admission and installation. Use Kit's own query/load refresh, navigation guards and state preservation. Do not reimplement its client cache or send unconditional full reload after each Go edit. |

gopls/Vite do not supply SKGO's generated bindings, native application ABI, or code-retirement policy. The existing [executed preflights](/Users/tyler/src/skgo-token-cache/index/executed-preflight.md) supply real native load/unload and a manually bridged SKGO remote, including typed errors and cookies. Use that work; do not reopen loader selection or repeat a technology-selection research project. The integration limits below are the remaining application questions.

## Every responsibility has an owner

These are ownership boundaries, not prescribed package names or new public APIs.

| Owner | What it must own |
|---|---|
| Go development session | Startup, stable public listener, watcher lifetime, current desired source, shared work/cancellation, current installed application, diagnostics, process shutdown and update readiness. A complete save-to-browser transition belongs here. |
| Shared Go analysis/generation | Source-bound analysis, dependency and producer inputs, generation outputs and deletion of outputs whose owner disappeared. Generation consumes the selected source revision rather than recapturing unrelated live files. |
| Native application integration | Generated dispatch, actual compiler inputs, compatibility classification, initialized host state, candidate installation and safe lifetime of executing/escaped native values. |
| Vite/adapter and goja integration | Vite's environment/module lifecycle, Kit's live graph/templates, delivery of ordered update payloads, and coherent runtime/manifest use for a render. |
| Kit client | Applying frontend HMR and refreshing active Go values while preserving the state Kit promises to preserve. |

One save flows through observation → retained input update → affected work → candidate generation/compilation → admission against the current source → installation → browser notification → Kit update. Every completion/error is associated with its inputs. Saving again during any stage makes obsolete results ineligible to publish. Generated-output events must not recursively cause rebuild loops or suppress later authored saves.

## Build order and accepted behavior

Deletion is independent work under #321. Replacement implementation begins from its merged result, in a separate worktree. During reconstruction there is one unfinished replacement and no operational old server. The milestones below are engineering checkpoints within that reconstruction, not separately supported development modes.

### 1. Restore a Go-served development session from upstream lifecycle and retained state

**Mission:** A developer can launch the new session, obtain a rendered page and use Go handlers through the public address, then change application structure and have the same session install the result. Build event observation, snapshots, work sharing and stale admission from the named upstream implementations at the outset. The new session can invoke complete builds where needed; it never calls the removed supervisor.

**Accepted behavior:** Startup works in the example and a fresh generated application. A save while the browser is idle starts work. A fixture's old/new literal responses prove installation; an edit during deliberately held work proves the newer edit wins. Route addition/removal and compiler error/repair work without relaunching the session. Requests perform no whole-project discovery. Canceling the session reaps every process it owns; unexpected Vite exit is visible.

**Ownership:** Go session and minimal adapter/application startup integration. Use Vite's environment lifecycle and gopls event/snapshot mechanisms; keep the integration small enough to reach the next milestone promptly. This restores an execution foundation, not the promised native performance result.

### 2. Make useful native edits and browser-state preservation work end to end

**Mission:** Ordinary saves of compatible Go bodies execute new compiled code through actual generated bindings, preserve host state, and update an open Kit page. Use selected GoLoader automatically through ordinary `skgo dev`; no enabling flag, query-specific constant evaluator, manually wired demo, or new annotation developers must add just to get this path.

**Accepted behavior:** Generated applications and the example both demonstrate a request-dependent remote body and a load/endpoint body using a helper and initialized state. Predetermined results prove the new computation executed; literal counter expectations and the same PID prove state retention. Compiler failure keeps the working revision; repair installs the next body. The open page changes while an unsent form field, a local component value and the document identity survive. A command's `skgo.Refresh` still targets the correct query after native replacement. A structural signature/type change is rebuilt by the same session, with its new wire shape reaching Kit.

**Ownership:** Native compiler/registration integration and the narrow adapter-to-Kit readiness bridge. Resolve application lifetime and symbol-identity questions here, before spending a session on cache refinements. If the selected engine cannot support a meaningful body boundary, bring back the exact failing integration case; do not declare a constant-string demo completion or silently restart all bodies.

### 3. Complete Vite's module lifecycle in Go and close the editing surface

**Mission:** Both the next SSR document and Kit's open client see coherent frontend and backend updates. Finish the source-backed Go ModuleRunner port and Vite environment integration, rather than rebuilding the deleted journal/proxy/refresh scheme.

**Accepted behavior:** Changed imported modules reach unchanged importers; aliases do not execute a module twice; cyclic/pending imports settle; changed edges and removals invalidate correctly; and an environment restart reconnects cleanly. Concurrent rendering across the mandatory goja pool cannot combine mismatched route, node and template state. A manifest lease alone is not proof: Vite does not retain old transformed modules merely because Go retains the manifest. Complete route/layout/options/template/SSR-off/CSR-off edits, enhanced and no-script forms, live streams, and error recovery through the real application.

**Ownership:** Vite/adapter and goja integration, with the Go session owning application readiness. Useful HTTP claims become Go tests; browser claims exercise Kit. Any unavoidable render synchronization must cover the actual module/manifest consistency boundary, not preserve old locks for their own sake.

### 4. Remove remaining unnecessary generation and compiler work

**Mission:** Make the common useful native edit fast by retaining the work whose inputs did not change. Extend the gopls-derived package/parse/dependency mechanisms into shared analysis and generation; use native compiler input identities for executable work.

**Accepted behavior:** A body edit changes the executed answer without regenerating unchanged contracts. Type/tag/comment/marker/constant and route-membership edits update every affected generated artifact and leave unrelated producers alone. A changed local dependency, imported constant or embed membership is not missed. A cache hit in generation never substitutes for fresh compilation of changed executable code. No whole-project recapture, discovery, compatibility scan or full link remains on every compatible edit simply under a new name.

**Ownership:** Shared analysis/generation and native build-input reuse; coordinate any necessary polytype changes in its own worktree. Preserve its public Go type-loading/backends and the production emitters. Do not create a second analysis framework or copy arbitrary Go compiler internals into a persistent compiler project.

Milestones 2–4 may expose a prerequisite that moves earlier; that does not authorize reviving old code or postponing the real native/browser demonstration behind an adoption checklist. Choose the next work by its contribution to the developer's edit experience.

## Integration limits to resolve early

1. **Stable application identity and meaningful bodies.** The preflight manually used a stable host anchor; it did not establish generated dispatch or `skgo.Refresh` identity. Native compilation must bind calls and initialized host state correctly, including helpers. Imported/inlined code and types must match the host's actual compiler artifacts. Do not conceal this seam with a literal-expression implementation. Competitor rewrite/dispatch mechanisms in [got-reload](/Users/tyler/src/skgo-token-cache/repos/reload-interpreters-watchers/got-reload/pkg/gotreload/gotreload.go:133) are source references, not permission to introduce Yaegi.
2. **Native lifetime.** GoLoader `Unload` does not drain calls. The preserved POC drains synchronous calls, but does not establish safety for arbitrary returned interfaces, closures, goroutines or deferred work. The SKGO bridge must establish ownership through actual response serialization and any escaping values before retirement. Hold a real call across publication and stress repeated updates with forced GC. If a construct cannot meet that boundary, classify its edit explicitly for a new application build; never unmap code that can still execute. Merely retaining every revision forever does not fulfill bounded long-session behavior.
3. **Source consistency beyond analysis.** gopls snapshots do not supply a compiler filesystem. Adapt the surviving production generation source-view/build facilities so generation, native compilation and full builds consume the admitted bytes. Do not invent a second recursive project copier. The exact multi-output/route startup ordering must be demonstrated, including edits arriving during writes.
4. **Platform/toolchain boundary.** The recorded native proof is released Go 1.27.2 on darwin/arm64. It uses cgo for executable-memory operations and a private source tree exposing Go object packages; compiler/linker/runtime binaries are stock and installed Go is untouched. Reproduce that supported development configuration automatically first. Other environments need real support or an explicit startup diagnostic. Production stays cgo-free and independent of loader preparation. No claims of arbitrary platform support follow from upstream's README alone.
5. **Kit recovery boundary.** Installed Kit's healthy refresh path preserves state; the preflight found a failed-query boundary requiring a document reload. Treat recovery through the actual pinned client as a concrete integration case. Do not promise preservation across every error or reset all healthy pages to avoid handling it.

These are bounded integration tasks. They do not require inventing a scheduler, snapshot model, JavaScript engine, compiler or native loader. A genuine missing upstream mechanism must be described with the source incompatibility and smallest necessary SKGO adaptation before growing into a separate design.

## Completion

Completion requires the entire replacement in ordinary `skgo dev`, including the example, generated applications, recipes and restored dev CI qualification. No optional native path and no resurrected code from #321. Work is not complete at “the loader tests pass,” “the old command name works again,” or “all old tests were ported.”

Use ordinary Go tests at the actual served handler with independent fixture expectations. Only browser-dependent claims belong in the existing Gherkin suite. Change or remove old assertions that prescribed polling, unconditional document reload, request blocking or process death on compile failure. Do not treat those mechanisms as acceptance criteria.

An independent validating agent must check that the relevant assertions actually fail when the capability is broken, and run them. Run `just test` and the existing `just e2e` qualification in both restored modes, including the required main qualification at integration. Missing prerequisites/skips do not pass. Inspect the rendered example and a generated application's real behavior; a green suite beside a broken page is a failed result. No new evidence framework, gate command, proof directory or timing ledger.

Measure real save-to-visible updates on a declared warm application, including observation, analysis, compilation, publication and Kit paint. Report representative nontrivial body edits, a dependency edit and a structural edit separately, with state/document preservation observed. Aim for a few hundred milliseconds or better for ordinary compatible Go edits and Vite's responsive frontend HMR, then remove the measured remaining overhead. Earlier isolated loader timings and the failed string-only branch's timings are historical observations, not this replacement's results.

Implementation remains Go; JavaScript is confined to Kit-required adapter/client glue. VitePlus owns frontend tools. Application I/O stays in Go, SSR stays in pooled goja, and the production application remains one binary with build-time Node only. Unrelated MCP, SDK packaging, add-on or tooling modernization is outside this development-server rebuild.
