# Workspace boundary audit

Target: the proposed workspace cleanup and the current compiler/plugin staging boundaries in this checkout. This is a pre-implementation audit, not final validation of edits. No implementation files were changed. No other checkout was consulted.

Outcome: **material findings remain** in the baseline. The requested direction is sound; preserve the requirements below while implementing it.

## Module inventory

The six actual project modules are the repository root, `example`, `plugins/cli`, `plugins/cli/example`, `plugins/native-shell`, and `plugins/native-shell/example`. They all declare Go 1.27.2. The current workspace includes only the first two, so four legitimate modules are missing.

`plugins/native-shell/example/web/src/routes/go.mod` is a generated traversal barrier, explicitly declaring itself never built. It is not a seventh project module. Remove it while adding the route directory to its owning example module's ignore list, then regenerate/compile that example's linked route packages. No concrete reason for excluding any of the six actual modules was found.

## Material preservation requirements

1. **Issue: ordinary membership and staged graphs have different owners.** `internal/pluginbuild/build.go` stages plugin source and reconstructs the invoking host's exact dependency graph; it also uses writable module resolution. That graph must not accidentally select this checkout's workspace or an explicitly inherited caller workspace. `internal/plugininstall/source.go`, `cmd/skgo-release/main.go`, and the release-version lookup in `internal/newapp/newapp.go` also execute Go commands for disposable or published source rather than the current checkout. Preserve their intended working directory and dependency selection after removing the existing environment overrides. An inherited absolute workspace selector can still escape a temporary directory; changing only the working directory is insufficient.

2. **Issue: the native compiler cannot blindly inherit workspace selection during private module edits.** `internal/hotreload/view.go` creates a private host module declaration, then prepares a loader before constructing a private workspace. Go forbids a separate module-file flag while a workspace is selected. `internal/hotreload/loader.go` also edits the copied loader's own declaration. Preserve those private edits, all captured application workspace members, and workspace replacements without modifying authored module files or relying on the removed overrides.

3. **Issue: captured analysis must remain a captured graph.** `internal/gen/cache_source.go` materializes captured module declarations and rebases workspace members and local replacements into private paths. `internal/gen/package_stage.go` substitutes staged package views in the active workspace. Removing an environment assignment must not send either analyzer back to live original source, drop another workspace member, or leave a relative replacement anchored to its former directory. The same requirement applies to `internal/hotreload/toolchain.go` defaults and fresh-process cache probes. Keep the production interpretation of ordinary caller environment settings; remove repository-imposed disabling, not the ability to observe caller configuration.

## Final validation targets

- Verify exactly the six real modules are workspace members, and that no generated route barrier remains.
- Compile and test each module through the shared workspace. A root `./...` pattern does not enumerate nested modules merely because the workspace includes them.
- In particular, the native example previously selected an older released core dependency. Workspace membership now selects current local core code, so compile/regenerate it rather than assuming the change is metadata-only.
- Exercise the existing multi-module capture/rebase cache test, staged embed/snapshot tests, native host workspace test, and plugin source/build/installation tests. Their existing literal fixtures and source-preservation assertions are useful guards for the boundaries above.
- Verify developer instructions, workflows and runtime children no longer impose the removed environment setting, including tests and historical working material. Removal of obsolete cross-checkout content must leave coherent local instructions rather than orphaned links or procedural fragments.
- Recheck plugin/host compatibility through the normal shared workspace; matching toolchain/dependency graphs remain required even when every module now appears in one workspace.

Final implementation validation will follow when the completed edits are available. No new build machinery or acceptance runner is required by this audit.

## Final implementation review

The staged implementation has now been reviewed. This section supersedes the baseline assessment above. No other checkout was consulted, no implementation files were edited by the reviewer, and no publication action was performed.

The three substantive baseline requirements are satisfied:

- Go reports exactly six main modules, matching the intended inventory. The obsolete route barrier is gone; the native example ignores its route tree and uses regenerated current bindings and a generated link. Package listing for that example and the CLI module reports no package errors. Test and vet recipes explicitly cover all six modules, so they do not rely on a root-only pattern to discover nested modules.
- Plugin/source staging clears inherited explicit workspace selection and permits ordinary discovery in its disposable source root. Host graph pins and source-copy protections remain intact. Native loader download no longer receives the host's alternate module file, while private declaration edits still use the edit command's supported module-file behavior. The final compiler workspace retains captured application members and replacements. Clearing selection is distinct from suppressing discovery.
- Captured analysis and staged package views still rebase selected workspace files and module replacements into their private views. The changes do not replace those captured graphs with the live checkout. Production code continues to interpret caller settings; it no longer imposes the removed setting.

### Independent checks

Personally executed and passed the existing tests for source-text rebasing, staged embeds, snapshot admission, route-tree ignoring, plugin source-copy boundaries, successful/failed/canceled plugin publication, retained caller/dependency guards, rejected native compiler profiles, application version selection/creation, and Go pin consistency. All selected test cases executed; no skipped or empty selection was counted as evidence. An initially unmatched application-test selector was replaced with three explicit existing tests, which passed.

The source-text rebase test is a focused string-mapping check, not an actual multi-module compilation witness. The implementer separately reported a passing existing captured native HTTP host test in 10.53 seconds and the existing actual capability CLI/plugin test in 14.109 seconds, both under the pinned toolchain. I inspected the host test's literal expectations: the real HTTP response must include independently specified workspace-member and workspace-replacement values, both before and after a body edit. Dropping either selected dependency cannot satisfy those assertions. Those already-completed expensive checks were not redundantly rerun by this reviewer.

The implementer also reported the default all-six-module suite, shared-workspace plugin build/generation, workspace synchronization, and linting of changed workflows passed. My independent checks complement those results; they do not claim independent reruns of those broader commands.

Tracked-text scans found zero removed assignments or legacy identifiers. The current workflow diff removes the retired dependencies while retaining local release identity, artifact integrity, source provenance, and existing approval-environment controls. Read the changed local instructions and historical-note edits for coherence. The staged diff has no whitespace errors. Removed the reviewer's own disposable build/probe artifacts after their results were recorded.

### Remaining finding

**Nitpick — orphaned companion-change assessment.** `ephemeral/reviews/date-transport-wire-round-01.md:5` still assesses an unnamed consumer change and refers to two diffs after that companion context was removed. This does not affect product behavior, but it misses the user's explicit requirement to leave the remaining prose coherent. Keep the assessment limited to the local core change. Reported to the implementer; no other material finding remains.

Cleanup scope is the current tracked source tree, workflows and historical working notes. Git object history has not been rewritten. The user-facing running application was not touched by the reviewer.

Final outcome: **only nitpicks remain**.

### Final adjudication

Read the corrected historical assessment. It now addresses only the local core change, with no orphaned companion-change sentence or reference to a second diff. The sole nitpick is resolved. This prose-only correction requires no repeated test run.

Final outcome after reconciliation: **no findings**.
