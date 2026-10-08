const std = @import("std");
const Cache = @import("cache.zig").Cache;
const a = std.testing.allocator;
const key = "hash/query/";

fn state(cache: *Cache, ready: bool, loading: bool, value: ?[]const u8, status: ?u16) !void {
    var snapshot = try cache.snapshot(key);
    defer snapshot.deinit();
    try std.testing.expectEqual(ready, snapshot.ready);
    try std.testing.expectEqual(loading, snapshot.loading);
    if (value) |v| try std.testing.expectEqualStrings(v, snapshot.value.?) else try std.testing.expect(snapshot.value == null);
    try std.testing.expectEqual(status, if (snapshot.fault) |f| f.status else null);
}
test "retained callers share one initial query, explicit refresh starts another" {
    var cache = try Cache.init(a, 2);
    defer cache.deinit();
    try cache.retain(key);
    try cache.retain(key);
    var starts: usize = 0;
    const first = (try cache.begin(key, false)).?;
    starts += 1;
    if (try cache.begin(key, false) != null) starts += 1;
    try std.testing.expectEqual(@as(usize, 1), starts);
    try state(&cache, false, true, null, null);
    try std.testing.expect(try cache.resolve(key, first, "1"));
    try state(&cache, true, false, "1", null);
    try std.testing.expect(try cache.begin(key, false) == null);
    const refresh = (try cache.begin(key, true)).?;
    starts += 1;
    try std.testing.expectEqual(@as(usize, 2), starts);
    try state(&cache, true, true, "1", null);
    try std.testing.expect(try cache.resolve(key, refresh, "2"));
    try cache.release(key);
    try state(&cache, true, false, "2", null);
    try cache.release(key);
    try std.testing.expectEqual(@as(usize, 0), cache.entries.count());
    try std.testing.expectError(error.NotRetained, cache.release(key));
}
test "refresh promises settle predecessors and ignore superseded completions" {
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain(key);
    const first = (try cache.begin(key, false)).?;
    const second = (try cache.begin(key, true)).?;
    try std.testing.expect(try cache.resolve(key, second, "2"));
    try std.testing.expect(!try cache.resolve(key, first, "1"));
    try state(&cache, true, false, "2", null);
    const third = (try cache.begin(key, true)).?;
    const fourth = (try cache.begin(key, true)).?;
    try std.testing.expect(try cache.resolve(key, third, "3"));
    try state(&cache, true, false, "3", null);
    try std.testing.expect(try cache.resolve(key, fourth, "4"));
    try state(&cache, true, false, "4", null);
}
test "query errors keep a previous value, and an explicit set supersedes pending refreshes" {
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain(key);
    try std.testing.expect(try cache.set(cache.epoch, key, "1"));
    const failed = (try cache.begin(key, true)).?;
    try std.testing.expect(try cache.reject(key, failed, .{ .status = 503, .message = "Temporarily unavailable" }));
    try state(&cache, true, false, "1", 503);
    const pending = (try cache.begin(key, true)).?;
    try std.testing.expect(try cache.set(cache.epoch, key, "2"));
    try std.testing.expect(!try cache.resolve(key, pending, "0"));
    try state(&cache, true, false, "2", null);
    try std.testing.expect(try cache.fail(cache.epoch, key, .{ .status = 400, .message = "Requested update was not handled by the remote function" }));
    try state(&cache, true, false, "2", 400);
}
test "prefetch retention is bounded, active resources cannot be evicted, missing errors are dropped" {
    var cache = try Cache.init(a, 2);
    defer cache.deinit();
    try cache.retain(key);
    try std.testing.expect(try cache.set(cache.epoch, key, "1"));
    for ([_][]const u8{ "hash/a/", "hash/b/", "hash/c/" }) |k| {
        try std.testing.expect(try cache.set(cache.epoch, k, "2"));
        try std.testing.expectEqual(@as(usize, 2), cache.entries.count());
        try state(&cache, true, false, "1", null);
    }
    try cache.retain("hash/c/");
    try std.testing.expectError(error.CacheFull, cache.retain("hash/d/"));
    try std.testing.expect(!try cache.set(cache.epoch, "hash/d/", "3"));
    try std.testing.expect(!try cache.fail(cache.epoch, "hash/missing/", .{ .status = 500, .message = "Dropped error" }));
    try std.testing.expectEqual(@as(usize, 2), cache.entries.count());
}
test "owned snapshots survive eviction and old session completions cannot populate the new cache" {
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain(key);
    const old = (try cache.begin(key, false)).?;
    try std.testing.expect(try cache.resolve(key, old, "{\"text\":\"owned transcript\"}"));
    var snapshot = try cache.snapshot(key);
    defer snapshot.deinit();
    const pending = (try cache.begin(key, true)).?;
    const epoch = cache.epoch;
    try cache.reset();
    try cache.retain(key);
    const current = (try cache.begin(key, false)).?;
    try std.testing.expect(!try cache.resolve(key, pending, "0"));
    try std.testing.expect(!try cache.set(epoch, key, "0"));
    try std.testing.expect(!try cache.fail(epoch, key, .{ .status = 500, .message = "Old session" }));
    try state(&cache, false, true, null, null);
    try std.testing.expect(try cache.resolve(key, current, "1"));
    try cache.release(key);
    try std.testing.expectEqual(@as(usize, 0), cache.entries.count());
    try std.testing.expectEqualStrings("{\"text\":\"owned transcript\"}", snapshot.value.?);
}
fn allocationCase(allocator: std.mem.Allocator) !void {
    var cache = try Cache.init(allocator, 2);
    defer cache.deinit();
    try cache.retain(key);
    const ticket = (try cache.begin(key, false)).?;
    _ = try cache.resolve(key, ticket, "{\"value\":1}");
    const refresh = (try cache.begin(key, true)).?;
    _ = try cache.reject(key, refresh, .{ .status = 503, .message = "Unavailable" });
    _ = try cache.fail(cache.epoch, key, .{ .status = 409, .message = "Revision conflict" });
    var snapshot = try cache.snapshot(key);
    defer snapshot.deinit();
    _ = try cache.applyUpdates(cache.epoch, &.{.{ .value = .{ .key = key, .bytes = "2" } }}, &.{key}, &.{});
    _ = try cache.set(cache.epoch, "hash/prefetch/", "null");
    _ = try cache.set(cache.epoch, "hash/next/", "2");
    try cache.release(key);
    try cache.reset();
}
test "cache allocation failures release keys, snapshots, pending tickets and result buffers" {
    try std.testing.checkAllAllocationFailures(a, allocationCase, .{});
}

