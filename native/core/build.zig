const std = @import("std");
pub fn build(b: *std.Build) void {
    const target = b.standardTargetOptions(.{});
    const optimize = b.standardOptimizeOption(.{});
    const d = b.dependency("devalue", .{ .target = target, .optimize = optimize });
    const core = b.addModule("skgo", .{ .root_source_file = b.path("src/root.zig"), .target = target, .optimize = optimize });
    core.addImport("devalue", d.module("devalue"));
    const tests = b.addTest(.{ .root_module = core });
    const run_tests = b.addRunArtifact(tests);
    b.step("test", "Run native remote protocol tests").dependOn(&run_tests.step);
    const cli_mod = b.createModule(.{ .root_source_file = b.path("src/cli.zig"), .target = target, .optimize = optimize });
    cli_mod.addImport("skgo", core);
    const cli = b.addExecutable(.{ .name = "skgo-remote", .root_module = cli_mod });
    b.installArtifact(cli);
}
