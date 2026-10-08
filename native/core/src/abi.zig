const std = @import("std");
const core = @import("skgo");
const d = core.devalue;
const a = std.heap.c_allocator;
const Bytes = extern struct { ptr: ?[*]const u8 = null, len: usize = 0 };
const Buffer = extern struct { ptr: ?[*]u8 = null, len: usize = 0 };
const Request = extern struct { url: Buffer = .{}, origin: Buffer = .{}, body: Buffer = .{}, key: Buffer = .{} };
const Ticket = extern struct { epoch: u64 = 0, serial: u64 = 0 };
const State = extern struct { ready: u32 = 0, loading: u32 = 0, status: u32 = 0, value: Buffer = .{}, message: Buffer = .{} };
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
        error.CacheFull => 4,
        error.NotRetained => 5,
        error.Exhausted => 6,
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

fn prepare(allocator: std.mem.Allocator, origin: []const u8, base: []const u8, id: []const u8, command: u32, argument: []const u8, refreshes: []const []const u8) !core.Request {
    if (command > 1 or (command == 0 and refreshes.len > 0)) return error.InvalidInput;
    var graph = d.Graph.init(allocator);
    defer graph.deinit();
    var parsed: ?std.json.Parsed(std.json.Value) = null;
    defer if (parsed) |*p| p.deinit();
    const value: d.Value = if (argument.len == 0) .undefined else blk: {
        parsed = try std.json.parseFromSlice(std.json.Value, allocator, argument, .{});
        break :blk try core.model_json.fromJSON(&graph, parsed.?.value, 0);
    };
    return core.prepare(allocator, .{ .origin = origin, .base = base }, id, if (command == 1) .command else .query, &graph, value, refreshes);
}
fn prepareBuffers(allocator: std.mem.Allocator, origin: []const u8, base: []const u8, id: []const u8, command: u32, argument: []const u8, refreshes: []const []const u8) !Request {
    var request = try prepare(allocator, origin, base, id, command, argument, refreshes);
    errdefer request.deinit();
    const key = if (command == 0) try request.queryKey(id) else null;
    allocator.free(request.payload);
    return .{ .url = buffer(request.url), .origin = buffer(request.origin), .body = if (request.body) |body| buffer(body) else .{}, .key = if (key) |k| buffer(k) else .{} };
}
pub export fn sk_prepare(origin: Bytes, base: Bytes, id: Bytes, command: u32, argument: Bytes, refreshes: Bytes, out: *Request) u32 {
    out.* = .{};
    const parsed = std.json.parseFromSlice([][]const u8, a, input(refreshes) catch |e| return status(e), .{}) catch |e| return status(e);
    defer parsed.deinit();
    out.* = prepareBuffers(a, input(origin) catch |e| return status(e), input(base) catch |e| return status(e), input(id) catch |e| return status(e), command, input(argument) catch |e| return status(e), parsed.value) catch |e| return status(e);
    return 0;
}

