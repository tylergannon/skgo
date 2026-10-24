// This application does not import the code generator.
const remote = @import("src/routes/[foo]/[bar=nice]/data.remote.zig");

pub fn main() void {
    const result = remote.lookup(.{ .id = 42 });
    if (result.id != 42) unreachable;
}
