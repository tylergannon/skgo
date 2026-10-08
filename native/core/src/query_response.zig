const std = @import("std");
const core = @import("root.zig");
const Cache = core.QueryCache;
const d = core.devalue;

/// Apply the ordinary-query subset of Kit 3.0.0 shared.svelte.js. The response
/// graph is exclusively confined to this call; the cache retains model bytes.
pub fn apply(cache: *Cache, epoch: u64, query_key: ?[]const u8, ticket: ?Cache.Ticket, response: *core.Response, requested: []const []const u8) !void {
    if (epoch != cache.epoch) return;
    if (response.kind == .http_error or response.kind == .remote_error) {
        if (query_key) |key| if (ticket) |t| {
            _ = try cache.reject(key, t, .{ .status = response.status orelse response.http_status, .message = response.message orelse "" });
        };
        return; // remote_request throws before fail_unhandled_refreshes
    }
    const graph = &response.parsed.?.graph;
    var arena = std.heap.ArenaAllocator.init(cache.allocator);
    defer arena.deinit();
    const a = arena.allocator();
    var updates: std.ArrayList(Cache.Update) = .empty;
    var ignored: std.ArrayList([]const u8) = .empty;
    // Decode every node before publishing any of them. Unsupported data never
    // silently becomes a different value, or a partly decoded successful call.
    if (try graph.get(response.parsed.?.value, "l")) |_| return error.UnsupportedValue;
    if (try graph.get(response.parsed.?.value, "q")) |q| {
        const node = try graph.node(q);
        if (node != .object) return error.InvalidResponse;
        for (node.object.properties.items) |field| {
            try validKey(field.key);
            if (try graph.get(field.value, "e")) |e| {
                const fault = try decodeFault(graph, e);
                try updates.append(a, .{ .failure = .{ .key = field.key, .fault = fault } });
            } else {
                const value = try graph.get(field.value, "v") orelse return error.InvalidResponse;
                const bytes = if (value == .undefined) null else try core.model_json.encode(a, graph, value);
                try updates.append(a, .{ .value = .{ .key = field.key, .bytes = bytes } });
            }
        }
    }
    if (try graph.get(response.parsed.?.value, "i")) |i| {
        const node = try graph.node(i);
        if (node != .array) return error.InvalidResponse;
        for (0..node.array.length) |index| {
            const value = try graph.arrayGet(i, @intCast(index)) orelse return error.InvalidResponse;
            if (value != .string) return error.InvalidResponse;
            try validKey(value.string);
            try ignored.append(a, value.string);
        }
    }
    const value = if (response.value == .undefined) null else try core.model_json.encode(a, graph, response.value);
    // Commands reject redirects before fail_unhandled_refreshes; queries never
    // carry requested refreshes. remote_request has already applied q updates.
    _ = try cache.applyUpdates(epoch, updates.items, if (response.kind == .redirect) &.{} else requested, ignored.items);
    if (query_key) |key| if (ticket) |t| {
        if (response.kind == .redirect) {
            _ = try cache.reject(key, t, .{ .status = 307, .message = response.message orelse "" });
        } else {
            // A q node for this query already settled its promises via set/fail.
            // Otherwise settle the native call's direct result.
            _ = try cache.resolve(key, t, value);
        }
    };
}
fn validKey(key: []const u8) !void {
    if (std.mem.lastIndexOfScalar(u8, key, '/') == null) return error.InvalidResponse;
}
fn decodeFault(graph: *d.Graph, value: d.Value) !Cache.Fault {
    const status = try graph.get(value, "status") orelse return error.InvalidResponse;
    const message = try graph.get(value, "message") orelse return error.InvalidResponse;
    if (status != .number or status.number < 400 or status.number > 599 or @floor(status.number) != status.number or message != .string) return error.InvalidResponse;
    return .{ .status = @intFromFloat(status.number), .message = message.string };
}

