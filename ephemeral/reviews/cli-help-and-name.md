# CLI help execution and plugin naming review

Target: the current staged changes in `/Users/tyler/.codex/worktrees/58f1/skgo`, against the approved outcomes of running routine CLI help/configuration checks in-process and renaming the generator plugin/module/package to `cli`, while preserving generated CLI and MCP functionality and the working example. Reviewed against current `AGENTS.md` and the adversarial-review skill.

Outcome: **no findings**. No material defect or genuine nitpick remained. No scope-narrowing instruction was supplied or ignored. The reviewer did not edit implementation files or commit. All disposable binaries, probes and Go overlays stayed under `plugins/cli/example/.local`.

## Evidence inspected

Read the staged command-dispatch/flag-parser/test changes and the renamed plugin's descriptor, marker, generator, shared client/bridges, scaffold, module files, generated example packages, ownership record, entrypoints, frontend metadata, READMEs and workflow. Also inspected the surrounding command implementations and plugin loading/discovery boundaries. Repository code/workflow searches found no remaining old generator module, publication-marker or generated-package references outside historical material. The unrelated developer-tools MCP interface remains a literal MCP interface.

The executable now calls the same dispatcher that the pure tests invoke. The dispatcher returns statuses and receives input/output streams; command implementations formerly using `ExitOnError` or direct exits return to that boundary. Process exit remains in `main`. Help uses actual flag parsers and asserts command-specific synopses and declared flags. Duplicate selections are checked in-process against a literal diagnostic and status 2. One actual executable-boundary check remains separately available for process status/structured output; the per-command help fanout is gone.

## Tests and load-bearing behavior personally checked

Ran the targeted root tests for the command table, usage inventory, every command's help, dispatch failure statuses, typed environment configuration, and duplicate application selections. All executed and passed without skips. `TestEveryCommandHelpExitsZero` covered 28 subcases and reported **0.09 seconds** in this ordinary targeted run; the complete selected package reported 0.335 seconds. This is measured test execution time, not a claim about compilation or an unmeasured prior baseline.

Then built a disposable test runner with `skgoBinary` overlaid to panic if any pure test requested the compiled CLI helper. Ran the help/configuration/status tests with an empty `PATH`. All passed, including the final exact-diagnostic and temporary-schema-path refinements. Help reported 0.15 seconds in this deliberately instrumented run. This directly establishes that the selected tests do not rely on a second executable or a tool available on `PATH`; it is stronger than counting apparent source-level subprocess calls.

Introduced two further separate disposable regressions, both compiling successfully:

- Bypassed the real `check` command handler so help returned success without output. Both `check -h` and `check --help` tests failed on their required command-specific synopsis.
- Changed duplicate-selection rejection from status 2 to status 1. The in-process exact-diagnostic test failed despite the message staying unchanged.

These failures demonstrate behavioral assertions, rather than merely a zero-exit or nonempty-output check.

In the renamed plugin, independently ran `GOWORK=off go test -count=1 -v ./generator ./scaffold`; all tests passed. In its example, ran `GOWORK=off go test -count=1 -v ./cmd/client`; the real-handler and help tests passed. No broad suite was redundantly rerun while the parent was already running the default suite.

## Actual plugin and running example

Loaded the actual renamed `.local/cli.so` through Go's plugin loader. Asserted its literal descriptor ID `cli` and marker `github.com/tylergannon/skgo/plugins/cli/clipublish#Tool`. Called its actual generator with a fixture and verified all three emitted files are under `internal/cliclient`, have Go package `cliclient`, and contain no obsolete module/package imports. Applied its retained `mcp-server` add-on to a disposable directory and verified the scaffold imports the new client/runtime locations.

Built distinct reviewer server, combined CLI/MCP, and standalone MCP executables from the renamed example. Started only the reviewer server on port 18082, with a process watcher. The user-facing server on port 8080 was not contacted, overwritten, restarted, or stopped by the reviewer.

Demonstrated a literal initial empty list, generated CLI mutation storing `@renamed-cli` as ID 1, real stdio initialization/discovery/mutation/list through the combined executable storing literal `@renamed-mcp` as ID 2, and the same real stdio sequence through the retained standalone MCP executable storing `@retained-mcp` as ID 3. Both MCP entrypoints exposed exactly the unchanged `addTask` and `listTasks` names and their Go-comment descriptions.

Opened the reviewer application in Chrome. Its real Kit page showed exactly those three tasks. Submitted `Renamed example browser task` through the page, inspected its screenshot showing count 4 and all four literal entries, then listed through the actual CLI and observed the exact same four records, including browser-created ID 4. No blank screen, error boundary or stack trace appeared. The renamed example therefore works across the browser and both compiled interfaces; generated files and test exit codes were not used as substitutes for that demonstration.

## Compatibility assessment

This is an intentional source/plugin-identity rename: existing consumers of the former module, markers, generated package, or plugin ID must update those references and regenerate. The current example, module replacements, scaffold, output ownership record, and CI paths all use the new names consistently. No old-name compatibility layer was requested, and none is presumed. Already compiled MCP transport semantics and the published tool names remain unchanged. External repositories and release qualification were not run or silently claimed as covered by this worktree review.

Final-source refinements inspected include the `skgo-cli-task-list` frontend package name, the literal duplicate-selection diagnostic, the temporary missing-schema fixture, and preservation of the original root-help behavior for optional registry write errors. `git diff --cached --check` passed.

Cleanup: closed the reviewer browser tab and stopped only the reviewer server and its watcher. The parent's user-facing server remains outside reviewer control.
