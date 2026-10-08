const std = @import("std");
const Allocator = std.mem.Allocator;

/// Ordinary-query state from Kit 3.0.0 query/instance.svelte.js. Every method,
/// including snapshot, requires exclusive access (the Swift client actor).
/// Only serialized model bytes are retained; no value graph escapes this core.
/// Never copy a live Cache or Snapshot; release each owner once with deinit.
pub const Cache = struct {
    allocator: Allocator,
    capacity: usize,
    epoch: u64 = 1,
    serial: u64 = 0,
    entries: std.StringHashMapUnmanaged(Entry) = .empty,

    pub const Ticket = struct { epoch: u64, serial: u64 };
    pub const Fault = struct { status: u16, message: []const u8 };
    const Entry = struct {
        refs: u32 = 0,
        ready: bool = false,
        loading: bool = true,
        value: ?[]u8 = null,
        fault: ?Fault = null,
        pending: std.ArrayList(u64) = .empty,
        touched: u64 = 0,
        fn deinit(self: *Entry, a: Allocator) void {
            if (self.value) |v| a.free(v);
            if (self.fault) |f| a.free(f.message);
            self.pending.deinit(a);
            self.* = undefined;
        }
    };
    /// These bytes own independent copies and survive eviction/reset/deinit.
    pub const Snapshot = struct {
        allocator: Allocator,
        ready: bool,
        loading: bool,
        value: ?[]u8,
        fault: ?Fault,
        pub fn deinit(self: *Snapshot) void {
            if (self.value) |v| self.allocator.free(v);
            if (self.fault) |f| self.allocator.free(f.message);
            self.* = undefined;
        }
    };
    pub fn init(a: Allocator, capacity: usize) !Cache {
        if (capacity == 0) return error.InvalidCapacity;
        return .{ .allocator = a, .capacity = capacity };
    }
    pub fn deinit(self: *Cache) void {
        self.clear();
        self.entries.deinit(self.allocator);
        self.* = undefined;
    }
    fn clear(self: *Cache) void {
        var it = self.entries.iterator();
        while (it.next()) |item| {
            item.value_ptr.deinit(self.allocator);
            self.allocator.free(item.key_ptr.*);
        }
        self.entries.clearRetainingCapacity();
    }
    /// Old HTTP completions carry their issued epoch and cannot affect this one.
    pub fn reset(self: *Cache) !void {
        if (self.epoch == std.math.maxInt(u64)) return error.Exhausted;
        self.clear();
        self.epoch += 1;
    }
    fn remove(self: *Cache, key: []const u8) void {
        if (self.entries.fetchRemove(key)) |item| {
            var entry = item.value;
            entry.deinit(self.allocator);
            self.allocator.free(item.key);
        }
    }
    fn evictable(self: *Cache) ?[]const u8 {
        var victim: ?[]const u8 = null;
        var oldest: u64 = std.math.maxInt(u64);
        var it = self.entries.iterator();
        while (it.next()) |item| if (item.value_ptr.refs == 0 and (victim == null or item.value_ptr.touched < oldest)) {
            oldest = item.value_ptr.touched;
            victim = item.key_ptr.*;
        };
        return victim;
    }
    fn ensure(self: *Cache, key: []const u8, required: bool) !?*Entry {
        if (self.entries.getPtr(key)) |entry| return entry;
        const victim = if (self.entries.count() >= self.capacity) self.evictable() else null;
        if (self.entries.count() >= self.capacity and victim == null) {
            if (required) return error.CacheFull;
            return null; // bounded prefetch retention must not evict a live query
        }
        const owned_key = try self.allocator.dupe(u8, key);
        errdefer self.allocator.free(owned_key);
        try self.entries.ensureUnusedCapacity(self.allocator, 1);
        // ensureUnusedCapacity may move the map, so only use the owned victim key.
        if (victim) |v| self.remove(v);
        self.entries.putAssumeCapacity(owned_key, .{ .touched = self.serial });
        return self.entries.getPtr(owned_key).?;
    }
    pub fn retain(self: *Cache, key: []const u8) !void {
        const entry = (try self.ensure(key, true)).?;
        if (entry.refs == std.math.maxInt(u32)) return error.Exhausted;
        entry.refs += 1;
    }
    /// Explicit native ownership replaces Kit's proxy GC/manual_ref. A last
    /// release removes the resource; only unsolicited successful updates are
    /// retained, bounded by capacity, like Kit's query_responses fallback.
    pub fn release(self: *Cache, key: []const u8) !void {
        const entry = self.entries.getPtr(key) orelse return error.NotRetained;
        if (entry.refs == 0) return error.NotRetained;
        entry.refs -= 1;
        if (entry.refs == 0) self.remove(key);
    }
    pub fn snapshot(self: *Cache, key: []const u8) !Snapshot {
        const entry = self.entries.getPtr(key) orelse return error.NotRetained;
        const value = if (entry.value) |v| try self.allocator.dupe(u8, v) else null;
        errdefer if (value) |v| self.allocator.free(v);
        const fault = if (entry.fault) |f| Fault{ .status = f.status, .message = try self.allocator.dupe(u8, f.message) } else null;
        return .{ .allocator = self.allocator, .ready = entry.ready, .loading = entry.loading, .value = value, .fault = fault };
    }
    pub fn begin(self: *Cache, key: []const u8, refresh: bool) !?Ticket {
        const entry = self.entries.getPtr(key) orelse return error.NotRetained;
        if (entry.refs == 0) return error.NotRetained;
        if (!refresh and (entry.ready or entry.pending.items.len > 0 or entry.fault != null)) return null;
        if (self.serial == std.math.maxInt(u64)) return error.Exhausted;
        const serial = self.serial + 1;
        try entry.pending.append(self.allocator, serial);
        self.serial = serial;
        entry.loading = true;
        return .{ .epoch = self.epoch, .serial = serial };
    }
    fn pendingIndex(self: *Cache, entry: *Entry, ticket: Ticket) ?usize {
        if (ticket.epoch != self.epoch) return null;
        for (entry.pending.items, 0..) |serial, i| if (serial == ticket.serial) return i;
        return null;
    }
    fn settleThrough(entry: *Entry, index: usize) void {
        const remaining = entry.pending.items.len - index - 1;
        std.mem.copyForwards(u64, entry.pending.items[0..remaining], entry.pending.items[index + 1 ..]);
        entry.pending.items.len = remaining;
    }
    fn replaceValue(self: *Cache, entry: *Entry, value: ?[]u8) void {
        if (entry.value) |v| self.allocator.free(v);
        if (entry.fault) |f| self.allocator.free(f.message);
        entry.value = value;
        entry.fault = null;
        entry.ready = true;
        entry.loading = false;
        entry.touched = self.serial;
    }
    /// The query instance's own promise completion settles its predecessors.
    /// A later completion wins; a superseded predecessor is ignored. Side-channel
    /// q updates instead call set/fail, which settle every pending refresh.
    pub fn resolve(self: *Cache, key: []const u8, ticket: Ticket, bytes: ?[]const u8) !bool {
        const entry = self.entries.getPtr(key) orelse return false;
        const index = self.pendingIndex(entry, ticket) orelse return false;
        const value = if (bytes) |v| try self.allocator.dupe(u8, v) else null;
        settleThrough(entry, index);
        self.replaceValue(entry, value);
        return true;
    }
    pub fn reject(self: *Cache, key: []const u8, ticket: Ticket, fault: Fault) !bool {
        const entry = self.entries.getPtr(key) orelse return false;
        const index = self.pendingIndex(entry, ticket) orelse return false;
        const message = try self.allocator.dupe(u8, fault.message);
        settleThrough(entry, index);
        if (entry.fault) |f| self.allocator.free(f.message);
        entry.fault = .{ .status = fault.status, .message = message };
        entry.loading = false;
        return true; // Kit keeps the previous ready/value on failure.
    }
    pub const Update = union(enum) {
        value: struct { key: []const u8, bytes: ?[]const u8 },
        failure: struct { key: []const u8, fault: Fault },
        pub fn key(self: Update) []const u8 {
            return switch (self) {
                .value => |v| v.key,
                .failure => |f| f.key,
            };
        }
    };
    /// Kit remote-functions/shared.svelte.js remote_request followed by
    /// fail_unhandled_refreshes. This runs only for a successful remote result;
    /// top-level HTTP/remote failures leave the requested queries untouched.
    pub fn applyUpdates(self: *Cache, epoch: u64, updates: []const Update, requested: []const []const u8, ignored: []const []const u8) !bool {
        if (epoch != self.epoch) return false;
        for (updates) |update| switch (update) {
            .value => |v| {
                _ = try self.set(epoch, v.key, v.bytes);
            },
            .failure => |f| {
                _ = try self.fail(epoch, f.key, f.fault);
            },
        };
        for (requested) |key| {
            var handled = false;
            for (updates) |update| if (std.mem.eql(u8, key, update.key())) {
                handled = true;
                break;
            };
            for (ignored) |ignored_key| if (std.mem.eql(u8, key, ignored_key)) {
                handled = true;
                break;
            };
            if (!handled) _ = try self.fail(epoch, key, .{ .status = 400, .message = "Requested update was not handled by the remote function" });
        }
        return true;
    }
    /// Called for each successful data.q node. Absent resources keep a bounded
    /// prefetch value. Results are copied before the response graph is destroyed.
    pub fn set(self: *Cache, epoch: u64, key: []const u8, bytes: ?[]const u8) !bool {
        if (epoch != self.epoch) return false;
        const value = if (bytes) |v| try self.allocator.dupe(u8, v) else null;
        errdefer if (value) |v| self.allocator.free(v);
        const entry = try self.ensure(key, false) orelse {
            if (value) |v| self.allocator.free(v);
            return false;
        };
        entry.pending.clearRetainingCapacity();
        self.replaceValue(entry, value);
        return true;
    }
    /// Called for data.q errors and unhandled requested refreshes. Kit drops
    /// absent-resource errors rather than storing them for a future query.
    pub fn fail(self: *Cache, epoch: u64, key: []const u8, fault: Fault) !bool {
        if (epoch != self.epoch) return false;
        const entry = self.entries.getPtr(key) orelse return false;
        if (entry.refs == 0) return false;
        const message = try self.allocator.dupe(u8, fault.message);
        entry.pending.clearRetainingCapacity();
        if (entry.fault) |f| self.allocator.free(f.message);
        entry.fault = .{ .status = fault.status, .message = message };
        entry.loading = false;
        return true;
    }
};

test {
    _ = @import("cache_test.zig");
}
