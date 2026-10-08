const std = @import("std");
const core = @import("root.zig");
const d = core.devalue;
const a = std.testing.allocator;

// Recorded by evaluating the argument functions from the installed Kit 3.0.0
// runtime/shared.js with its own resolved devalue 5.9.4; empty transport hooks.
// Inputs are upstream devalue documents, not output produced by the Zig codec.
const Case = struct { name: []const u8, input: []const u8, query: []const u8, command: []const u8 };
const Corpus = struct { kit: []const u8, devalue: []const u8, cases: []Case };
test "pinned Kit query and command argument bytes" {
    const fixtures = try std.json.parseFromSlice(Corpus, a, @embedFile("kit-arguments.json"), .{});
    defer fixtures.deinit();
    try std.testing.expectEqualStrings("3.0.0", fixtures.value.kit);
    try std.testing.expectEqualStrings(d.upstream_version, fixtures.value.devalue);
    try std.testing.expectEqual(@as(usize, 6), fixtures.value.cases.len);
    for (fixtures.value.cases) |case| for ([_]core.Kind{ .query, .command }) |kind| {
        var arg = try d.parse(a, case.input, &.{});
        defer arg.deinit();
        var request = try core.prepare(a, .{ .origin = "http://127.0.0.1:8080", .base = "/app", .app_dir = "assets" }, "hash/call", kind, &arg.graph, arg.value, &.{});
        defer request.deinit();
        try std.testing.expectEqualStrings(if (kind == .query) case.query else case.command, request.payload);
        if (kind == .query) {
            const expected = if (case.query.len == 0) try a.dupe(u8, "http://127.0.0.1:8080/app/assets/remote/hash/call") else try std.fmt.allocPrint(a, "http://127.0.0.1:8080/app/assets/remote/hash/call?payload={s}", .{case.query});
            defer a.free(expected);
            try std.testing.expectEqualStrings(expected, request.url);
            try std.testing.expect(request.body == null);
        } else {
            try std.testing.expectEqualStrings("http://127.0.0.1:8080/app/assets/remote/hash/call", request.url);
            const body = try std.json.parseFromSlice(struct { payload: []const u8, refreshes: []const []const u8 }, a, request.body.?, .{});
            defer body.deinit();
            try std.testing.expectEqualStrings(case.command, body.value.payload);
            try std.testing.expectEqual(@as(usize, 0), body.value.refreshes.len);
        }
    };
}

test "remote result retains input after response bytes are released" {
    const bytes = try a.dupe(u8, "{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},{\\\"text\\\":2,\\\"revision\\\":3},\\\"native transcript\\\",1]\"}");
    var response = try core.receive(a, 200, bytes);
    a.free(bytes);
    defer response.deinit();
    try std.testing.expectEqual(.result, response.kind);
    try std.testing.expectEqualStrings("native transcript", (try response.parsed.?.graph.get(response.value, "text")).?.string);
    try std.testing.expectEqual(@as(f64, 1), (try response.parsed.?.graph.get(response.value, "revision")).?.number);
}

test "HTTP failures and HTTP-200 remote errors remain distinct" {
    var plain = try core.receive(a, 502, "Bad gateway");
    defer plain.deinit();
    try std.testing.expectEqual(.http_error, plain.kind);
    try std.testing.expectEqual(@as(?u16, 502), plain.status);
    var denied = try core.receive(a, 403, "{\"type\":\"error\",\"error\":{\"message\":\"Forbidden\"}}");
    defer denied.deinit();
    try std.testing.expectEqual(.http_error, denied.kind);
    try std.testing.expectEqual(@as(?u16, 403), denied.status);
    var remote = try core.receive(a, 200, "{\"type\":\"error\",\"error\":{\"status\":409,\"message\":\"Revision conflict\"}}");
    defer remote.deinit();
    try std.testing.expectEqual(.remote_error, remote.kind);
    try std.testing.expectEqual(@as(?u16, 409), remote.status);
    try std.testing.expectEqualStrings("Revision conflict", remote.message.?);
    try std.testing.expectError(error.InvalidResponse, core.receive(a, 200, "{}"));
    try std.testing.expectError(error.InvalidResponse, core.receive(a, 200, "{\"type\":\"result\",\"data\":123}"));
}

fn allocationCase(allocator: std.mem.Allocator) !void {
    var graph = d.Graph.init(allocator);
    defer graph.deinit();
    const arg = try graph.object(false);
    try graph.put(arg, "text", try graph.string("voice <text>"));
    var request = try core.prepare(allocator, .{ .origin = "https://example.test" }, "hash/call", .query, &graph, arg, &.{});
    defer request.deinit();
    var command = try core.prepare(allocator, .{ .origin = "https://example.test" }, "hash/call", .command, &graph, arg, &.{"hash/query/"});
    defer command.deinit();
    var response = try core.receive(allocator, 200, "{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},\\\"acknowledged\\\"]\"}");
    defer response.deinit();
}
test "allocation failures release request and response ownership" {
    // Refuse resize/remap during exhaustive allocation failure injection so
    // SafeAllocator's address-dependent growth cannot change allocation counts.
    var no_growth = std.testing.FailingAllocator.init(std.testing.allocator, .{ .resize_fail_index = 0 });
    try std.testing.checkAllAllocationFailures(no_growth.allocator(), allocationCase, .{});
}

test "unsupported argument nodes and invalid endpoint paths fail explicitly" {
    var graph = d.Graph.init(a);
    defer graph.deinit();
    const regex = try graph.regexp("a", "");
    for ([_]core.Kind{ .query, .command }) |kind| try std.testing.expectError(error.UnsupportedValue, core.prepare(a, .{ .origin = "https://example.test" }, "hash/call", kind, &graph, regex, &.{}));
    try std.testing.expectError(error.InvalidEndpoint, core.prepare(a, .{ .origin = "https://example.test/path" }, "hash/call", .query, &graph, .null, &.{}));
    try std.testing.expectError(error.InvalidEndpoint, core.prepare(a, .{ .origin = "https://example.test", .base = "/../secret" }, "hash/call", .query, &graph, .null, &.{}));
}

// Kit sends the location inside devalue data; it is not an HTTP redirect.
test "query redirect is surfaced as a location rather than an empty result" {
    var response = try core.receive(a, 200, "{\"type\":\"result\",\"data\":\"[{\\\"redirect\\\":1},\\\"/sign-in\\\"]\"}");
    defer response.deinit();
    try std.testing.expectEqual(.redirect, response.kind);
    try std.testing.expectEqualStrings("/sign-in", response.message.?);
}

test "canonical argument keys coalesce reordered objects into one retained query" {
    const expected = "hash/page/W1siX19za3JhbyIsMV0seyJsaW1pdCI6Miwib2Zmc2V0IjozfSwxMCwyMF0";
    var cache = try core.QueryCache.init(a, 1);
    defer cache.deinit();
    var starts: usize = 0;
    for ([_][]const u8{ "[{\"offset\":1,\"limit\":2},20,10]", "[{\"limit\":1,\"offset\":2},10,20]" }) |input| {
        var arg = try d.parse(a, input, &.{});
        defer arg.deinit();
        var request = try core.prepare(a, .{ .origin = "https://example.test" }, "hash/page", .query, &arg.graph, arg.value, &.{});
        defer request.deinit();
        const key = try request.queryKey("hash/page");
        defer a.free(key);
        try std.testing.expectEqualStrings(expected, key);
        try cache.retain(key);
        if (try cache.begin(key, false) != null) starts += 1;
    }
    try std.testing.expectEqual(@as(usize, 1), starts);
    try cache.release(expected);
    try cache.release(expected);
}
