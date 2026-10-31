using System.Runtime.CompilerServices;
using System.Threading.Channels;

namespace SKGoNative;

public sealed record RemoteRequest(Uri Url, string Origin, byte[]? Body);
public sealed record RemoteResponse(ushort Status, byte[] Body);
public interface IRemoteTransport
{
    Task<RemoteResponse> PerformAsync(RemoteRequest request, CancellationToken cancellationToken);
}

// The application owns HttpClient, headers, cookies, and its disposal. Configure
// its handler with AllowAutoRedirect=false: commands must never be replayed.
public sealed class HttpClientTransport(HttpClient client) : IRemoteTransport
{
    public async Task<RemoteResponse> PerformAsync(RemoteRequest request, CancellationToken cancellationToken)
    {
        using var message = new HttpRequestMessage(request.Body is null ? HttpMethod.Get : HttpMethod.Post, request.Url);
        message.Headers.Add("Origin", request.Origin);
        if (request.Body is not null)
        {
            message.Content = new ByteArrayContent(request.Body);
            message.Content.Headers.ContentType = new("application/json");
        }
        using var response = await client.SendAsync(message, cancellationToken).ConfigureAwait(false);
        return new(checked((ushort)response.StatusCode), await response.Content.ReadAsByteArrayAsync(cancellationToken).ConfigureAwait(false));
    }
}

internal sealed record QueryState(bool Ready, bool Loading, byte[]? Value, RemoteException? Error);
public sealed record QuerySnapshot<T>(bool Ready, bool Loading, T? Current, RemoteException? Error);
public sealed class QueryUpdate
{
    internal RemoteClient Client { get; }
    internal Guid Lease { get; }
    internal QueryUpdate(RemoteClient client, Guid lease) { Client = client; Lease = lease; }
}
public sealed class RemoteQuery<T> : IDisposable
{
    private readonly RemoteClient client;
    private readonly Guid lease;
    private readonly Func<byte[]?, T> decode;
    internal RemoteQuery(RemoteClient client, Guid lease, Func<byte[]?, T> decode) { this.client = client; this.lease = lease; this.decode = decode; }
    public QueryUpdate Update => new(client, lease);
    public async Task<T> ValueAsync(CancellationToken cancellationToken = default) => decode(await client.QueryValueAsync(lease, false, cancellationToken).ConfigureAwait(false));
    public async Task<T> RefreshAsync(CancellationToken cancellationToken = default) => decode(await client.QueryValueAsync(lease, true, cancellationToken).ConfigureAwait(false));
    private QuerySnapshot<T> Convert(QueryState s) => new(s.Ready, s.Loading, s.Ready ? decode(s.Value) : default, s.Error);
    public QuerySnapshot<T> Snapshot() => Convert(client.Snapshot(lease));
    public async IAsyncEnumerable<QuerySnapshot<T>> Changes([EnumeratorCancellation] CancellationToken cancellationToken = default)
    {
        var (id, reader) = client.Observe(lease);
        try { await foreach (var state in reader.ReadAllAsync(cancellationToken).ConfigureAwait(false)) yield return Convert(state); }
        finally { client.Unobserve(lease, id); }
    }
    public void Dispose() { client.Release(lease); GC.SuppressFinalize(this); }
    ~RemoteQuery() { client.Release(lease); }
}

