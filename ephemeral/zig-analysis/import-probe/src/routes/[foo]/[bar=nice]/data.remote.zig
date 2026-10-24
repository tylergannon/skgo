pub const answer: u32 = 42;
pub const Input = struct { id: u32 };
pub const Output = struct { id: u32, label: []const u8 };
pub fn lookup(input: Input) Output {
    return .{ .id = input.id, .label = "nice" };
}
