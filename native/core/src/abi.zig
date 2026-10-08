const std = @import("std");
const core = @import("skgo");
const d = core.devalue;
const a = std.heap.c_allocator;
const Bytes = extern struct { ptr: ?[*]const u8 = null, len: usize = 0 };
const Buffer = extern struct { ptr: ?[*]u8 = null, len: usize = 0 };
const Request = extern struct { url: Buffer = .{}, origin: Buffer = .{}, body: Buffer = .{} };
const Reply = extern struct { kind: u32 = 0, status: u32 = 0, value: Buffer = .{}, message: Buffer = .{} };
fn input(v: Bytes) ![]const u8 {
    if (v.len == 0) return &.{};
    return (v.ptr orelse return error.InvalidInput)[0..v.len];
}
fn buffer(v: []u8) Buffer {
    return .{ .ptr = v.ptr, .len = v.len };
}
fn status(err: anyerror) u32 {
    return switch (err) {
        error.OutOfMemory => 1,
        error.UnsupportedValue, error.UnsupportedString, error.DepthLimit => 3,
        else => 2,
    };
}
// Every returned buffer is independent of input, and is released exactly once.
fn release(allocator: std.mem.Allocator, b: *Buffer) void {
    if (b.ptr) |ptr| allocator.free(ptr[0..b.len]);
    b.* = .{};
}
pub export fn sk_buffer_release(b: *Buffer) void {
    release(a, b);
}

fn prepare(allocator: std.mem.Allocator, origin: []const u8, base: []const u8, id: []const u8, command: u32, argument: []const u8) !core.Request {
    if (command > 1) return error.InvalidInput;
    var graph = d.Graph.init(allocator);
    defer graph.deinit();
    var parsed: ?std.json.Parsed(std.json.Value) = null;
    defer if (parsed) |*p| p.deinit();
    const value: d.Value = if (argument.len == 0) .undefined else blk: {
        parsed = try std.json.parseFromSlice(std.json.Value, allocator, argument, .{});
        break :blk try core.model_json.fromJSON(&graph, parsed.?.value, 0);
    };
    return core.prepare(allocator, .{ .origin = origin, .base = base }, id, if (command == 1) .command else .query, &graph, value, &.{});
}
fn prepareBuffers(allocator: std.mem.Allocator, origin: []const u8, base: []const u8, id: []const u8, command: u32, argument: []const u8) !Request {
    const request = try prepare(allocator, origin, base, id, command, argument);
    allocator.free(request.payload);
    return .{ .url = buffer(request.url), .origin = buffer(request.origin), .body = if (request.body) |body| buffer(body) else .{} };
}
pub export fn sk_prepare(origin: Bytes, base: Bytes, id: Bytes, command: u32, argument: Bytes, out: *Request) u32 {
    out.* = .{};
    out.* = prepareBuffers(a, input(origin) catch |e| return status(e), input(base) catch |e| return status(e), input(id) catch |e| return status(e), command, input(argument) catch |e| return status(e)) catch |e| return status(e);
    return 0;
}

fn resultJSON(allocator: std.mem.Allocator, response: *core.Response) ![]u8 {
    return core.model_json.encode(allocator, &response.parsed.?.graph, response.value);
}
fn decodeBuffers(allocator: std.mem.Allocator, http_status: u16, body: []const u8) !Reply {
    var response = try core.receive(allocator, http_status, body);
    defer response.deinit();
    const value = if (response.kind == .result and response.value != .undefined) try resultJSON(allocator, &response) else null;
    errdefer if (value) |v| allocator.free(v);
    const message = if (response.message) |m| try allocator.dupe(u8, m) else null;
    return .{ .kind = @backingInt(response.kind), .status = response.status orelse response.http_status, .value = if (value) |v| buffer(v) else .{}, .message = if (message) |m| buffer(m) else .{} };
}
pub export fn sk_decode(http_status: u16, body: Bytes, out: *Reply) u32 {
    out.* = .{};
    out.* = decodeBuffers(a, http_status, input(body) catch |e| return status(e)) catch |e| return status(e);
    return 0;
}
fn allocationCase(allocator: std.mem.Allocator) !void {
    var request = try prepareBuffers(allocator, "http://127.0.0.1:8080", "", "hash/call", 1, "{\"text\":\"copied input\"}");
    defer {
        release(allocator, &request.url);
        release(allocator, &request.origin);
        release(allocator, &request.body);
    }
    var response = try decodeBuffers(allocator, 200, "{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},{\\\"text\\\":2},\\\"owned result\\\"]\"}");
    defer {
        release(allocator, &response.value);
        release(allocator, &response.message);
    }
    try std.testing.expectEqualStrings("{\"text\":\"owned result\"}", response.value.ptr.?[0..response.value.len]);
    var failure = try decodeBuffers(allocator, 200, "{\"type\":\"error\",\"error\":{\"status\":409,\"message\":\"Revision conflict\"}}");
    defer {
        release(allocator, &failure.value);
        release(allocator, &failure.message);
    }
    try std.testing.expectEqualStrings("Revision conflict", failure.message.ptr.?[0..failure.message.len]);
}
test "owned buffers release every allocation failure" {
    // Refuse resize/remap during exhaustive allocation failure injection so
    // SafeAllocator's address-dependent growth cannot change allocation counts.
    var no_growth = std.testing.FailingAllocator.init(std.testing.allocator, .{ .resize_fail_index = 0 });
    try std.testing.checkAllAllocationFailures(no_growth.allocator(), allocationCase, .{});
}
test "release zeroes buffers and result bytes outlive parsing" {
    var response = try decodeBuffers(std.testing.allocator, 200, "{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},\\\"owned\\\"]\"}");
    try std.testing.expectEqualStrings("\"owned\"", response.value.ptr.?[0..response.value.len]);
    release(std.testing.allocator, &response.value);
    try std.testing.expect(response.value.ptr == null and response.value.len == 0);
    release(std.testing.allocator, &response.value);
}
