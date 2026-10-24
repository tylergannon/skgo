const std = @import("std");
const nested = @import("src/routes/[foo]/[bar=nice]/data.remote.zig");
const grouped = @import("src/routes/(marketing)/[[optional]]/[...rest]/data.remote.zig");
test "imports Kit route paths directly" {
    try std.testing.expectEqual(@as(u32, 42), nested.answer);
    try std.testing.expectEqual(@as(u32, 17), grouped.answer);
}

test "compiler exposes remote function signature" {
    const signature = @typeInfo(@TypeOf(nested.lookup)).@"fn";
    try std.testing.expect(signature.param_types[0].? == nested.Input);
    try std.testing.expect(signature.return_type.? == nested.Output);
    try std.testing.expectEqual(@as(u32, 9), nested.lookup(.{ .id = 9 }).id);
    try std.testing.expectEqualStrings("nice", nested.lookup(.{ .id = 9 }).label);
}
