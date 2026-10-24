const std = @import("std");
const remote = @import("src/routes/[foo]/[bar=nice]/data.remote.zig");
const types = @import("types.zig");

pub fn build(b: *std.Build) void {
    const generated = b.addWriteFiles();
    const ts = generated.add("types.ts", comptime types.typeScriptDecl("Input", remote.Input) ++ "\n" ++ types.typeScriptDecl("Output", remote.Output));
    const swift = generated.add("Types.swift", comptime types.swiftDecl("Input", remote.Input) ++ "\n" ++ types.swiftDecl("Output", remote.Output));
    const generate = b.step("generate", "Generate TypeScript and Swift from native types");
    generate.dependOn(&b.addInstallFile(ts, "generated/types.ts").step);
    generate.dependOn(&b.addInstallFile(swift, "generated/Types.swift").step);
    b.getInstallStep().dependOn(generate);

    const app = b.addExecutable(.{
        .name = "import-probe",
        .root_module = b.createModule(.{
            .root_source_file = b.path("app.zig"),
            .target = b.standardTargetOptions(.{}),
            .optimize = b.standardOptimizeOption(.{}),
        }),
    });
    b.installArtifact(app);
}
