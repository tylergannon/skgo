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
    private let origin: String
    private let base: String
    private let transport: any RemoteTransport
    private var nextID: UInt64 = 0
    private struct Pending { let task: Task<Void, Never>; let continuation: CheckedContinuation<Data?, Error> }
    private var pending: [UInt64: Pending] = [:]
    private var closed = false
    public init(origin: String, base: String = "", transport: any RemoteTransport = URLSessionTransport()) {
        self.origin = origin; self.base = base; self.transport = transport
    }
    public func shutdown() {
        closed = true
        for call in pending.values { call.task.cancel(); call.continuation.resume(throwing: RemoteError.closed) }
        pending.removeAll()
    }
    private func cancel(_ id: UInt64) {
        guard let call = pending.removeValue(forKey: id) else { return }
        call.task.cancel()
        call.continuation.resume(throwing: CancellationError())
    }
    private func complete(_ id: UInt64, result: Result<RemoteResponse, Error>) {
        guard let call = pending.removeValue(forKey: id) else { return }
        do { call.continuation.resume(returning: try receiveResponse(result.get())) }
        catch { call.continuation.resume(throwing: error) }
    }
    public func call(_ id: String, kind: Kind, argument: Data? = nil) async throws -> Data? {
        try Task.checkCancellation()
        guard !closed else { throw RemoteError.closed }
        if let argument, argument.isEmpty { throw RemoteError.invalidWire }
        let request = try prepareRequest(origin: origin, base: base, id: id, kind: kind, argument: argument)
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
                pending[callID] = Pending(task: task, continuation: continuation)
            }
        } onCancel: { Task { await self.cancel(callID) } }
    }

}

// JSON is the local ABI format. Zig alone translates to/from Kit's devalue wire.
private func prepareRequest(origin: String, base: String, id: String, kind: RemoteClient.Kind, argument: Data?) throws -> RemoteRequest {
    var request = SKRequest()
    defer { sk_buffer_release(&request.url); sk_buffer_release(&request.origin); sk_buffer_release(&request.body) }
    let status = withBytes(Data(origin.utf8)) { origin in
        withBytes(Data(base.utf8)) { base in
            withBytes(Data(id.utf8)) { id in
                withBytes(argument ?? Data()) { argument in sk_prepare(origin, base, id, kind == .command ? 1 : 0, argument, &request) }
            }
        }
    }
    try check(status)
    guard let url = URL(string: try text(request.url)) else { throw RemoteError.invalidWire }
    return RemoteRequest(url: url, origin: try text(request.origin), body: kind == .command ? copy(request.body) : nil)
}
private func receiveResponse(_ response: RemoteResponse) throws -> Data? {
    var reply = SKReply()
    defer { sk_buffer_release(&reply.value); sk_buffer_release(&reply.message) }
    try check(withBytes(response.body) { sk_decode(response.status, $0, &reply) })
    let message = try text(reply.message)
    switch reply.kind {
    case 0: return reply.value.ptr == nil ? nil : copy(reply.value)
    case 1: throw RemoteError.http(Int(reply.status), message)
    case 2: throw RemoteError.remote(Int(reply.status), message)
    case 3: throw RemoteError.redirect(message)
    default: throw RemoteError.invalidWire
    }
}
