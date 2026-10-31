# Windows-shell replacement — independent adversarial review, round 03

Outcome: **no findings**.

## Review target

The complete replacement at `51fa6890d370639bf3e61975277632ca4efb6c2d`, including the full implementation diff from `4c1510b3`, against `ephemeral/issue-444.md` (#444) and repository instructions. The earlier `codex/csharp-remote-client` branch was treated as historical reference and left unchanged. No restrictions on defect categories or review scope were requested or applied.

## Evidence inspected

- Distinct Apple and Windows plugin surfaces/configuration, shared dependency alignment, embedded SDK resource composition and ownership, generator publication boundaries, source installation and isolated plugin building, C# generation/model admission/runtime lifecycle, Zig protocol/cache/C ABI, Swift extraction/static linkage, and separate CI qualification.
- Surrounding production generator/installer code and installed Kit query-instance/command semantics inspected across the review rounds; the unchanged implementation remains within the issue's ordinary-query/command and query-instance-update scope.
- The current literal emitter and compiled model assertions. It now emits ASCII C# source with fixed-width `\u`/`\U` escapes, correctly preserving control characters followed by hexadecimal text, NUL, supplementary Unicode, backslashes, U+0085, U+2028, U+2029 and CR/LF. Both prior findings are resolved.
- Current replacement logs: `ephemeral/windows-client-final-checks.log`, `ephemeral/plugin-coexistence-final-checks.log`, `ephemeral/swift-client-checks.log`, `ephemeral/windows-consumer.log`, and `ephemeral/windows-race-checks.log`.
- Independently reran ordinary tests for Windows-shell, native-client, native-shell and pluginbuild; all passed.
- Independently reran `go test -tags skgo_qualification -count=1 ./plugins/windows-shell/csharpgen ./plugins/windows-shell/sdk`; generated model compilation/literal assertions and the standalone real-handler consumer both passed.

The replacement proof exercises macOS 27.0.1 ARM64, .NET SDK 10.0.403/runtime 10.0.14 and Zig 0.17.0. The consumer demonstrates literal read/change/read-back, retained observation and refreshes, errors/redirects and omission/null, cancellation, shutdown, and reset discarding old completions. Coexistence proof loads both actual plugins in either order, generates and exports repeatedly in one project, and protects peer output and authored files. Swift proof demonstrates generated real-handler calls, static archive linkage and the existing 14 SDK lifecycle contracts after extraction. Apple qualification does not acquire a .NET dependency.

## Findings

No material findings or genuine nitpicks remain. The current implementation and available proof satisfy the bounded replacement outcome in #444. Windows shell/runtime packaging and published-host release certification remain outside the requested proof and are not claimed by this review.
