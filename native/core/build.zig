const std = @import("std");
pub fn build(b: *std.Build) void {
    const default_target: std.Target.Query = if (@import("builtin").os.tag == .macos)
        .{ .os_version_min = .{ .semver = .{ .major = 13, .minor = 0, .patch = 0 } } }
    else
        .{};
    const target = b.standardTargetOptions(.{ .default_target = default_target });
    const optimize = b.standardOptimizeOption(.{});
    const d = b.dependency("devalue", .{ .target = target, .optimize = optimize });
    const core = b.addModule("skgo", .{ .root_source_file = b.path("src/root.zig"), .target = target, .optimize = optimize });
    core.addImport("devalue", d.module("devalue"));
    const tests = b.addTest(.{ .root_module = core });
    const run_tests = b.addRunArtifact(tests);
    const test_step = b.step("test", "Run native remote protocol tests");
    test_step.dependOn(&run_tests.step);
    const cli_mod = b.createModule(.{ .root_source_file = b.path("src/cli.zig"), .target = target, .optimize = optimize });
    cli_mod.addImport("skgo", core);
    const cli = b.addExecutable(.{ .name = "skgo-remote", .root_module = cli_mod });
    b.installArtifact(cli);
    const abi = b.createModule(.{ .root_source_file = b.path("src/abi.zig"), .target = target, .optimize = optimize, .link_libc = true });
    abi.addImport("skgo", core);
    const library = b.addLibrary(.{ .name = "skgo_native_core", .root_module = abi, .linkage = .static });
    library.bundle_compiler_rt = true;
    const install_library = b.addInstallArtifact(library, .{});
    b.getInstallStep().dependOn(&install_library.step);
    b.step("library", "Install the native client library without the desktop CLI").dependOn(&install_library.step);
    const abi_tests = b.addTest(.{ .root_module = abi });
    const run_abi_tests = b.addRunArtifact(abi_tests);
    test_step.dependOn(&run_abi_tests.step);
}