test "command updates fulfill query requests, explicit ignores preserve values, unhandled keys fail" {
    var cache = try Cache.init(a, 4);
    defer cache.deinit();
    const requested = [_][]const u8{ key, "hash/ignored/", "hash/unhandled/" };
    for (requested) |k| {
        try cache.retain(k);
        _ = try cache.set(cache.epoch, k, "1");
    }
    _ = (try cache.begin(key, true)).?;
    const updates = [_]Cache.Update{ .{ .value = .{ .key = key, .bytes = "2" } }, .{ .failure = .{ .key = "hash/missing/", .fault = .{ .status = 500, .message = "Drop me" } } } };
    try std.testing.expect(try cache.applyUpdates(cache.epoch, &updates, &requested, &.{"hash/ignored/"}));
    try state(&cache, true, false, "2", null);
    var ignored = try cache.snapshot("hash/ignored/");
    defer ignored.deinit();
    try std.testing.expectEqualStrings("1", ignored.value.?);
    try std.testing.expect(ignored.fault == null);
    var unhandled = try cache.snapshot("hash/unhandled/");
    defer unhandled.deinit();
    try std.testing.expectEqualStrings("1", unhandled.value.?);
    try std.testing.expectEqual(@as(u16, 400), unhandled.fault.?.status);
    try std.testing.expectEqualStrings("Requested update was not handled by the remote function", unhandled.fault.?.message);
    try std.testing.expectEqual(@as(usize, 3), cache.entries.count());
}

test "ready undefined and JSON null remain distinct cached results" {
    var cache = try Cache.init(a, 1);
    defer cache.deinit();
    try cache.retain(key);
    _ = try cache.set(cache.epoch, key, null);
    try state(&cache, true, false, null, null);
    _ = try cache.set(cache.epoch, key, "null");
    try state(&cache, true, false, "null", null);
}
