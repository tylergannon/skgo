public sealed class NativeModelException(string message) : System.Exception(message);
// None means absent for Optional fields, and JSON null for Nullable fields.
public readonly record struct Maybe<T>
{
    public bool HasValue { get; }
    public T Value { get; }
    public Maybe(T value) { if (value is null) throw new NativeModelException("Expected a value"); HasValue = true; Value = value; }
    public static implicit operator Maybe<T>(T value) => new(value);
}
internal static class Wire
{
    internal static void Depth(int d) { if (d > 128) throw new NativeModelException("Model depth exceeded"); }
    internal static System.Text.Json.Nodes.JsonNode? Parse(byte[]? bytes) => bytes is null ? throw new NativeModelException("Expected a result") : System.Text.Json.Nodes.JsonNode.Parse(bytes);
    internal static byte[] Bytes(System.Text.Json.Nodes.JsonNode? node) => System.Text.Json.JsonSerializer.SerializeToUtf8Bytes(node);
    internal static System.Text.Json.Nodes.JsonObject Object(System.Text.Json.Nodes.JsonNode? n, params string[] keys)
    {
        if (n is not System.Text.Json.Nodes.JsonObject o || o.Any(p => !keys.Contains(p.Key))) throw new NativeModelException("Expected a closed object");
        return o;
    }
    internal static System.Text.Json.Nodes.JsonNode? Required(System.Text.Json.Nodes.JsonObject o, string key) => o.TryGetPropertyValue(key, out var n) ? n : throw new NativeModelException("Missing " + key);
    internal static T StringOrBool<T>(System.Text.Json.Nodes.JsonNode? n)
    {
        if (n is System.Text.Json.Nodes.JsonValue v && v.TryGetValue<T>(out var value) && value is not null) return value;
        throw new NativeModelException("Expected " + typeof(T).Name);
    }
    internal static System.Text.Json.Nodes.JsonNode EncodeText(string v) => System.Text.Json.Nodes.JsonValue.Create(v ?? throw new NativeModelException("Expected string"));
    internal static T Number<T>(System.Text.Json.Nodes.JsonNode? node, bool integer) where T : System.Numerics.INumberBase<T>
    {
        if (node is not System.Text.Json.Nodes.JsonValue v || !v.TryGetValue<double>(out var n) || !double.IsFinite(n) || (integer && (System.Math.Truncate(n) != n || System.Math.Abs(n) > 9007199254740991d))) throw new NativeModelException("Invalid number");
        try { var result = T.CreateChecked(n); if (!T.IsFinite(result)) throw new NativeModelException("Invalid number"); return result; }
        catch (System.OverflowException) { throw new NativeModelException("Number outside model range"); }
    }
    internal static System.Text.Json.Nodes.JsonNode EncodeNumber<T>(T value, bool integer) where T : System.Numerics.INumberBase<T>
    {
        var number = double.CreateChecked(value);
        if (!double.IsFinite(number) || (integer && System.Math.Abs(number) > 9007199254740991d)) throw new NativeModelException("Invalid number");
        return System.Text.Json.Nodes.JsonValue.Create(value)!;
    }
    internal static System.Collections.Generic.List<T> DecodeArray<T>(System.Text.Json.Nodes.JsonNode? n, long length, System.Func<System.Text.Json.Nodes.JsonNode?, T> decode)
    {
        if (n is not System.Text.Json.Nodes.JsonArray a || (length >= 0 && a.Count != length)) throw new NativeModelException("Invalid array length or type");
        return a.Select(decode).ToList();
    }
    internal static System.Text.Json.Nodes.JsonNode EncodeArray<T>(System.Collections.Generic.List<T> a, long length, System.Func<T, System.Text.Json.Nodes.JsonNode?> encode)
    {
        if (a is null || (length >= 0 && a.Count != length)) throw new NativeModelException("Invalid array length or type");
        return new System.Text.Json.Nodes.JsonArray(a.Select(encode).ToArray());
    }
    internal static Maybe<T> Optional<T>(System.Text.Json.Nodes.JsonObject o, string key, System.Func<System.Text.Json.Nodes.JsonNode?, T> decode) => o.TryGetPropertyValue(key, out var n) ? new(decode(n)) : default;
    internal static Maybe<T> Nullable<T>(System.Text.Json.Nodes.JsonObject o, string key, System.Func<System.Text.Json.Nodes.JsonNode?, T> decode) => Required(o, key) is {} n ? new(decode(n)) : default;
}