fn resultJSON(allocator: std.mem.Allocator, response: *core.Response) ![]u8 {
    return core.model_json.encode(allocator, &response.parsed.?.graph, response.value);
}
fn decodeBuffers(allocator: std.mem.Allocator, http_status: u16, body: []const u8) !Reply {
    var response = try core.receive(allocator, http_status, body);
    defer response.deinit();
    return replyBuffers(allocator, &response);
}
fn replyBuffers(allocator: std.mem.Allocator, response: *core.Response) !Reply {
    const value = if (response.kind == .result and response.value != .undefined) try resultJSON(allocator, response) else null;
    errdefer if (value) |v| allocator.free(v);
    const message = if (response.message) |m| try allocator.dupe(u8, m) else null;
    return .{ .kind = @backingInt(response.kind), .status = response.status orelse response.http_status, .value = if (value) |v| buffer(v) else .{}, .message = if (message) |m| buffer(m) else .{} };
}
pub export fn sk_decode(http_status: u16, body: Bytes, out: *Reply) u32 {
    out.* = .{};
    out.* = decodeBuffers(a, http_status, input(body) catch |e| return status(e)) catch |e| return status(e);
    return 0;
}
// One private client cache context; public Swift models never contain Zig handles.
pub export fn sk_cache_create(capacity: u32, out: *?*core.QueryCache) u32 {
    out.* = null;
    const cache = a.create(core.QueryCache) catch |e| return status(e);
    cache.* = core.QueryCache.init(a, capacity) catch |e| {
        a.destroy(cache);
        return status(e);
    };
    out.* = cache;
    return 0;
}
pub export fn sk_cache_release(out: *?*core.QueryCache) void {
    if (out.*) |cache| {
        cache.deinit();
        a.destroy(cache);
    }
    out.* = null;
}
pub export fn sk_cache_retain_query(cache: *core.QueryCache, key: Bytes) u32 {
    cache.retain(input(key) catch |e| return status(e)) catch |e| return status(e);
    return 0;
}
pub export fn sk_cache_release_query(cache: *core.QueryCache, key: Bytes) u32 {
    cache.release(input(key) catch |e| return status(e)) catch |e| return status(e);
    return 0;
}
pub export fn sk_cache_reset(cache: *core.QueryCache) u32 {
    cache.reset() catch |e| return status(e);
    return 0;
}
pub export fn sk_cache_begin(cache: *core.QueryCache, key: Bytes, refresh: u32, out: *Ticket) u32 {
    out.* = .{};
    if (refresh > 1) return 2;
    const ticket = cache.begin(input(key) catch |e| return status(e), refresh == 1) catch |e| return status(e);
    out.* = .{ .epoch = cache.epoch, .serial = if (ticket) |t| t.serial else 0 };
    return 0;
}
pub export fn sk_cache_waiting(cache: *core.QueryCache, key: Bytes, ticket: Ticket) u32 {
    return @intFromBool(cache.waiting(input(key) catch return 0, .{ .epoch = ticket.epoch, .serial = ticket.serial }));
}
pub export fn sk_cache_abort(cache: *core.QueryCache, key: Bytes, ticket: Ticket) void {
    cache.abort(input(key) catch return, .{ .epoch = ticket.epoch, .serial = ticket.serial });
}
pub export fn sk_cache_reject(cache: *core.QueryCache, key: Bytes, ticket: Ticket, fault_status: u16, message: Bytes) u32 {
    _ = cache.reject(input(key) catch |e| return status(e), .{ .epoch = ticket.epoch, .serial = ticket.serial }, .{ .status = fault_status, .message = input(message) catch |e| return status(e) }) catch |e| return status(e);
    return 0;
}
pub export fn sk_cache_snapshot(cache: *core.QueryCache, key: Bytes, out: *State) u32 {
    out.* = .{};
    const snapshot = cache.snapshot(input(key) catch |e| return status(e)) catch |e| return status(e);
    out.* = .{ .ready = @intFromBool(snapshot.ready), .loading = @intFromBool(snapshot.loading), .status = if (snapshot.fault) |f| f.status else 0, .value = if (snapshot.value) |v| buffer(v) else .{}, .message = if (snapshot.fault) |f| buffer(@constCast(f.message)) else .{} };
    return 0;
}
fn cacheResponse(cache: *core.QueryCache, key: []const u8, ticket: Ticket, http_status: u16, bytes: []const u8, requested: []const []const u8) !Reply {
    var response = try core.receive(cache.allocator, http_status, bytes);
    defer response.deinit();
    var reply = try replyBuffers(cache.allocator, &response);
    errdefer {
        release(cache.allocator, &reply.value);
        release(cache.allocator, &reply.message);
    }
    try core.query_response.apply(cache, ticket.epoch, if (key.len > 0) key else null, if (ticket.serial > 0) .{ .epoch = ticket.epoch, .serial = ticket.serial } else null, &response, requested);
    return reply;
}
pub export fn sk_cache_receive(cache: *core.QueryCache, key: Bytes, ticket: Ticket, http_status: u16, bytes: Bytes, requested: Bytes, out: *Reply) u32 {
    out.* = .{};
    const parsed = std.json.parseFromSlice([][]const u8, a, input(requested) catch |e| return status(e), .{}) catch |e| return status(e);
    defer parsed.deinit();
    out.* = cacheResponse(cache, input(key) catch |e| return status(e), ticket, http_status, input(bytes) catch |e| return status(e), parsed.value) catch |e| return status(e);
    return 0;
}
fn allocationCase(allocator: std.mem.Allocator) !void {
    var request = try prepareBuffers(allocator, "http://127.0.0.1:8080", "", "hash/call", 1, "{\"text\":\"copied input\"}", &.{});
    defer {
        release(allocator, &request.url);
        release(allocator, &request.origin);
        release(allocator, &request.body);
        release(allocator, &request.key);
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

fn cacheAllocationCase(allocator: std.mem.Allocator) !void {
    var cache = try core.QueryCache.init(allocator, 2);
    defer cache.deinit();
    try cache.retain("hash/a/");
    const ticket = (try cache.begin("hash/a/", false)).?;
    var reply = try cacheResponse(&cache, "hash/a/", .{ .epoch = ticket.epoch, .serial = ticket.serial }, 200,
        \\{"type":"result","data":"[{\"_\":1,\"q\":2},{\"text\":3},{\"hash/a/\":4,\"hash/b/\":5},\"owned cache result\",{\"v\":1},{\"v\":6},2]"}
    , &.{});
    defer {
        release(allocator, &reply.value);
        release(allocator, &reply.message);
    }
    var snapshot = try cache.snapshot("hash/a/");
    defer snapshot.deinit();
    try cache.reset();
    try std.testing.expectEqualStrings("{\"text\":\"owned cache result\"}", snapshot.value.?);
    try std.testing.expectEqualStrings("{\"text\":\"owned cache result\"}", reply.value.ptr.?[0..reply.value.len]);
}
test "cache response buffers and snapshots survive reset and release every allocation failure" {
    var no_growth = std.testing.FailingAllocator.init(std.testing.allocator, .{ .resize_fail_index = 0 });
    try std.testing.checkAllAllocationFailures(no_growth.allocator(), cacheAllocationCase, .{});
}
test "release zeroes buffers and result bytes outlive parsing" {
    var response = try decodeBuffers(std.testing.allocator, 200, "{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},\\\"owned\\\"]\"}");
    try std.testing.expectEqualStrings("\"owned\"", response.value.ptr.?[0..response.value.len]);
    release(std.testing.allocator, &response.value);
    try std.testing.expect(response.value.ptr == null and response.value.len == 0);
    release(std.testing.allocator, &response.value);
}
