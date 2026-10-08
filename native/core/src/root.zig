const std = @import("std");
pub const devalue = @import("devalue");
const d = devalue;
const Allocator = std.mem.Allocator;
pub const QueryCache = @import("cache.zig").Cache;
pub const Kind = enum { query, command };

/// All strings in a prepared request are owned. Release once with deinit.
/// Preparing a query may append canonicalization nodes to the argument graph;
/// both graph reads and preparation require exclusive access.
pub const Request = struct {
    allocator: Allocator,
    kind: Kind,
    url: []u8,
    origin: []u8,
    payload: []u8,
    body: ?[]u8,
    /// Kit runtime/shared.js create_remote_key: function id + slash + payload.
    pub fn queryKey(self: *const Request, id: []const u8) ![]u8 {
        if (self.kind != .query) return error.InvalidInput;
        return std.fmt.allocPrint(self.allocator, "{s}/{s}", .{ id, self.payload });
    }
    pub fn deinit(self: *Request) void {
        self.allocator.free(self.url);
        self.allocator.free(self.origin);
        self.allocator.free(self.payload);
        if (self.body) |body| self.allocator.free(body);
        self.* = undefined;
    }
};

pub const Endpoint = struct {
    /// Scheme and authority only; base is configured separately.
    origin: []const u8,
    base: []const u8 = "",
    app_dir: []const u8 = "_app",
};

/// Native arguments currently admit primitives, plain/null-prototype objects
/// and arrays. Other graph nodes fail explicitly. Application transport hooks,
/// files and query Map/Set canonicalization are outside this finite-model path.
pub fn prepare(a: Allocator, endpoint: Endpoint, id: []const u8, kind: Kind, graph: *d.Graph, value: d.Value, refreshes: []const []const u8) !Request {
    const uri = std.Uri.parse(endpoint.origin) catch return error.InvalidEndpoint;
    if ((!std.mem.eql(u8, uri.scheme, "http") and !std.mem.eql(u8, uri.scheme, "https")) or uri.host == null or uri.user != null or uri.password != null or uri.query != null or uri.fragment != null or uri.path.percent_encoded.len != 0) return error.InvalidEndpoint;
    if ((endpoint.base.len != 0 and (endpoint.base[0] != '/' or endpoint.base[endpoint.base.len - 1] == '/')) or !validPath(endpoint.base, true) or !validPath(endpoint.app_dir, false) or !validPath(id, true)) return error.InvalidEndpoint;
    var canonical: Canonical = .{ .allocator = a };
    defer canonical.deinit();
    const reducers: []const d.Reducer = if (kind == .query) &.{.{ .name = "__skrao", .context = &canonical, .reduce = Canonical.reduce }} else &.{.{ .name = "__skrag", .reduce = guard }};
    const wire = if (value == .undefined) try a.dupe(u8, "") else try d.stringify(a, graph, value, reducers);
    defer a.free(wire);
    const encoder = std.base64.url_safe_no_pad.Encoder;
    const payload = try a.alloc(u8, encoder.calcSize(wire.len));
    errdefer a.free(payload);
    _ = encoder.encode(payload, wire);
    const url = if (kind == .query and payload.len > 0)
        try std.fmt.allocPrint(a, "{s}{s}/{s}/remote/{s}?payload={s}", .{ endpoint.origin, endpoint.base, endpoint.app_dir, id, payload })
    else
        try std.fmt.allocPrint(a, "{s}{s}/{s}/remote/{s}", .{ endpoint.origin, endpoint.base, endpoint.app_dir, id });
    errdefer a.free(url);
    const origin = try a.dupe(u8, endpoint.origin);
    errdefer a.free(origin);
    const body = if (kind == .command) try std.json.Stringify.valueAlloc(a, .{ .payload = payload, .refreshes = refreshes }, .{}) else null;
    return .{ .allocator = a, .kind = kind, .url = url, .origin = origin, .payload = payload, .body = body };
}

fn validPath(path: []const u8, slash: bool) bool {
    if (path.len == 0) return slash;
    for (path) |c| if (!std.ascii.isAlphanumeric(c) and c != '_' and c != '-' and c != '.' and !(slash and c == '/')) return false;
    var parts = std.mem.splitScalar(u8, path, '/');
    while (parts.next()) |part| if (std.mem.eql(u8, part, ".") or std.mem.eql(u8, part, "..")) return false;
    return true;
}

fn guard(_: ?*anyopaque, graph: *d.Graph, value: d.Value) d.Error!?d.Value {
    if (value == .ref) switch (try graph.node(value)) {
        .object, .array => {},
        else => return error.UnsupportedValue,
    };
    return null;
}

