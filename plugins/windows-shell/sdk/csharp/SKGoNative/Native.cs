using System.Runtime.InteropServices;
using System.Text;
using System.Text.Json;

namespace SKGoNative;

public sealed class RemoteException(string code, uint status = 0, string message = "") : Exception(message.Length == 0 ? code : message)
{
    public string Code { get; } = code;
    public uint Status { get; } = status;
}

// Only the RemoteClient gate may access a cache. Every ABI buffer is copied and
// freed synchronously. No borrowed pointer survives a call or crosses an await.
internal static class Native
{
    private const string Library = "skgo_native_core_shared";
    [StructLayout(LayoutKind.Sequential)] internal struct Buffer { internal IntPtr Ptr; internal nuint Length; }
    [StructLayout(LayoutKind.Sequential)] internal struct Request { internal Buffer Url, Origin, Body, Key; }
    [StructLayout(LayoutKind.Sequential)] internal struct Reply { internal uint Kind, Status; internal Buffer Value, Message; }
    [StructLayout(LayoutKind.Sequential)] internal struct Ticket { internal ulong Epoch, Serial; }
    [StructLayout(LayoutKind.Sequential)] internal struct State { internal uint Ready, Loading, Kind, Status; internal Buffer Value, Message; }
    // SafeHandle's critical finalizer runs after query lease finalizers, and
    // P/Invoke pins the handle throughout every native cache operation.
    internal sealed class CacheHandle : SafeHandle
    {
        internal CacheHandle(uint capacity) : base(IntPtr.Zero, true)
        {
            Check(sk_cache_create(capacity, out var pointer)); SetHandle(pointer);
        }
        public override bool IsInvalid => handle == IntPtr.Zero;
        protected override bool ReleaseHandle()
        {
            var pointer = handle; sk_cache_release(ref pointer); handle = pointer; return true;
        }
    }
    internal sealed class Input : IDisposable
    {
        private GCHandle handle;
        internal Buffer Bytes;
        internal Input(byte[]? bytes)
        {
            if (bytes is not { Length: > 0 }) return;
            handle = GCHandle.Alloc(bytes, GCHandleType.Pinned);
            Bytes = new() { Ptr = handle.AddrOfPinnedObject(), Length = (nuint)bytes.Length };
        }
        internal Input(string text) : this(Encoding.UTF8.GetBytes(text)) { }
        public void Dispose() { if (handle.IsAllocated) handle.Free(); }
    }
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_prepare(Buffer origin, Buffer @base, Buffer id, uint command, Buffer argument, Buffer refreshes, out Request request);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern void sk_buffer_release(ref Buffer buffer);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_create(uint capacity, out IntPtr cache);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern void sk_cache_release(ref IntPtr cache);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_reset(CacheHandle cache);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_retain_query(CacheHandle cache, Buffer key);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_release_query(CacheHandle cache, Buffer key);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_begin(CacheHandle cache, Buffer key, uint refresh, out Ticket ticket);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_waiting(CacheHandle cache, Buffer key, Ticket ticket);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern void sk_cache_abort(CacheHandle cache, Buffer key, Ticket ticket);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_reject(CacheHandle cache, Buffer key, Ticket ticket, uint kind, ushort status, Buffer message);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_snapshot(CacheHandle cache, Buffer key, out State state);
    [DllImport(Library, CallingConvention = CallingConvention.Cdecl)] internal static extern uint sk_cache_receive(CacheHandle cache, Buffer key, Ticket ticket, ushort status, Buffer body, Buffer requested, out Reply reply);
    internal static void Check(uint status)
    {
        if (status != 0) throw new RemoteException(status switch { 1 => "allocation", 2 => "invalidWire", 3 => "unsupportedValue", 4 => "cacheFull", 5 => "releasedQuery", _ => "exhausted" });
    }
    internal static byte[]? Copy(Buffer b)
    {
        if (b.Ptr == IntPtr.Zero) return null;
        var bytes = new byte[checked((int)b.Length)];
        Marshal.Copy(b.Ptr, bytes, 0, bytes.Length);
        return bytes;
    }
    internal static string Text(Buffer b) => Encoding.UTF8.GetString(Copy(b) ?? []);
    internal static RemoteException Failure(uint kind, uint status, string message) => new(kind switch { 1 => "http", 2 => "remote", 3 => "redirect", _ => "invalidWire" }, status, message);
    internal static (RemoteRequest Request, string Key) Prepare(string origin, string @base, string id, bool command, byte[]? argument, string[] refreshes)
    {
        if (argument is { Length: 0 }) throw new RemoteException("invalidWire");
        using var o = new Input(origin); using var b = new Input(@base); using var i = new Input(id);
        using var a = new Input(argument); using var r = new Input(JsonSerializer.SerializeToUtf8Bytes(refreshes));
        var status = sk_prepare(o.Bytes, b.Bytes, i.Bytes, command ? 1u : 0u, a.Bytes, r.Bytes, out var value);
        try { Check(status); return (new(new Uri(Text(value.Url)), Text(value.Origin), command ? Copy(value.Body) : null), Text(value.Key)); }
        finally { sk_buffer_release(ref value.Url); sk_buffer_release(ref value.Origin); sk_buffer_release(ref value.Body); sk_buffer_release(ref value.Key); }
    }
}

