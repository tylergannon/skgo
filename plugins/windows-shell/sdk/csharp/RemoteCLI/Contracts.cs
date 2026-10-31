using System.Text;
using System.Threading.Channels;
using SKGoNative;

// Host-side concurrency and finite-wire regressions. The running-server proof
// in Program remains separate from these literal transport fixtures.
internal static class Contracts
{
    private static byte[] Bytes(string s) => Encoding.UTF8.GetBytes(s);
    private static string? Text(byte[]? b) => b is null ? null : Encoding.UTF8.GetString(b);
    private static void Check(bool yes) { if (!yes) throw new Exception("Client contract failed"); }
    private static async Task Error(Func<Task> action, string code)
    {
        try { await action(); throw new Exception("Expected " + code); }
        catch (RemoteException e) { Check(e.Code == code); }
    }
    internal static async Task RunAsync()
    {
        var http = new Controlled();
        using var client = new RemoteClient("http://localhost:8080", http, cacheCapacity: 1);
        using var first = client.Query("hash/query", Text, Bytes("{\"offset\":20,\"limit\":10}"));
        using var second = client.Query("hash/query", Text, Bytes("{\"limit\":10,\"offset\":20}"));
        var request = await http.Next();
        Check(http.Count == 1 && first.Snapshot().Loading);
        await Error(() => Task.Run(() => client.Query("hash/full", Text)), "cacheFull");
        request.Finish(Result("1"));
        Check(await first.ValueAsync() == "1" && await second.ValueAsync() == "1" && http.Count == 1);
        using var cancelled = new CancellationTokenSource();
        cancelled.Cancel();
        try { await second.ValueAsync(cancelled.Token); throw new Exception("Cancelled await passed"); } catch (OperationCanceledException) { }
        using var cancelledWait = new CancellationTokenSource();
        var cancelledRefresh = first.RefreshAsync(cancelledWait.Token);
        var sharedFlight = await http.Next();
        var survivingWait = second.ValueAsync();
        cancelledWait.Cancel();
        try { await cancelledRefresh.WaitAsync(TimeSpan.FromSeconds(2)); throw new Exception("Cancelled waiter succeeded"); } catch (OperationCanceledException) { }
        sharedFlight.Finish(Result("3")); Check(await survivingWait == "3");
        first.Dispose(); second.Dispose();
        using var ordered = client.Query("hash/order", Text);
        var initialOrder = await http.Next();
        var refresh = ordered.RefreshAsync(); var older = await http.Next();
        var refresh2 = ordered.RefreshAsync(); var newer = await http.Next();
        newer.Finish(Result("2", "hash/order/"));
        Check(await refresh2 == "2" && await refresh == "2");
        older.Finish(Result("1", "hash/order/"));
        Check(ordered.Snapshot().Current == "1");
        initialOrder.Finish(Result("0", "hash/order/"));
        Check(ordered.Snapshot().Current == "0");
        var failedRefresh = ordered.RefreshAsync();
        var failed = await http.Next();
        failed.Finish(new(200, Bytes("""{"type":"error","error":{"status":503,"message":"Unavailable"}}""")));
        await Error(async () => { await failedRefresh; }, "remote");
        Check(ordered.Snapshot() is { Ready: true, Loading: false, Current: "0", Error.Status: 503 });
        ordered.Dispose();
        using var pinned = client.Query("hash/pinned", Text);
        request = await http.Next();
        var value = pinned.ValueAsync(); pinned.Dispose();
        request.Finish(Result("7"));
        Check(await value == "7");

        using var raw = new RemoteClient("http://localhost:8080", http);
        foreach (var arg in new[] { Array.Empty<byte>(), Bytes("broken"), Bytes("9223372036854775807") })
        {
            var code = arg.Length > 10 ? "unsupportedValue" : "invalidWire";
            await Error(async () => { await raw.CallAsync("hash/command", RemoteClient.Kind.Command, arg); }, code);
        }
        using var before = new CancellationTokenSource(); before.Cancel(); var count = http.Count;
        try { await raw.CallAsync("hash/command", RemoteClient.Kind.Command, cancellationToken: before.Token); throw new Exception("Cancelled dispatch"); } catch (OperationCanceledException) { }
        Check(http.Count == count);
        var omitted = raw.CallAsync("hash/query", RemoteClient.Kind.Query);
        request = await http.Next(); Check(request.Request.Url.AbsoluteUri == "http://localhost:8080/_app/remote/hash/query");
        request.Finish(new(200, Bytes("""{"type":"result","data":"[{}]"}""")));
        Check(await omitted is null);
        var nullCall = raw.CallAsync("hash/query", RemoteClient.Kind.Query, Bytes("null"));
        request = await http.Next(); Check(request.Request.Url.AbsoluteUri.EndsWith("?payload=W251bGxd"));
        request.Finish(Result("null")); Check(Text(await nullCall) == "null");
        foreach (var (status, body, code) in new (ushort,string,string)[] {
            (502,"Bad gateway","http"),
            (200,"""{"type":"error","error":{"status":409,"message":"Revision conflict"}}""","remote"),
            (200,"""{"type":"result","data":"[{\"redirect\":1},\"/sign-in\"]"}""","redirect"),
            (200,"broken","invalidWire"),
            (200,"""{"type":"result","data":"[{\"_\":1},{\"self\":1}]"}""","unsupportedValue") })
        {
            using var q = raw.Query("hash/error", Text);
            request = await http.Next(); request.Finish(new(status, Bytes(body)));
            Check(!q.Snapshot().Loading);
            await Error(async () => { await q.ValueAsync(); }, code is "invalidWire" or "unsupportedValue" ? "remote" : code);
        }
        var calls = Enumerable.Range(0,16).Select(i => raw.CallAsync("hash/command", RemoteClient.Kind.Command, Bytes(i.ToString()))).ToArray();
        var requests = new List<Controlled.Held>();
        for(var i=0;i<16;i++) requests.Add(await http.Next());
        foreach(var r in requests.AsEnumerable().Reverse()) {
            using var body = System.Text.Json.JsonDocument.Parse(r.Request.Body!);
            var encoded = body.RootElement.GetProperty("payload").GetString()!.Replace('-', '+').Replace('_', '/');
            encoded = encoded.PadRight((encoded.Length + 3) / 4 * 4, '=');
            var argument = Encoding.UTF8.GetString(Convert.FromBase64String(encoded));
            r.Finish(Result(argument[1..^1]));
        }
        Check((await Task.WhenAll(calls)).Select((b, i) => Text(b) == i.ToString()).All(b => b));
        var replacement = new Controlled();
        raw.ResetSession("http://localhost:9090", "/other", replacement);
        var changedOrigin = raw.CallAsync("hash/new", RemoteClient.Kind.Query);
        request = await replacement.Next();
        Check(request.Request.Url.AbsoluteUri == "http://localhost:9090/other/_app/remote/hash/new" && request.Request.Origin == "http://localhost:9090");
        request.Finish(Result("5")); Check(Text(await changedOrigin) == "5");
        Console.WriteLine("contracts: cache capacity/sharing, refresh ordering, await pinning, pre-dispatch cancellation, invalid wire/models, concurrent calls PASS");
    }
    private static RemoteResponse Result(string json, string? key = null) => new(200, Bytes("{\"type\":\"result\",\"data\":" + System.Text.Json.JsonSerializer.Serialize(key is null ? "[{\"_\":1},"+json+"]" : "[{\"_\":1,\"q\":2},"+json+",{\""+key+"\":3},{\"v\":1}]") + "}"));
    private sealed class Controlled : IRemoteTransport
    {
        internal sealed record Held(RemoteRequest Request)
        {
            internal readonly TaskCompletionSource<RemoteResponse> Completion = new();
            internal void Finish(RemoteResponse response) => Completion.SetResult(response);
        }
        private readonly Channel<Held> channel = Channel.CreateUnbounded<Held>();
        private int count;
        internal int Count => Volatile.Read(ref count);
        internal async Task<Held> Next() => await channel.Reader.ReadAsync().AsTask().WaitAsync(TimeSpan.FromSeconds(5));
        public Task<RemoteResponse> PerformAsync(RemoteRequest request, CancellationToken cancellationToken)
        {
            Interlocked.Increment(ref count);
            var h = new Held(request); channel.Writer.TryWrite(h); return h.Completion.Task;
        }
    }
}