const Canonical = struct {
    allocator: Allocator,
    clones: std.AutoHashMapUnmanaged(d.Handle, d.Value) = .empty,
    marked: std.AutoHashMapUnmanaged(d.Handle, void) = .empty,
    fn deinit(self: *@This()) void {
        self.clones.deinit(self.allocator);
        self.marked.deinit(self.allocator);
    }
    fn reduce(ctx: ?*anyopaque, graph: *d.Graph, value: d.Value) d.Error!?d.Value {
        const self: *@This() = @ptrCast(@alignCast(ctx.?));
        _ = try guard(null, graph, value);
        if (value != .ref or self.marked.contains(value.ref)) return null;
        const node = try graph.node(value);
        if (node != .object) return null;
        if (self.clones.get(value.ref)) |clone| return clone;
        const props = try self.allocator.dupe(d.Property, node.object.properties.items);
        defer self.allocator.free(props);
        std.mem.sort(d.Property, props, {}, propertyLess);
        const clone = try graph.object(node.object.null_proto);
        try self.clones.put(self.allocator, value.ref, clone);
        try self.marked.put(self.allocator, clone.ref, {});
        for (props) |prop| {
            const child = if (prop.value == .ref) self.clones.get(prop.value.ref) orelse prop.value else prop.value;
            try graph.put(clone, prop.key, child);
        }
        return clone;
    }
};

// JavaScript .sort() compares UTF-16 units, not UTF-8 bytes/code points.
const Units = struct {
    it: std.unicode.Utf8Iterator,
    low: ?u16 = null,
    fn init(bytes: []const u8) @This() {
        return .{ .it = .{ .bytes = bytes, .i = 0 } };
    }
    fn next(self: *@This()) ?u16 {
        if (self.low) |low| {
            self.low = null;
            return low;
        }
        const cp = self.it.nextCodepoint() orelse return null;
        if (cp <= 0xffff) return @intCast(cp);
        self.low = @intCast(0xdc00 + ((cp - 0x10000) & 0x3ff));
        return @intCast(0xd800 + ((cp - 0x10000) >> 10));
    }
};
fn propertyLess(_: void, lhs: d.Property, rhs: d.Property) bool {
    var left = Units.init(lhs.key);
    var right = Units.init(rhs.key);
    while (left.next()) |l| {
        const r = right.next() orelse return false;
        if (l != r) return l < r;
    }
    return right.next() != null;
}

/// Response graph owns all retained bytes, including remote error messages.
/// Never copy a live Response. value/error/redirect borrow its graph.
pub const Response = struct {
    parsed: ?d.ParseResult = null,
    envelope: ?std.json.Parsed(std.json.Value) = null,
    http_status: u16,
    kind: enum { result, http_error, remote_error, redirect },
    value: d.Value = .undefined,
    message: ?[]const u8 = null,
    status: ?u16 = null,
    pub fn deinit(self: *Response) void {
        if (self.parsed) |*parsed| parsed.deinit();
        if (self.envelope) |*envelope| envelope.deinit();
        self.* = undefined;
    }
};

pub fn receive(a: Allocator, http_status: u16, bytes: []const u8) !Response {
    const success = http_status >= 200 and http_status < 300;
    var envelope = std.json.parseFromSlice(std.json.Value, a, bytes, .{ .allocate = .alloc_always }) catch |err| {
        if (err == error.OutOfMemory) return err;
        if (!success) return .{ .http_status = http_status, .kind = .http_error, .status = http_status };
        return error.InvalidResponse;
    };
    errdefer envelope.deinit();
    if (!success) {
        var result: Response = .{ .envelope = envelope, .http_status = http_status, .kind = .http_error, .status = http_status };
        if (envelope.value == .object) if (envelope.value.object.get("error")) |e| {
            if (e == .object) if (e.object.get("message")) |m| {
                if (m == .string) result.message = m.string;
            };
        };
        return result;
    }
    const obj = if (envelope.value == .object) envelope.value.object else return error.InvalidResponse;
    const tag = obj.get("type") orelse return error.InvalidResponse;
    if (tag != .string) return error.InvalidResponse;
    if (!success or std.mem.eql(u8, tag.string, "error")) {
        var result: Response = .{ .envelope = envelope, .http_status = http_status, .kind = if (success) .remote_error else .http_error, .status = if (success) null else http_status };
        if (obj.get("error")) |e| {
            if (e != .object) return error.InvalidResponse;
            if (e.object.get("message")) |message| {
                if (message != .string) return error.InvalidResponse;
                result.message = message.string;
            }
            if (success) if (e.object.get("status")) |status| {
                if (status != .integer or status.integer < 400 or status.integer > 599) return error.InvalidResponse;
                result.status = @intCast(status.integer);
            };
        }
        return result;
    }
    if (!std.mem.eql(u8, tag.string, "result")) return error.InvalidResponse;
    const data = obj.get("data") orelse return error.InvalidResponse;
    if (data != .string) return error.InvalidResponse;
    var parsed = try d.parse(a, data.string, &.{});
    errdefer parsed.deinit();
    const value = try parsed.graph.get(parsed.value, "_") orelse .undefined;
    const redirect = try parsed.graph.get(parsed.value, "redirect");
    if (redirect) |r| {
        if (r != .string) return error.InvalidResponse;
        return .{ .envelope = envelope, .parsed = parsed, .http_status = http_status, .kind = .redirect, .message = r.string };
    }
    return .{ .envelope = envelope, .parsed = parsed, .http_status = http_status, .kind = .result, .value = value };
}

test {
    _ = @import("tests.zig");
    _ = QueryCache;
}
