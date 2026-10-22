# Generated CLI and retained MCP review

Review target: the current uncommitted implementation in `/Users/tyler/.codex/worktrees/58f1/skgo`, particularly `plugins/mcp`, its task-list example, and `ephemeral/worklog/generated-cli.md`. Reviewed against the user's approved design and example-based definition of done, current `AGENTS.md`, and the adversarial-review skill.

Outcome: **no findings**.

No material defect or genuine nitpick remained after the checks below. No narrowing instructions were supplied or ignored. Implementation source was not edited, and no commits were created.

## Evidence inspected

Read the relevant tracked diff and new implementation files: command generation and schema projection, CLI bridge, shared JSON conversion, request client callback, MCP bridge, application-owned command composition, generated example commands, task handlers, authentication entrypoint, plugin/example READMEs, dependencies, output ownership manifest, and associated tests. Verified both installed Kit packages report `3.1.0` and read `ephemeral/sveltekit-current/SKILL.md`. This change does not introduce a new Kit protocol or mirrored behavior.

## Checks personally run

- In the plugin module: `go test -count=1 -v ./...` passed. All selected tests executed; there were no skips.
- In its example module: `go test -count=1 -v ./...` passed, including literal expected task IDs, exact file/newline preservation, missing-file rejection, invalid input rejection, authentication, shared CLI/MCP handlers, and help without a connection.
- Built the actual server, combined CLI/MCP binary, and retained standalone MCP binary under distinct `.local/reviewer-*` names.
- `git diff --check` passed.

The existing frontend build was used to run the actual server; I did not regenerate outputs or independently repeat the frontend build, because this review was read-only for implementation files. The running page visibly contained the updated CLI/MCP text.

## Live example, independently demonstrated

Started a separate authenticated task-list process on loopback port 18081, with a watcher attached to its process. Its state began with the independently expected empty list. Used the actual compiled CLI, a real browser, the actual stdio MCP binaries, and the generated Go client against this server.

1. Generated `tasks list` returned exactly `{"result":[]}`.
2. Generated `tasks add --title @...` read a local fixture containing `@reviewer file\nsecond line\n`. The response retained the leading at-sign and both newlines exactly, with ID 1.
3. Generated `--title @@reviewer` stored literal `@reviewer`, with ID 2.
4. The application-authored `tasks add-file` command read the same fixture and used the generated typed client, returning the same exact content with ID 3.
5. Generated complete JSON input stored literal `@literal-json`, with ID 4.
6. A missing `@file` produced the documented escape hint; a wrong API key produced HTTP 401. Listing afterward contained exactly those four tasks, so neither failure added a task.
7. Opened the real Kit page in the in-app browser with the example's Basic authentication. Its rendered list contained those four entries. Submitted `Reviewer browser task` through the page's form; the page displayed count 5 and the new task with ID 5.
8. Through a real stdio MCP client, initialized the combined executable's `mcp` mode, discovered exactly `addTask` and `listTasks` with their Go-comment descriptions, and called `addTask` with literal `@MCP-reviewer`. It returned ID 6 without attempting file expansion. MCP list showed all prior CLI and browser records.
9. Repeated initialization, discovery, mutation, and list through the retained standalone MCP executable. Literal `@old-MCP-reviewer` became ID 7.
10. A direct generated Go client stored literal `@.local/reviewer-title.txt` as ID 8 even though that file actually existed. It did not read its contents.
11. The direct client was configured with a callback capable of returning an error and an instrumented HTTP transport. One successful mutation dispatched once; a callback rejection dispatched zero requests and preserved both the original error and `ErrNotSent`; a subsequent list showed exactly eight tasks. The instrumented counts were three callback executions and two HTTP dispatches.
12. Refreshed the browser and inspected its screenshot. It displayed all eight entries, including CLI, custom-command, browser, both MCP, and direct-client mutations. No blank page, error boundary, or stack trace appeared.

Also performed the parent's requested real stdio MCP mutation against the parent's separate process on port 8080. It returned exactly `{"result":{"id":3,"title":"Created through MCP"}}`; MCP list returned the two known existing records and this third record. The parent was notified so it could independently inspect the same page. That process was not stopped or restarted.

The built command's detailed help included the Go function description, input and result type/field comments, required title flag, at-sign convention, application-authored examples, and inherited authentication/connection flags. Help worked without contacting the application and did not disclose the environment API key.

## Load-bearing checks

Used disposable Go `-overlay` inputs rather than editing implementation files:

- Replaced string expansion with an identity function. Existing bridge tests failed on file contents, double-at escaping, one-pass escaping, and missing-file handling.
- Disabled the outgoing callback. The request-client test failed because the actual transport did not receive the expected prepared header.
- Allowed dispatch after a callback error. The same test failed when the rejected call reached the transport without its required prepared header.

These are behavioral failures after successful compilation, not assertions about source text. The real-handler example test additionally anchors task IDs, complete output, and rejected-mutation absence in literal fixtures rather than a counter sampled from the implementation.

## Assessment and boundaries

The example demonstrates the approved capability: individual generated Cobra constructors, application-owned composition and custom commands, Go-comment help, shared typed HTTP execution, retained MCP tools, optional per-request authentication customization and rejection, strict CLI file/escape behavior, and literal MCP/direct-client strings. The shared task state was observed through the actual browser and both compiled interfaces. Passing test commands were not treated as the definition of done.

Binary size was not measured; no binary-size claim is made. This review is for the requested plugin example capability, not release qualification for the whole SKGo project.

## Final-source reconciliation

After the initial review, the parent made three bounded example refinements. I inspected the final source and independently reran `go test -count=1 -v ./cmd/client`; both tests passed with no skips. The real-handler test now additionally invokes the application-authored `add-file` command and asserts literal ID 5, exact `@from-file\n` contents, and the complete five-task list.

Rebuilt the reviewer server and combined client from those final sources. Ran server help with a known environment API key and asserted the key was absent from both output streams. The key is now read after flag parsing, so help cannot print it as a flag default. Ran the custom `add-file` command against a fixture containing invalid UTF-8 with an intentionally unreachable server origin; it exited 1 with `file must contain UTF-8 text` before any network attempt. No findings arose from these changes. Earlier live browser/CLI/MCP demonstrations remain applicable because the shared generator, client, transport, and CLI bridge did not change.

Final outcome: **no findings**. The isolated reviewer server and browser tab have been closed; the parent's server remains untouched.
