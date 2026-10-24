// Deliberately small projection probe, not a production wire schema.
fn tsType(comptime T: type) []const u8 {
    if (T == []const u8) return "string";
    return switch (@typeInfo(T)) {
        .int => |info| if (info.bits <= 53) "number" else @compileError("integer needs explicit wire policy"),
        .bool => "boolean",
        .optional => |info| tsType(info.child) ++ " | null",
        else => @compileError("unsupported probe type: " ++ @typeName(T)),
    };
}

fn swiftType(comptime T: type) []const u8 {
    if (T == []const u8) return "String";
    if (T == u32) return "UInt32";
    if (T == bool) return "Bool";
    return switch (@typeInfo(T)) {
        .optional => |info| swiftType(info.child) ++ "?",
        else => @compileError("unsupported probe type: " ++ @typeName(T)),
    };
}

pub fn typeScriptDecl(comptime name: []const u8, comptime T: type) []const u8 {
    const info = @typeInfo(T).@"struct";
    comptime var result: []const u8 = "export interface " ++ name ++ " {\n";
    inline for (info.field_names, info.field_types) |field, Field| {
        result = result ++ "  " ++ field ++ ": " ++ tsType(Field) ++ ";\n";
    }
    return result ++ "}\n";
}

pub fn swiftDecl(comptime name: []const u8, comptime T: type) []const u8 {
    const info = @typeInfo(T).@"struct";
    comptime var result: []const u8 = "struct " ++ name ++ ": Codable {\n";
    inline for (info.field_names, info.field_types) |field, Field| {
        result = result ++ "    let " ++ field ++ ": " ++ swiftType(Field) ++ "\n";
    }
    return result ++ "}\n";
}