fn fixture(a: std.mem.Allocator, data: []const u8) !core.Response {
    const body = try std.json.Stringify.valueAlloc(a, .{ .type = "result", .data = data }, .{});
    defer a.free(body);
    return core.receive(a, 200, body);
}
fn expectState(cache: *Cache, key: []const u8, value: ?[]const u8, status: ?u16) !void {
    var snapshot = try cache.snapshot(key);
    defer snapshot.deinit();
    if (value) |v| try std.testing.expectEqualStrings(v, snapshot.value.?) else try std.testing.expect(snapshot.value == null);
    try std.testing.expectEqual(status, if (snapshot.fault) |f| f.status else null);
}
test "literal Kit q and i envelopes copy updates and fail unhandled keys" {
    const a = std.testing.allocator;
    var cache = try Cache.init(a, 4);
    defer cache.deinit();
    for ([_][]const u8{ "hash/a/", "hash/b/", "hash/c/" }) |key| {
        try cache.retain(key);
        _ = try cache.set(cache.epoch, key, "1");
    }
    var response = try fixture(a,
        \\[{"_":1,"q":2,"i":7},"ack",{"hash/a/":3,"hash/missing/":5},{"v":4},2,{"e":6},{"status":8,"message":9},[10],503,"Dropped error","hash/b/"]
    );
    defer response.deinit();
    try apply(&cache, cache.epoch, null, null, &response, &.{ "hash/a/", "hash/b/", "hash/c/" });
    try expectState(&cache, "hash/a/", "2", null);
    try expectState(&cache, "hash/b/", "1", null);
    try expectState(&cache, "hash/c/", "1", 400);
    try std.testing.expectEqual(@as(usize, 3), cache.entries.count());
}
test "q self update settles every refresh and is not suppressed by promise ordering" {
    const a = std.testing.allocator;
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain("hash/a/");
    const first = (try cache.begin("hash/a/", false)).?;
    const second = (try cache.begin("hash/a/", true)).?;
    var newer = try fixture(a,
        \\[{"_":1,"q":2},2,{"hash/a/":3},{"v":1}]
    );
    defer newer.deinit();
    try apply(&cache, cache.epoch, "hash/a/", second, &newer, &.{});
    try std.testing.expect(!cache.waiting("hash/a/", first));
    try std.testing.expect(!cache.waiting("hash/a/", second));
    var older = try fixture(a,
        \\[{"_":1,"q":2},1,{"hash/a/":3},{"v":1}]
    );
    defer older.deinit();
    try apply(&cache, cache.epoch, "hash/a/", first, &older, &.{});
    try expectState(&cache, "hash/a/", "1", null);
}
test "HTTP errors do not fail command refreshes and old epochs cannot update a new session" {
    const a = std.testing.allocator;
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain("hash/a/");
    _ = try cache.set(cache.epoch, "hash/a/", "1");
    var failed = try core.receive(a, 503, "Unavailable");
    defer failed.deinit();
    try apply(&cache, cache.epoch, null, null, &failed, &.{"hash/a/"});
    try expectState(&cache, "hash/a/", "1", null);
    const old = cache.epoch;
    try cache.reset();
    try cache.retain("hash/a/");
    var stale = try fixture(a,
        \\[{"q":1},{"hash/a/":2},{"v":3},2]
    );
    defer stale.deinit();
    try apply(&cache, old, null, null, &stale, &.{});
    try expectState(&cache, "hash/a/", null, null);
}
test "unsupported later q nodes cannot publish earlier decoded values" {
    const a = std.testing.allocator;
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain("hash/a/");
    _ = try cache.set(cache.epoch, "hash/a/", "1");
    var unsupported = try fixture(a,
        \\[{"q":1},{"hash/a/":2,"hash/b/":4},{"v":3},2,{"v":5},["Date","2026-10-08T00:00:00.000Z"]]
    );
    defer unsupported.deinit();
    try std.testing.expectError(error.UnsupportedValue, apply(&cache, cache.epoch, null, null, &unsupported, &.{}));
    try expectState(&cache, "hash/a/", "1", null);
}
