import CSKGo
import Foundation
#if canImport(FoundationNetworking)
import FoundationNetworking
#endif

public struct RemoteRequest: Sendable {
    public let url: URL
    public let origin: String
    public let body: Data?
    public var method: String { body == nil ? "GET" : "POST" }
}
public struct RemoteResponse: Sendable {
    public let status: UInt16
    public let body: Data
    public init(status: UInt16, body: Data) { self.status = status; self.body = body }
}
public protocol RemoteTransport: Sendable {
    func perform(_ request: RemoteRequest) async throws -> RemoteResponse
}

// Kit redirects are data inside a successful envelope. HTTP redirects must not
// replay a command, including URLSession's automatic 307/308 replay behavior.
private final class NoRedirect: NSObject, URLSessionTaskDelegate, Sendable {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
                    newRequest request: URLRequest, completionHandler: @escaping @Sendable (URLRequest?) -> Void) {
        completionHandler(nil)
    }
}
public final class URLSessionTransport: RemoteTransport {
    private let session: URLSession
    private let delegate = NoRedirect()
    public init(configuration: URLSessionConfiguration = .default) {
        session = URLSession(configuration: configuration)
    }
    public func perform(_ request: RemoteRequest) async throws -> RemoteResponse {
        var urlRequest = URLRequest(url: request.url)
        urlRequest.httpMethod = request.method
        urlRequest.httpBody = request.body
        urlRequest.setValue(request.origin, forHTTPHeaderField: "Origin")
        if request.body != nil { urlRequest.setValue("application/json", forHTTPHeaderField: "Content-Type") }
        let (body, response) = try await session.data(for: urlRequest, delegate: delegate)
        guard let http = response as? HTTPURLResponse, (100...599).contains(http.statusCode) else { throw RemoteError.invalidWire }
        return RemoteResponse(status: UInt16(http.statusCode), body: body)
    }
    deinit { session.invalidateAndCancel() }
}

