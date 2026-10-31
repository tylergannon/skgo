const std = @import("std");
const d = @import("devalue");

// Finite-model JSON is the local buffer ABI format. Graphs remain private to
// Zig and are destroyed before returning any independently owned result bytes.
pub fn fromJSON(graph: *d.Graph, value: std.json.Value, depth: usize) !d.Value {
    if (depth >= 256) return error.DepthLimit;
    return switch (value) {
        .null => .null,
        .bool => |b| .{ .boolean = b },
        .integer => |n| blk: {
            if (n < -9007199254740991 or n > 9007199254740991) return error.UnsupportedValue;
            break :blk .{ .number = @floatFromInt(n) };
        },
        .float => |n| blk: {
            if (!std.math.isFinite(n)) return error.UnsupportedValue;
            break :blk .{ .number = n };
        },
        .number_string => return error.UnsupportedValue,
        .string => |s| try graph.string(s),
        .array => |array| blk: {
            if (array.items.len > std.math.maxInt(u32)) return error.UnsupportedValue;
            const result = try graph.array(@intCast(array.items.len));
            for (array.items, 0..) |child, i| try graph.arrayPut(result, @intCast(i), try fromJSON(graph, child, depth + 1));
            break :blk result;
        },
        .object => |object| blk: {
            const result = try graph.object(false);
            var it = object.iterator();
            while (it.next()) |field| try graph.put(result, field.key_ptr.*, try fromJSON(graph, field.value_ptr.*, depth + 1));
            break :blk result;
        },
    };
}
pub fn toJSON(allocator: std.mem.Allocator, graph: *d.Graph, value: d.Value, ancestors: *std.ArrayList(d.Handle)) !std.json.Value {
    if (ancestors.items.len >= 256) return error.DepthLimit;
    return switch (value) {
        .null => .null,
        .boolean => |b| .{ .bool = b },
        .string => |s| .{ .string = s },
        .number => |n| blk: {
            if (!std.math.isFinite(n)) return error.UnsupportedValue;
            break :blk .{ .float = n };
        },
        .ref => |h| blk: {
            for (ancestors.items) |ancestor| if (ancestor == h) return error.UnsupportedValue;
            try ancestors.append(allocator, h);
            defer _ = ancestors.pop();
            const node = try graph.node(value);
            switch (node) {
                .object => |object| {
                    var result: std.json.ObjectMap = .{};
                    for (object.properties.items) |field| {
                        if (field.value == .undefined) return error.UnsupportedValue;
                        try result.put(allocator, field.key, try toJSON(allocator, graph, field.value, ancestors));
                    }
                    break :blk .{ .object = result };
                },
                .array => |array| {
                    var result: std.array_list.Managed(std.json.Value) = .init(allocator);
                    for (0..array.length) |i| {
                        const child = try graph.arrayGet(value, @intCast(i)) orelse return error.UnsupportedValue;
                        try result.append(try toJSON(allocator, graph, child, ancestors));
                    }
                    break :blk .{ .array = result };
                },
                else => return error.UnsupportedValue,
            }
        },
        else => error.UnsupportedValue,
    };
}
pub fn encode(allocator: std.mem.Allocator, graph: *d.Graph, value: d.Value) ![]u8 {
    var arena = std.heap.ArenaAllocator.init(allocator);
    defer arena.deinit();
    var ancestors: std.ArrayList(d.Handle) = .empty;
    const json = try toJSON(arena.allocator(), graph, value, &ancestors);
    return std.json.Stringify.valueAlloc(allocator, json, .{});
}
