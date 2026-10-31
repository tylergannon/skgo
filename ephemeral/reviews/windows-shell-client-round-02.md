# Windows-shell replacement — independent adversarial review, round 02

Outcome: **material findings remain**.

## Review target

The complete replacement at `49b26cc8011a305a18d4339a5727411f330d8d53`, against `ephemeral/issue-444.md` and repository instructions. Reviewed the current implementation, the full replacement diff from `4c1510b3`, surrounding code and proof examined in round 01, the round-01 finding, and the new literal-quoting fix. The historical `codex/csharp-remote-client` branch remains unchanged. No review-scope narrowing was requested or applied.

## Evidence inspected

- Distinct Apple/Windows plugin surfaces and configuration, common Zig/C ABI resources and model validation, SDK ownership and generation publication boundaries, build isolation and dependency alignment, C# emission and managed transport/cache lifecycle, Apple integration and static linkage, CI separation, and same-project coexistence contracts.
- Existing real-handler Swift/static-link proof and shared-core contracts inspected in round 01; source for those features is unchanged in the new fix.
- Current `ephemeral/windows-client-final-checks.log` and `ephemeral/plugin-coexistence-final-checks.log`: model and real-handler generated consumer qualification pass on macOS ARM64, .NET SDK 10.0.403/runtime 10.0.14 and Zig 0.17.0; both real plugins load in either order, generate and export repeatedly, and preserve peer/authored output.
- Independently reran `go test -tags skgo_qualification -count=1 ./plugins/windows-shell/csharpgen ./plugins/windows-shell/sdk`; both packages passed.
- Independently generated and compiled a C# model with control and Unicode string enum literals using the actual generator and .NET 10. This exposed the finding below.

## Findings

### 1. Issue — JSON quoting leaves a C# source newline unescaped

**Evidence:** `plugins/windows-shell/csharpgen/csharp.go:93–99` now returns `json.Marshal(s)` as a C# regular string literal. JSON quoting correctly fixes the round-01 greedy hexadecimal escape, but JSON permits a literal U+0085 (NEXT LINE). C# interprets U+0085 as a source newline and rejects it inside a regular string literal. String enum values are admitted UTF-8 constants, so this input remains inside the required supported model subset.

**Reproduction:** Generate a named string enum `Code` with member `NEL` whose `constant.MakeString` value is `"\u0085a"`, and a root referencing that enum. Add the generated file to a .NET 10 console project with the same namespace stub used by the existing model qualification, then build. The generator returns success; its encoder and decoder contain a literal U+0085 inside quoted C# strings. The independently executed build failed with:

```text
Generated.cs(72,31): error CS1010: Newline in constant
Generated.cs(73,2): error CS1010: Newline in constant
Generated.cs(81,1): error CS1010: Newline in constant
The build failed. Fix the build errors and run again.
```

This regresses a value the previous ASCII quoting escaped as `\u0085`, and prevents compilation of otherwise supported Go models. It violates #444's resolved-model and Swift-subset requirements. Ensure every C# source newline character, including U+0085, is escaped; retain fixed-width escapes for control characters followed by hexadecimal text. Compile and round-trip literal expectations for both cases.

The original round-01 U+0001-followed-by-`a` corruption is corrected by the current implementation and covered by the new literal assertions. No other material finding was established in the complete replacement review.