public actor RemoteClient {
    public enum Kind: Sendable { case query, command }
    private var origin: String
    private var base: String
    private let transport: any RemoteTransport
    private var nextID: UInt64 = 0
    private struct Pending { let task: Task<Void, Never>; let continuation: CheckedContinuation<Data?, Error>; let epoch: UInt64; let refreshes: [String] }
    private var pending: [UInt64: Pending] = [:]
    private var closed = false
    private let cacheCapacity: UInt32
    private var cache: CoreCache?
    private struct Lease { let key: Data; let request: RemoteRequest; var observers: [UUID: AsyncStream<QueryState>.Continuation] = [:] }
    private struct Flight { let key: Data; let ticket: SKTicket; let task: Task<Void, Never> }
    private struct Waiter { let key: Data; let ticket: SKTicket; let continuation: CheckedContinuation<Data?, Error> }
    private var leases: [UUID: Lease] = [:]
    private var flights: [UInt64: Flight] = [:]
    private var waiters: [UUID: Waiter] = [:]
    var queryWaiterCount: Int { waiters.count }
    public init(origin: String, base: String = "", transport: any RemoteTransport = URLSessionTransport(), cacheCapacity: UInt32 = 256) {
        self.origin = origin; self.base = base; self.transport = transport; self.cacheCapacity = cacheCapacity
    }
    public func shutdown() {
        closed = true
        discard(RemoteError.closed)
        cache = nil
    }
    /// Call after replacing authentication or the server. Existing leases close;
    /// consumers retain new queries in the new session.
    public func resetSession(origin: String? = nil, base: String? = nil) throws {
        guard !closed else { throw RemoteError.closed }
        discard(RemoteError.sessionChanged)
        try cache?.reset()
        if let origin { self.origin = origin }
        if let base { self.base = base }
    }
    private func discard(_ error: RemoteError) {
        for call in pending.values { call.task.cancel(); call.continuation.resume(throwing: error) }
        pending.removeAll()
        for flight in flights.values { flight.task.cancel() }
        flights.removeAll()
        for waiter in waiters.values { waiter.continuation.resume(throwing: error) }
        waiters.removeAll()
        for lease in leases.values { for observer in lease.observers.values { observer.finish() } }
        leases.removeAll()
    }
    private func core() throws -> CoreCache {
        if let cache { return cache }
        let value = try CoreCache(capacity: cacheCapacity)
        cache = value
        return value
    }
    private func cancel(_ id: UInt64) {
        guard let call = pending.removeValue(forKey: id) else { return }
        call.task.cancel()
        call.continuation.resume(throwing: CancellationError())
    }
    private func complete(_ id: UInt64, result: Result<RemoteResponse, Error>) {
        guard let call = pending.removeValue(forKey: id) else { return }
        do { call.continuation.resume(returning: try core().receive(result.get(), ticket: SKTicket(epoch: call.epoch, serial: 0), requested: call.refreshes)) }
        catch { call.continuation.resume(throwing: error) }
        publish()
    }
    public func call(_ id: String, kind: Kind, argument: Data? = nil, updates: [QueryUpdate] = []) async throws -> Data? {
        try Task.checkCancellation()
        guard !closed else { throw RemoteError.closed }
        if let argument, argument.isEmpty { throw RemoteError.invalidWire }
        if kind == .query {
            guard updates.isEmpty else { throw RemoteError.invalidWire }
            let query = try query(id, argument: argument) { $0 }
            do { let value = try await query.value(); await query.release(); return value }
            catch { await query.release(); throw error }
        }
        let cache = try core()
        var refreshes: [String] = []
        for update in updates {
            guard update.client === self, let lease = leases[update.lease], let key = String(data: lease.key, encoding: .utf8) else { throw RemoteError.releasedQuery }
            if !refreshes.contains(key) { refreshes.append(key) }
        }
        let request = try prepareRequest(origin: origin, base: base, id: id, kind: kind, argument: argument, refreshes: refreshes).request
        let callID = nextID
        // Wrapping would make a late completion ambiguous; it cannot occur in a
        // realistic process lifetime, but must never silently reuse an ID.
        guard nextID < UInt64.max else { throw RemoteError.closed }
        nextID += 1
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                guard !Task.isCancelled else { continuation.resume(throwing: CancellationError()); return }
                let task = Task {
                    do {
                        try Task.checkCancellation()
                        let response = try await transport.perform(request)
                        complete(callID, result: .success(response))
                    } catch { complete(callID, result: .failure(error)) }
                }
                pending[callID] = Pending(task: task, continuation: continuation, epoch: cache.epoch, refreshes: refreshes)
            }
        } onCancel: { Task { await self.cancel(callID) } }
    }

    public func query<Value: Sendable>(_ id: String, argument: Data? = nil,
        decode: @escaping @Sendable (Data?) throws -> Value) throws -> RemoteQuery<Value> {
        guard !closed else { throw RemoteError.closed }
        if let argument, argument.isEmpty { throw RemoteError.invalidWire }
        let prepared = try prepareRequest(origin: origin, base: base, id: id, kind: .query, argument: argument)
        let cache = try core()
        try cache.retain(prepared.key)
        let lease = UUID()
        leases[lease] = Lease(key: prepared.key, request: prepared.request)
        do { _ = try start(prepared.key, request: prepared.request, refresh: false) }
        catch { leases.removeValue(forKey: lease); cache.release(prepared.key); throw error }
        return RemoteQuery(client: self, lease: lease, decode: decode)
    }
    private func start(_ key: Data, request: RemoteRequest, refresh: Bool) throws -> SKTicket? {
        let cache = try core()
        let ticket = try cache.begin(key, refresh: refresh)
        if ticket.serial != 0 {
            let task = Task {
                do { finishQuery(ticket.serial, result: .success(try await transport.perform(request))) }
                catch { finishQuery(ticket.serial, result: .failure(error)) }
            }
            flights[ticket.serial] = Flight(key: key, ticket: ticket, task: task)
            publish()
            return ticket
        }
        return flights.values.filter { $0.key == key && cache.waiting(key, $0.ticket) }.max { $0.ticket.serial < $1.ticket.serial }?.ticket
    }
    private func finishQuery(_ serial: UInt64, result: Result<RemoteResponse, Error>) {
        guard let flight = flights.removeValue(forKey: serial), let cache else { return }
        var failure: Error?
        do { _ = try cache.receive(result.get(), key: flight.key, ticket: flight.ticket) }
        catch {
            failure = error
            do { try cache.reject(flight.key, flight.ticket, error: error) }
            catch { cache.abort(flight.key, flight.ticket) }
        }
        publish(rejected: failure.map { (serial, $0) })
    }
    func queryValue(_ id: UUID, refresh: Bool) async throws -> Data? {
        try Task.checkCancellation()
        guard let lease = leases[id] else { throw RemoteError.releasedQuery }
        let cache = try core()
        guard let ticket = try start(lease.key, request: lease.request, refresh: refresh) else {
            let state = try cache.snapshot(lease.key)
            if let error = state.error { throw error }
            guard state.ready else { throw RemoteError.invalidWire }
            return state.value
        }
        // Awaiting pins the resource independently of the UI lease.
        try cache.retain(lease.key)
        let waiter = UUID()
        return try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                if Task.isCancelled { cache.release(lease.key); continuation.resume(throwing: CancellationError()); return }
                waiters[waiter] = Waiter(key: lease.key, ticket: ticket, continuation: continuation)
            }
        } onCancel: { Task { await self.cancelWaiter(waiter) } }
    }
    private func cancelWaiter(_ id: UUID) {
        guard let waiter = waiters.removeValue(forKey: id) else { return }
        cache?.release(waiter.key)
        waiter.continuation.resume(throwing: CancellationError())
        cancelUnretainedFlights()
    }
    func releaseQuery(_ id: UUID) {
        guard let lease = leases.removeValue(forKey: id) else { return }
        for observer in lease.observers.values { observer.finish() }
        cache?.release(lease.key)
        cancelUnretainedFlights()
    }
    private func cancelUnretainedFlights() {
        for (serial, flight) in flights where !leases.values.contains(where: { $0.key == flight.key }) && !waiters.values.contains(where: { $0.key == flight.key }) {
            flight.task.cancel(); flights.removeValue(forKey: serial)
        }
    }
    func querySnapshot(_ id: UUID) throws -> QueryState {
        guard let lease = leases[id] else { throw RemoteError.releasedQuery }
        return try core().snapshot(lease.key)
    }
    func queryChanges(_ id: UUID) throws -> AsyncStream<QueryState> {
        guard var lease = leases[id] else { throw RemoteError.releasedQuery }
        let state = try core().snapshot(lease.key)
        let observer = UUID()
        let (stream, continuation) = AsyncStream<QueryState>.makeStream(bufferingPolicy: .bufferingNewest(1))
        lease.observers[observer] = continuation
        leases[id] = lease
        continuation.yield(state)
        continuation.onTermination = { _ in Task { await self.removeObserver(id, observer) } }
        return stream
    }
    private func removeObserver(_ id: UUID, _ observer: UUID) { leases[id]?.observers.removeValue(forKey: observer) }
    private func publish(rejected: (UInt64, Error)? = nil) {
        guard let cache else { return }
        for lease in leases.values {
            if let state = try? cache.snapshot(lease.key) { for observer in lease.observers.values { observer.yield(state) } }
        }
        for (id, waiter) in waiters where !cache.waiting(waiter.key, waiter.ticket) {
            waiters.removeValue(forKey: id)
            do {
                if let rejected, rejected.0 == waiter.ticket.serial { throw rejected.1 }
                let state = try cache.snapshot(waiter.key)
                if state.ready { waiter.continuation.resume(returning: state.value) }
                else { throw state.error ?? RemoteError.invalidWire }
            } catch { waiter.continuation.resume(throwing: error) }
            cache.release(waiter.key)
        }
        cancelUnretainedFlights()
    }

}