internal sealed class CoreCache : IDisposable
{
    private readonly Native.CacheHandle pointer;
    internal ulong Epoch { get; private set; } = 1;
    internal CoreCache(uint capacity) { pointer = new(capacity); }
    public void Dispose() => pointer.Dispose();
    internal void Reset() { Native.Check(Native.sk_cache_reset(pointer)); Epoch++; }
    internal void Retain(string key) { using var k = new Native.Input(key); Native.Check(Native.sk_cache_retain_query(pointer, k.Bytes)); }
    internal void Release(string key) { using var k = new Native.Input(key); Native.Check(Native.sk_cache_release_query(pointer, k.Bytes)); }
    internal Native.Ticket Begin(string key, bool refresh)
    {
        using var k = new Native.Input(key); Native.Check(Native.sk_cache_begin(pointer, k.Bytes, refresh ? 1u : 0u, out var ticket)); return ticket;
    }
    internal bool Waiting(string key, Native.Ticket ticket) { using var k = new Native.Input(key); return Native.sk_cache_waiting(pointer, k.Bytes, ticket) != 0; }
    internal void Abort(string key, Native.Ticket ticket) { using var k = new Native.Input(key); Native.sk_cache_abort(pointer, k.Bytes, ticket); }
    internal void Reject(string key, Native.Ticket ticket, Exception error)
    {
        var e = error as RemoteException;
        using var k = new Native.Input(key); using var m = new Native.Input(error.Message);
        Native.Check(Native.sk_cache_reject(pointer, k.Bytes, ticket, e?.Code switch { "http" => 1u, "redirect" => 3u, _ => 2u }, (ushort)(e?.Status is > 0 and < 600 ? e.Status : 500), m.Bytes));
    }
    internal QueryState Snapshot(string key)
    {
        using var k = new Native.Input(key);
        var status = Native.sk_cache_snapshot(pointer, k.Bytes, out var state);
        try { Native.Check(status); return new(state.Ready != 0, state.Loading != 0, Native.Copy(state.Value), state.Kind == 0 ? null : Native.Failure(state.Kind, state.Status, Native.Text(state.Message))); }
        finally { Native.sk_buffer_release(ref state.Value); Native.sk_buffer_release(ref state.Message); }
    }
    internal byte[]? Receive(RemoteResponse response, Native.Ticket ticket, string key = "", string[]? requested = null)
    {
        using var k = new Native.Input(key); using var b = new Native.Input(response.Body);
        using var r = new Native.Input(JsonSerializer.SerializeToUtf8Bytes(requested ?? []));
        var status = Native.sk_cache_receive(pointer, k.Bytes, ticket, response.Status, b.Bytes, r.Bytes, out var reply);
        try { Native.Check(status); if (reply.Kind != 0) throw Native.Failure(reply.Kind, reply.Status, Native.Text(reply.Message)); return Native.Copy(reply.Value); }
        finally { Native.sk_buffer_release(ref reply.Value); Native.sk_buffer_release(ref reply.Message); }
    }
}