public sealed class RemoteClient : IDisposable
{
    public enum Kind { Query, Command }
    private readonly object gate = new();
    private string origin, basePath;
    private IRemoteTransport transport;
    private readonly CoreCache cache;
    private bool closed;
    private sealed record Pending(CancellationTokenSource Cancel, TaskCompletionSource<byte[]?> Completion, ulong Epoch, string[] Refreshes);
    private sealed record Flight(string Key, Native.Ticket Ticket, CancellationTokenSource Cancel);
    private sealed record Waiter(string Key, Native.Ticket Ticket, TaskCompletionSource<byte[]?> Completion);
    private sealed class Lease(string key, RemoteRequest request)
    {
        internal readonly string Key = key;
        internal readonly RemoteRequest Request = request;
        internal readonly Dictionary<Guid, Channel<QueryState>> Observers = [];
    }
    private readonly Dictionary<Guid, Pending> pending = [];
    private readonly Dictionary<ulong, Flight> flights = [];
    private readonly Dictionary<Guid, Waiter> waiters = [];
    private readonly Dictionary<Guid, Lease> leases = [];
    public RemoteClient(string origin, IRemoteTransport transport, string basePath = "", uint cacheCapacity = 256)
    {
        this.origin = origin; this.transport = transport; this.basePath = basePath; cache = new(cacheCapacity);
    }
    private static TaskCompletionSource<byte[]?> Completion() => new(TaskCreationOptions.RunContinuationsAsynchronously);
    private void Open() { if (closed) throw new RemoteException("closed"); }
    public void ResetSession(string? origin = null, string? basePath = null, IRemoteTransport? transport = null)
    {
        lock (gate)
        {
            Open(); Discard(new("sessionChanged")); cache.Reset();
            if (origin is not null) this.origin = origin;
            if (basePath is not null) this.basePath = basePath;
            if (transport is not null) this.transport = transport;
        }
    }
    public void Dispose()
    {
        lock (gate) { if (closed) return; closed = true; Discard(new("closed")); cache.Dispose(); }
    }
    public void Shutdown() => Dispose();
    private static void Cancel(CancellationTokenSource source) { _ = source.CancelAsync(); }
    private void Discard(RemoteException error)
    {
        foreach (var p in pending.Values) { p.Completion.TrySetException(error); Cancel(p.Cancel); }
        pending.Clear();
        foreach (var f in flights.Values) Cancel(f.Cancel);
        flights.Clear();
        foreach (var w in waiters.Values) w.Completion.TrySetException(error);
        waiters.Clear();
        foreach (var l in leases.Values) foreach (var o in l.Observers.Values) o.Writer.TryComplete();
        leases.Clear();
    }
    public async Task<byte[]?> CallAsync(string id, Kind kind, byte[]? argument = null, IReadOnlyList<QueryUpdate>? updates = null, CancellationToken cancellationToken = default)
    {
        cancellationToken.ThrowIfCancellationRequested();
        if (kind is not Kind.Query and not Kind.Command) throw new RemoteException("invalidWire");
        if (kind == Kind.Query)
        {
            if (updates is { Count: > 0 }) throw new RemoteException("invalidWire");
            using var query = Query(id, v => v, argument);
            return await query.ValueAsync(cancellationToken).ConfigureAwait(false);
        }
        Guid callID; Task<byte[]?> completion;
        lock (gate)
        {
            Open();
            var refreshes = (updates ?? []).Select(u => ReferenceEquals(u.Client, this) && leases.TryGetValue(u.Lease, out var l) ? l.Key : throw new RemoteException("releasedQuery")).Distinct().ToArray();
            var request = Native.Prepare(origin, basePath, id, true, argument, refreshes).Request;
            callID = Guid.NewGuid(); var p = new Pending(new(), Completion(), cache.Epoch, refreshes);
            pending.Add(callID, p); completion = p.Completion.Task;
            var http = transport;
            _ = Task.Run(async () =>
            {
                RemoteResponse? response = null; Exception? failure = null;
                try { p.Cancel.Token.ThrowIfCancellationRequested(); response = await http.PerformAsync(request, p.Cancel.Token).ConfigureAwait(false); }
                catch (Exception e) { failure = e; }
                lock (gate)
                {
                    if (pending.Remove(callID))
                    {
                        try { if (failure is not null) throw failure; p.Completion.TrySetResult(cache.Receive(response!, new() { Epoch = p.Epoch }, requested: p.Refreshes)); }
                        catch (Exception e) { p.Completion.TrySetException(e); }
                        Publish();
                    }
                }
                p.Cancel.Dispose();
            });
        }
        try { return await completion.WaitAsync(cancellationToken).ConfigureAwait(false); }
        finally
        {
            lock (gate) if (pending.Remove(callID, out var p)) { p.Completion.TrySetCanceled(cancellationToken); Cancel(p.Cancel); }
        }
    }
    public RemoteQuery<T> Query<T>(string id, Func<byte[]?, T> decode, byte[]? argument = null)
    {
        lock (gate)
        {
            Open(); var prepared = Native.Prepare(origin, basePath, id, false, argument, []);
            cache.Retain(prepared.Key); var idLease = Guid.NewGuid();
            leases.Add(idLease, new(prepared.Key, prepared.Request));
            try { Start(prepared.Key, prepared.Request, false); }
            catch { leases.Remove(idLease); cache.Release(prepared.Key); throw; }
            return new(this, idLease, decode);
        }
    }
    private Native.Ticket? Start(string key, RemoteRequest request, bool refresh)
    {
        var ticket = cache.Begin(key, refresh);
        if (ticket.Serial == 0) return flights.Values.Where(f => f.Key == key && cache.Waiting(key, f.Ticket)).OrderByDescending(f => f.Ticket.Serial).Select(f => (Native.Ticket?)f.Ticket).FirstOrDefault();
        var flight = new Flight(key, ticket, new()); flights.Add(ticket.Serial, flight);
        var http = transport;
        _ = Task.Run(async () =>
        {
            RemoteResponse? response = null; Exception? failure = null;
            try { flight.Cancel.Token.ThrowIfCancellationRequested(); response = await http.PerformAsync(request, flight.Cancel.Token).ConfigureAwait(false); }
            catch (Exception e) { failure = e; }
            lock (gate)
            {
                // Both identity and epoch matter: serials may be reused after reset.
                if (flights.TryGetValue(ticket.Serial, out var current) && ReferenceEquals(current, flight))
                {
                    flights.Remove(ticket.Serial);
                    try { if (failure is not null) throw failure; cache.Receive(response!, ticket, key); }
                    catch (Exception e)
                    {
                        failure = e;
                        try { cache.Reject(key, ticket, e); } catch { cache.Abort(key, ticket); }
                    }
                    Publish(failure is null ? null : (ticket.Serial, failure));
                }
            }
            flight.Cancel.Dispose();
        });
        Publish(); return ticket;
    }
    internal Task<byte[]?> QueryValueAsync(Guid id, bool refresh, CancellationToken token)
    {
        token.ThrowIfCancellationRequested();
        lock (gate)
        {
            var l = GetLease(id);
            var ticket = Start(l.Key, l.Request, refresh);
            if (ticket is null)
            {
                var s = cache.Snapshot(l.Key);
                if (s.Error is not null) throw s.Error;
                if (!s.Ready) throw new RemoteException("invalidWire");
                return Task.FromResult(s.Value);
            }
            cache.Retain(l.Key); var w = new Waiter(l.Key, ticket.Value, Completion()); var waiterID = Guid.NewGuid();
            waiters.Add(waiterID, w); return Await(waiterID, w, token);
        }
    }
    private async Task<byte[]?> Await(Guid id, Waiter w, CancellationToken token)
    {
        try { return await w.Completion.Task.WaitAsync(token).ConfigureAwait(false); }
        finally
        {
            lock (gate) if (waiters.Remove(id)) { w.Completion.TrySetCanceled(token); cache.Release(w.Key); CancelUnused(); }
        }
    }
    private Lease GetLease(Guid id) => leases.TryGetValue(id, out var l) ? l : throw new RemoteException("releasedQuery");
    internal QueryState Snapshot(Guid id) { lock (gate) return cache.Snapshot(GetLease(id).Key); }
    internal void Release(Guid id)
    {
        lock (gate)
        {
            if (!leases.Remove(id, out var l)) return;
            foreach (var o in l.Observers.Values) o.Writer.TryComplete();
            cache.Release(l.Key); CancelUnused();
        }
    }
    internal (Guid, ChannelReader<QueryState>) Observe(Guid id)
    {
        lock (gate)
        {
            var l = GetLease(id); var channel = Channel.CreateBounded<QueryState>(new BoundedChannelOptions(1) { FullMode = BoundedChannelFullMode.DropOldest });
            var observer = Guid.NewGuid(); l.Observers.Add(observer, channel); channel.Writer.TryWrite(cache.Snapshot(l.Key)); return (observer, channel.Reader);
        }
    }
    internal void Unobserve(Guid id, Guid observer) { lock (gate) if (leases.TryGetValue(id, out var l)) l.Observers.Remove(observer); }
    private void Publish((ulong Serial, Exception Error)? rejected = null)
    {
        foreach (var l in leases.Values)
        {
            var state = cache.Snapshot(l.Key);
            foreach (var o in l.Observers.Values) o.Writer.TryWrite(state);
        }
        foreach (var (id, w) in waiters.ToArray())
        {
            if (cache.Waiting(w.Key, w.Ticket)) continue;
            waiters.Remove(id);
            try
            {
                if (rejected is { } r && r.Serial == w.Ticket.Serial) throw r.Error;
                var state = cache.Snapshot(w.Key);
                if (!state.Ready) throw state.Error ?? new RemoteException("invalidWire");
                w.Completion.TrySetResult(state.Value);
            }
            catch (Exception e) { w.Completion.TrySetException(e); }
            cache.Release(w.Key);
        }
        CancelUnused();
    }
    private void CancelUnused()
    {
        foreach (var (serial, f) in flights.ToArray())
        {
            if (leases.Values.Any(l => l.Key == f.Key) || waiters.Values.Any(w => w.Key == f.Key)) continue;
            cache.Abort(f.Key, f.Ticket); flights.Remove(serial); Cancel(f.Cancel);
        }
    }
}
