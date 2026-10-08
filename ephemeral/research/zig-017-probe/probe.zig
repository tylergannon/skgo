const std = @import("std");
const Snapshot = struct {
    allocator: std.mem.Allocator,
    arena: std.heap.ArenaAllocator,
    bytes: []const u8,
    refs: usize,

    fn create(allocator: std.mem.Allocator, input: []const u8) !*Snapshot {
        const s = try allocator.create(Snapshot);
        errdefer allocator.destroy(s);
        s.* = .{ .allocator = allocator, .arena = .init(allocator), .bytes = &.{}, .refs = 1 };
        errdefer s.arena.deinit();
        s.bytes = try s.arena.allocator().dupe(u8, input);
        return s;
    }
};
const Bytes = extern struct { ptr: ?[*]const u8, len: usize };

export fn snapshot_new(ptr: [*]const u8, len: usize) ?*Snapshot {
    return Snapshot.create(std.heap.page_allocator, ptr[0..len]) catch null;
}
export fn snapshot_retain(s: *Snapshot) void { s.refs += 1; }
export fn snapshot_release(s: *Snapshot) void {
    s.refs -= 1;
    if (s.refs == 0) {
        const allocator = s.allocator;
        s.arena.deinit();
        allocator.destroy(s);
    }
}
export fn snapshot_bytes(s: *const Snapshot) Bytes {
    return .{ .ptr = if (s.bytes.len == 0) null else s.bytes.ptr, .len = s.bytes.len };
}
fn allocationCase(allocator: std.mem.Allocator) !void {
    const s = try Snapshot.create(allocator, "fixture");
    snapshot_retain(s);
    snapshot_release(s);
    try std.testing.expectEqualStrings("fixture", s.bytes);
    snapshot_release(s);
}
test "snapshot retention and every allocation failure" {
    try std.testing.checkAllAllocationFailures(std.testing.allocator, allocationCase, .{});
}
fn sleeper(io: std.Io) std.Io.Cancelable!void {
    try io.sleep(.fromSeconds(60), .awake);
}
test "Threaded Io cancellation joins task" {
    var threaded: std.Io.Threaded = .init(std.testing.allocator, .{});
    defer threaded.deinit();
    const io = threaded.io();
    var task = try io.concurrent(sleeper, .{io});
    try std.testing.expectError(error.Canceled, task.cancel(io));
}
