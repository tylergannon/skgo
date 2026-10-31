using System.Runtime.InteropServices;
using System.Text;
using System.Threading.Channels;
using GeneratedRemote;
using SKGoNative;

internal static class Program
{
    private static void Check(bool yes, string claim) { if (!yes) throw new Exception("FAILED: " + claim); }
    private static async Task Error(Func<Task> call, string code, uint? status = null, string? message = null)
    {
        try { await call(); throw new Exception("Expected " + code); }
        catch (RemoteException e) { Check(e.Code == code && (status is null || e.Status == status) && (message is null || e.Message == message), $"error {code}: {e}"); }
    }
    private static async Task Cancelled(Task task)
    {
        try { await task.WaitAsync(TimeSpan.FromSeconds(2)); throw new Exception("Cancellation succeeded"); }
        catch (OperationCanceledException) { }
    }
    public static async Task Main(string[] args)
    {
        if (args.Length != 2) throw new Exception("usage: RemoteCLI ORIGIN PRESENCE_REMOTE_ID (generated fixture API required)");
        Console.WriteLine($"host={RuntimeInformation.OSDescription}; arch={RuntimeInformation.ProcessArchitecture}; runtime={RuntimeInformation.FrameworkDescription}");
        using var http = new HttpClient(new HttpClientHandler { AllowAutoRedirect = false });
        http.DefaultRequestHeaders.Add("X-Native-Consumer", "csharp");
        var transport = new HttpClientTransport(http);
        using var core = new RemoteClient(args[0], transport, "/native");
        var api = new NativeAPI(core);
        var initial = await api.getTextAsync();
        Check(initial.text == "" && initial.revision == 0, "initial literal");
        Console.WriteLine("initial: text=\"\" revision=0");
        using var retained = api.RetainGetText();
        using var shared = api.RetainGetText();
        Check((await retained.ValueAsync()).revision == 0 && (await shared.ValueAsync()).revision == 0, "retained initial");
        await using var observer = retained.Changes().GetAsyncEnumerator();
        Check(await observer.MoveNextAsync() && observer.Current.Ready, "observe initial");
        var changed = await api.acceptTextAsync("Native voice transcript: café 😀", [retained.Update]);
        Check(changed.text == "Native voice transcript: café 😀" && changed.revision == 1, "command literal");
        Check(await observer.MoveNextAsync() && observer.Current.Current?.revision == 1, "observe single flight update");
        Check(shared.Snapshot().Current?.revision == 1, "shared cache update");
        var confirmed = await retained.RefreshAsync();
        Check((await api.getTextAsync()).revision == 1, "generated one-shot read");
        Check(confirmed.text == "Native voice transcript: café 😀" && confirmed.revision == 1, "confirmed literal");
        Console.WriteLine("confirmed: text=\"Native voice transcript: café 😀\" revision=1");
        Check((await retained.RefreshAsync()).revision == 1, "explicit refresh");
        await api.ignoreUpdatesAsync([retained.Update]);
        Check(retained.Snapshot().Error is null && retained.Snapshot().Current?.revision == 1, "ignored update");
        await api.unhandledUpdatesAsync([retained.Update]);
        Check(retained.Snapshot().Error is { Code: "remote", Status: 400 } && retained.Snapshot().Current?.revision == 1, "unhandled update");
        Check((await retained.RefreshAsync()).revision == 1, "refresh clears error");
        Console.WriteLine("retained: shared value, observed command update, refresh, ignored/unhandled updates PASS");
        await Error(async () => { await api.conflictAsync(); }, "remote", 409, "Revision conflict");
        await Error(async () => { await api.signInAsync(); }, "redirect", message: "/sign-in");
        await Error(async () => { await api.httpRedirectAsync(); }, "http", 307);
        var omitted = await core.CallAsync(args[1], RemoteClient.Kind.Query);
        var explicitNull = await core.CallAsync(args[1], RemoteClient.Kind.Query, Encoding.UTF8.GetBytes("null"));
        Check(Encoding.UTF8.GetString(omitted!) == "\"omitted\"" && Encoding.UTF8.GetString(explicitNull!) == "\"null\"", "omission/null");
        Console.WriteLine("errors: remote 409, Kit redirect, HTTP 307 without replay, omitted/null PASS");
        await Lifecycle(args[0], transport);
        await Contracts.RunAsync();
    }
    private static async Task Lifecycle(string origin, IRemoteTransport http)
    {
        var controlled = new HeldTransport(http);
        using var core = new RemoteClient(origin, controlled, "/native", 1);
        var api = new NativeAPI(core);
        using var cancelled = new CancellationTokenSource();
        var command = api.ignoreUpdatesAsync(cancellationToken: cancelled.Token);
        var held = await controlled.Next(); // real server has already answered
        cancelled.Cancel(); await Cancelled(command); held.Release();
        using var query = api.RetainGetText();
        held = await controlled.Next();
        var waiter = query.ValueAsync();
        var pendingCommand = api.ignoreUpdatesAsync();
        var heldCommand = await controlled.Next();
        core.Shutdown();
        await Error(async () => { await pendingCommand; }, "closed");
        heldCommand.Release();
        await Error(async () => { await waiter; }, "closed");
        await Error(async () => { await api.getTextAsync(); }, "closed");
        held.Release();
        Console.WriteLine("lifecycle: cancellation releases caller; shutdown closes pending/future work PASS");

        using var reset = new RemoteClient(origin, controlled, "/native", 1);
        var resetAPI = new NativeAPI(reset);
        using var old = resetAPI.RetainSessionNumber();
        var oldResponse = await controlled.Next();
        var oldValue = old.ValueAsync();
        await using var oldObserver = old.Changes().GetAsyncEnumerator();
        Check(await oldObserver.MoveNextAsync(), "old observer starts");
        var oldCommand = resetAPI.ignoreUpdatesAsync();
        var oldCommandResponse = await controlled.Next();
        var replacementTransport = new HeldTransport(http);
        reset.ResetSession(transport: replacementTransport);
        await Error(async () => { await oldValue; }, "sessionChanged");
        await Error(async () => { await oldCommand; }, "sessionChanged");
        Check(!await oldObserver.MoveNextAsync(), "reset closes observers");
        using var current = resetAPI.RetainSessionNumber();
        var newResponse = await replacementTransport.Next();
        var currentValue = current.ValueAsync();
        oldCommandResponse.Release();
        // Deliver a distinct literal new-session value. Late old completions
        // must neither settle its await nor overwrite it after it is ready.
        Check(!currentValue.IsCompleted && !current.Snapshot().Ready, "new query waits for its own response");
        newResponse.Release();
        Check(await currentValue == 2, "new session value");
        oldResponse.Release();
        old.Dispose();
        Check(current.Snapshot().Ready && current.Snapshot().Current == 2, "old lease cannot release current query");
        Console.WriteLine("lifecycle: reset rejects pending query/command, closes observation, discards old completions PASS");
    }
}

// Only delays responses from real HTTP; it deliberately ignores cancellation.
// Application code needs only IRemoteTransport/HttpClientTransport.
internal sealed class HeldTransport(IRemoteTransport http) : IRemoteTransport
{
    internal sealed record Held(RemoteResponse Response)
    {
        internal readonly TaskCompletionSource<RemoteResponse> Completion = new();
        internal void Release() => Completion.TrySetResult(Response);
    }
    private readonly Channel<Held> responses = Channel.CreateUnbounded<Held>();
    internal async Task<Held> Next() => await responses.Reader.ReadAsync().AsTask().WaitAsync(TimeSpan.FromSeconds(5));
    public async Task<RemoteResponse> PerformAsync(RemoteRequest request, CancellationToken cancellationToken)
    {
        var held = new Held(await http.PerformAsync(request, CancellationToken.None));
        responses.Writer.TryWrite(held);
        return await held.Completion.Task;
    }
}
