import CSKGo
import Foundation

public struct QuerySnapshot<Value: Sendable>: Sendable {
    public let ready: Bool
    public let loading: Bool
    public let current: Value?
    public let error: RemoteError?
}

/// A Swift-owned lease. It contains no foreign value or cache-entry pointer.
public final class RemoteQuery<Value: Sendable>: Sendable {
    private let client: RemoteClient
    private let lease: UUID
    private let decode: @Sendable (Data?) throws -> Value
    init(client: RemoteClient, lease: UUID, decode: @escaping @Sendable (Data?) throws -> Value) {
        self.client = client; self.lease = lease; self.decode = decode
    }
    public var update: QueryUpdate { QueryUpdate(client: client, lease: lease) }
    public func value() async throws -> Value { try decode(try await client.queryValue(lease, refresh: false)) }
    public func refresh() async throws -> Value { try decode(try await client.queryValue(lease, refresh: true)) }
    public func snapshot() async throws -> QuerySnapshot<Value> { try convert(try await client.querySnapshot(lease)) }
    public func release() async { await client.releaseQuery(lease) }
    public func changes() async throws -> AsyncThrowingStream<QuerySnapshot<Value>, Error> {
        let source = try await client.queryChanges(lease)
        let (stream, continuation) = AsyncThrowingStream<QuerySnapshot<Value>, Error>.makeStream(bufferingPolicy: .bufferingNewest(1))
        let decode = self.decode
        let task = Task {
            do {
                for await state in source {
                    continuation.yield(QuerySnapshot(ready: state.ready, loading: state.loading,
                        current: state.ready ? try decode(state.value) : nil, error: state.error))
                }
                continuation.finish()
            } catch { continuation.finish(throwing: error) }
        }
        continuation.onTermination = { _ in task.cancel() }
        return stream
    }
    private func convert(_ state: QueryState) throws -> QuerySnapshot<Value> {
        QuerySnapshot(ready: state.ready, loading: state.loading, current: state.ready ? try decode(state.value) : nil, error: state.error)
    }
    deinit {
        let client = client, lease = lease
        Task { await client.releaseQuery(lease) }
    }
}
public struct QueryUpdate: Sendable {
    let client: RemoteClient
    let lease: UUID
}
struct QueryState: Sendable {
    let ready: Bool
    let loading: Bool
    let value: Data?
    let error: RemoteError?
}

/// Owned once by RemoteClient and accessed only on that actor. Buffers returned
/// by the ABI are copied/released synchronously; the opaque context never leaks.
final class CoreCache: @unchecked Sendable {
    private var pointer: OpaquePointer?
    var epoch: UInt64 = 1
    init(capacity: UInt32) throws { try check(sk_cache_create(capacity, &pointer)) }
    deinit { sk_cache_release(&pointer) }
    func reset() throws { try check(sk_cache_reset(pointer)); epoch += 1 }
    func retain(_ key: Data) throws { try check(withBytes(key) { sk_cache_retain_query(pointer, $0) }) }
    func release(_ key: Data) { _ = withBytes(key) { sk_cache_release_query(pointer, $0) } }
    func begin(_ key: Data, refresh: Bool) throws -> SKTicket {
        var ticket = SKTicket()
        try check(withBytes(key) { sk_cache_begin(pointer, $0, refresh ? 1 : 0, &ticket) })
        return ticket
    }
    func waiting(_ key: Data, _ ticket: SKTicket) -> Bool { withBytes(key) { sk_cache_waiting(pointer, $0, ticket) } != 0 }
    func abort(_ key: Data, _ ticket: SKTicket) { withBytes(key) { sk_cache_abort(pointer, $0, ticket) } }
    func reject(_ key: Data, _ ticket: SKTicket, error: Error) throws {
        let status: UInt16, message: String
        switch error as? RemoteError {
        case .http(let s, let m), .remote(let s, let m): status = UInt16(s); message = m
        case .redirect(let m): status = 307; message = m
        default: status = 500; message = String(describing: error)
        }
        try check(withBytes(key) { k in withBytes(Data(message.utf8)) { sk_cache_reject(pointer, k, ticket, status, $0) } })
    }
    func snapshot(_ key: Data) throws -> QueryState {
        var state = SKCacheState()
        defer { sk_buffer_release(&state.value); sk_buffer_release(&state.message) }
        try check(withBytes(key) { sk_cache_snapshot(pointer, $0, &state) })
        return QueryState(ready: state.ready != 0, loading: state.loading != 0,
            value: state.value.ptr == nil ? nil : copy(state.value),
            error: state.status == 0 ? nil : .remote(Int(state.status), try text(state.message)))
    }
    func receive(_ response: RemoteResponse, key: Data = Data(), ticket: SKTicket, requested: [String] = []) throws -> Data? {
        var reply = SKReply()
        defer { sk_buffer_release(&reply.value); sk_buffer_release(&reply.message) }
        let refreshes = try JSONSerialization.data(withJSONObject: requested)
        try check(withBytes(key) { k in withBytes(response.body) { b in withBytes(refreshes) {
            sk_cache_receive(pointer, k, ticket, response.status, b, $0, &reply)
        } } })
        return try result(reply)
    }
}