// JSON is the local ABI format. Zig alone translates to/from Kit's devalue wire.
private func prepareRequest(origin: String, base: String, id: String, kind: RemoteClient.Kind, argument: Data?, refreshes: [String] = []) throws -> (request: RemoteRequest, key: Data) {
    var request = SKRequest()
    defer { sk_buffer_release(&request.url); sk_buffer_release(&request.origin); sk_buffer_release(&request.body); sk_buffer_release(&request.key) }
    let refreshData = try JSONSerialization.data(withJSONObject: refreshes)
    let status = withBytes(Data(origin.utf8)) { origin in
        withBytes(Data(base.utf8)) { base in
            withBytes(Data(id.utf8)) { id in
                withBytes(argument ?? Data()) { argument in withBytes(refreshData) { refreshes in sk_prepare(origin, base, id, kind == .command ? 1 : 0, argument, refreshes, &request) } }
            }
        }
    }
    try check(status)
    guard let url = URL(string: try text(request.url)) else { throw RemoteError.invalidWire }
    return (RemoteRequest(url: url, origin: try text(request.origin), body: kind == .command ? copy(request.body) : nil), copy(request.key))
}
func result(_ reply: SKReply) throws -> Data? {
    let message = try text(reply.message)
    switch reply.kind {
    case 0: return reply.value.ptr == nil ? nil : copy(reply.value)
    case 1: throw RemoteError.http(Int(reply.status), message)
    case 2: throw RemoteError.remote(Int(reply.status), message)
    case 3: throw RemoteError.redirect(message)
    default: throw RemoteError.invalidWire
    }
}
