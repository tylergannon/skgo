# Windows-shell replacement — independent adversarial review, round 01

Outcome: **material findings remain**.

## Review target

Issue `ephemeral/issue-444.md` (#444), repository `AGENTS.md`, and replacement implementation at `93f9eac0fb69030d030bb7abc04e7c18c2f40d3c` (`codex/windows-shell-client`). The implementation was initially inspected as the working-tree diff and committed during review. The prior `codex/csharp-remote-client` branch was inspected as historical reference and left unchanged.

This review was performed directly as the independent fallback after the normal reviewer CLI failed authentication. No defect-category or scope restrictions were imposed.

## Evidence inspected

- Full relevant diff and new shared-library/Windows-plugin source: model admission, embedded resources, SDK synchronization/ownership, plugin build isolation, plugin surfaces, C# generator/runtime, Zig C ABI/protocol/cache, Swift extraction/integration, workflows, and coexistence contracts.
- Surrounding production generator publication/ownership and source installer code, plus Polytype's grammar validation.
- Installed Kit query-instance/command implementations and current-Kit guidance, including promise ordering versus side-channel cache updates.
- Replacement evidence in `ephemeral/windows-consumer.log`, `ephemeral/swift-client-checks.log`, `ephemeral/plugin-coexistence-checks.log`, and `ephemeral/windows-race-checks.log`: real-handler generated .NET consumer on macOS ARM64; Swift static linkage, real-handler calls and 14 SDK lifecycle contracts; actual plugins loading in either order and generating/exporting together; ordinary race checks.
- Independently ran ordinary tests for Windows-shell, native-client, native-shell, and pluginbuild; all passed. Independently reran generated C# model/real-handler SDK qualification and shared-core wire qualification; their reported results passed.
- Independently compiled and executed the literal-string reproduction below with .NET 10, using source produced by the actual generator.

## Findings

### 1. Issue — Go string escapes silently change admitted C# enum values

**Evidence:** `plugins/windows-shell/csharpgen/csharp.go:92` implements C# quoting with `strconv.QuoteToASCII`. `enumLiteral` uses it for string enum constants at lines 447–452. The same helper supplies union tags and JSON property names.

Go's `\x` escape consumes exactly two hexadecimal digits; C# consumes up to four. A valid string enum constant containing U+0001 followed by `a` is therefore emitted as `"\x01a"`, which C# interprets as U+001A. The generated source compiles, so this silently changes protocol values.

**Reproduction:** Pass `Generate` a definition named `Code` containing a string enum member `Control` whose `constant.MakeString` value is `"\x01a"`, with a root reference to that definition. Compile the generated source in a .NET 10 console and execute:

```csharp
var encoded = ModelCodec.EncodeRoot0(Code.Control)!.GetValue<string>();
Console.WriteLine(string.Join(",", encoded.Select(c => (int)c)));
ModelCodec.DecodeRoot0(JsonValue.Create("\u0001a"));
```

Observed output from the independent probe:

```text
Encoded code points: 26
DECODE ERROR: Unknown enum
```

Expected encoded code points are `1,97`; decoding the literal supported value must succeed. Polytype admits this UTF-8 string constant, and Swift's target-language escape routine preserves it. This violates #444's requirement to generate models from the resolved Go model and match Swift's supported subset. Use C#-correct quoting, such as fixed-width Unicode escapes, and a literal round-trip check for a control character followed by a hexadecimal character.

No other material finding was established. The separation, common core ownership, bundled resources, Apple-only tooling, SDK ownership boundaries, and exercised replacement proof align with the requested architecture.
