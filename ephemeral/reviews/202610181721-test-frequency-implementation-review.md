# Test-frequency implementation review

Target: current worktree changes against `origin/main`, implementing the accepted `20261018-test-frequency-astra.md` recommendation. Review performed independently and read-only except for this artifact. No scope exclusions were accepted; the caller's ownership descriptions were treated as coordination context.

Outcome: **no findings** in the final inspected revision.

Evidence inspected: AGENTS.md and agent-protocol/adversarial-review instructions; accepted review; complete CI, qualification and release workflows; Justfile; Bob runner Dockerfile/Compose; native Zig build and remote tests; qualification-tag changes and selected test names; generator emitter and retained-consumer changes; adapter embedding/test helpers; add-on package metadata and tests; root development bridge and focused development-session fixture.

Three material points raised during review were corrected before this verdict:

- Candidate selection now diffs from the branch merge base. Previously a second, docs-only push could cancel the first qualification run and skip its replacement because selection used `event.before`.
- Development selection now includes root Go runtime files and compiler/toolchain inputs. Previously edits to the actual `dev_bridge.go` and `dev_kit.go` boundary skipped development qualification.
- The cheap add-on test now checks that the root export names `sv-addon.js` and that the publication file declaration includes that bundle. Removing package-manager subprocess tests consequently retains the relevant SKGO-owned declaration assertion.

Final inspection also confirmed explicit compiler-witness selection, native replacement on Linux/macOS, Zig exchange on both platforms, and the Swift consumer on macOS. Trusted Linux jobs retain local Go caches on Bob; fork PR jobs select hosted runners. Ordinary CI excludes main and tags. Release qualification defaults to all selected witnesses, shares the built example between handler and browser checks, and does not repeat ordinary repository tests. Add-on preparation builds and stamps one package on Bob, uploads that tarball, and hosted npm publication consumes it only after qualification and CLI jobs succeed. GitHub release publication also requires successful package preparation.

Validation performed here: `go test ./internal/sv` passed (0.256s package time); `git diff --check` passed. This is workflow/implementation review, not evidence that GitHub Actions, macOS qualification, npm publication, or the complete application suite ran successfully. No full suites were rerun for this review. Memory search yielded no relevant evidence used in the verdict.
