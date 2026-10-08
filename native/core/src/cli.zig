const std = @import("std");
const core = @import("skgo");

// Command-line transport for development, not the Apple platform transport.
// Input and output are devalue documents; application JSON codecs are unneeded.
pub fn main(init: std.process.Init) !void {
    const a = init.gpa;
    const args = try init.minimal.args.toSlice(init.arena.allocator());
    if (args.len != 6) {
        std.debug.print("usage: skgo-remote ORIGIN BASE ID query|command DEVALUE\n", .{});
        return error.InvalidArguments;
    }
    const kind: core.Kind = if (std.mem.eql(u8, args[4], "query")) .query else if (std.mem.eql(u8, args[4], "command")) .command else return error.InvalidArguments;
    var argument = try core.devalue.parse(a, args[5], &.{});
    defer argument.deinit();
    var request = try core.prepare(a, .{ .origin = args[1], .base = args[2] }, args[3], kind, &argument.graph, argument.value, &.{});
    defer request.deinit();
    var client: std.http.Client = .{ .allocator = a, .io = init.io };
    defer client.deinit();
    var output: std.Io.Writer.Allocating = .init(a);
    defer output.deinit();
    const fetched = try client.fetch(.{
        .location = .{ .url = request.url },
        .method = if (kind == .query) .GET else .POST,
        .payload = request.body,
        .response_writer = &output.writer,
        .redirect_behavior = .unhandled,
        .headers = .{ .content_type = .{ .override = "application/json" } },
        .extra_headers = &.{.{ .name = "Origin", .value = request.origin }},
    });
    var response = try core.receive(a, @intCast(@backingInt(fetched.status)), output.written());
    defer response.deinit();
    if (response.kind != .result) {
        std.debug.print("{s} ({d}): {s}\n", .{ @tagName(response.kind), response.status orelse response.http_status, response.message orelse "Remote call failed" });
        return error.RemoteCallFailed;
    }
    const wire = try core.devalue.stringify(a, &response.parsed.?.graph, response.value, &.{});
    defer a.free(wire);
    var buffer: [4096]u8 = undefined;
    var stdout = std.Io.File.stdout().writer(init.io, &buffer);
    try stdout.interface.writeAll(wire);
    try stdout.interface.writeByte('\n');
    try stdout.interface.flush();
}
