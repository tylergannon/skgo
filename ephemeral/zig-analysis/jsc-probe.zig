const std = @import("std");
const Context = opaque {};
const Value = opaque {};
const JSString = opaque {};
extern fn JSGlobalContextCreate(global_class: ?*anyopaque) ?*Context;
extern fn JSGlobalContextRelease(ctx: *Context) void;
extern fn JSStringCreateWithUTF8CString(text: [*:0]const u8) ?*JSString;
extern fn JSStringRelease(text: *JSString) void;
extern fn JSEvaluateScript(ctx: *Context, script: *JSString, this_object: ?*anyopaque, source_url: ?*JSString, starting_line: c_int, exception: *?*const Value) ?*const Value;
extern fn JSValueToNumber(ctx: *Context, value: *const Value, exception: *?*const Value) f64;

test "Zig embeds system JavaScriptCore through its C API" {
    const ctx = JSGlobalContextCreate(null) orelse return error.ContextCreationFailed;
    defer JSGlobalContextRelease(ctx);
    const script = JSStringCreateWithUTF8CString("class C { #v = 42; read() { return this.#v; } }; new C().read()") orelse return error.StringCreationFailed;
    defer JSStringRelease(script);
    var exception: ?*const Value = null;
    const value = JSEvaluateScript(ctx, script, null, null, 1, &exception) orelse return error.EvaluationFailed;
    try std.testing.expect(exception == null);
    try std.testing.expectEqual(@as(f64, 42), JSValueToNumber(ctx, value, &exception));
    try std.testing.expect(exception == null);
}
