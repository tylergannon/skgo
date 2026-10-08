import Foundation
import Testing
@testable import SKGoNative

private let acknowledgement = RemoteResponse(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1},{\"text\":2,\"revision\":3},\"Native voice transcript: café 😀\",1]"}"#.utf8))

actor ControlledTransport: RemoteTransport {
    var requests: [RemoteRequest] = []
    private var waiting: [Int: CheckedContinuation<RemoteResponse, Error>] = [:]
    private var observations: [CheckedContinuation<Int, Never>] = []
    private var unobserved: [Int] = []
    func perform(_ request: RemoteRequest) async throws -> RemoteResponse {
        let id = requests.count
        requests.append(request)
        return try await withCheckedThrowingContinuation { continuation in
            waiting[id] = continuation
            if !observations.isEmpty { observations.removeFirst().resume(returning: id) }
            else { unobserved.append(id) }
        }
    }
    func next() async -> Int {
        if !unobserved.isEmpty { return unobserved.removeFirst() }
        return await withCheckedContinuation { observations.append($0) }
    }
    func finish(_ id: Int, _ response: RemoteResponse = acknowledgement) { waiting.removeValue(forKey: id)?.resume(returning: response) }
    func request(_ id: Int) -> RemoteRequest { requests[id] }
    func count() -> Int { requests.count }
}

@Test func omittedArgumentAndNullRemainDistinctBuffers() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let omitted = Task { try await client.call("hash/query", kind: .query) }
    let first = await transport.next()
    #expect(await transport.request(first).url.absoluteString == "http://127.0.0.1:8080/_app/remote/hash/query")
    await transport.finish(first, .init(status: 200, body: Data(#"{"type":"result","data":"[{}]"}"#.utf8)))
    #expect(try await omitted.value == nil)
    let null = Task { try await client.call("hash/query", kind: .query, argument: Data("null".utf8)) }
    let second = await transport.next()
    #expect(await transport.request(second).url.absoluteString == "http://127.0.0.1:8080/_app/remote/hash/query?payload=W251bGxd")
    await transport.finish(second, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1},null]"}"#.utf8)))
    #expect(try await null.value == Data("null".utf8))
}

@Test func invalidArgumentBuffersNeverDispatch() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    for (input, expected) in [(Data(), RemoteError.invalidWire), (Data("broken".utf8), .invalidWire), (Data("9223372036854775807".utf8), .unsupportedValue)] {
        do { _ = try await client.call("hash/command", kind: .command, argument: input); Issue.record("Invalid input was accepted") }
        catch { #expect(error as? RemoteError == expected) }
    }
    #expect(await transport.count() == 0)
}

@Test func literalInputAndSwiftOwnedResult() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", base: "/native", transport: transport)
    let call = Task { try await client.call("hash/acceptText", kind: .command, argument: Data(#""Native voice transcript: café 😀""#.utf8)) }
    let id = await transport.next()
    let request = await transport.request(id)
    #expect(request.method == "POST")
    #expect(request.origin == "http://127.0.0.1:8080")
    #expect(request.url.absoluteString == "http://127.0.0.1:8080/native/_app/remote/hash/acceptText")
    #expect(String(data: request.body!, encoding: .utf8) == #"{"payload":"WyJOYXRpdmUgdm9pY2UgdHJhbnNjcmlwdDogY2Fmw6kg8J-YgCJd","refreshes":[]}"#)
    await transport.finish(id)
    let value = try await call.value
    await client.shutdown()
    let object = try JSONSerialization.jsonObject(with: value!) as! [String: Any]
    #expect(object["text"] as? String == "Native voice transcript: café 😀")
    #expect(object["revision"] as? Int == 1)
}

@Test func cancellationBeforeDispatchSendsNothing() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    // Cancellation is installed before the actor receives this call.
    let call = Task {
        withUnsafeCurrentTask { $0?.cancel() }
        return try await client.call("hash/command", kind: .command)
    }
    do { _ = try await call.value; Issue.record("Cancelled call succeeded") } catch is CancellationError {} catch { throw error }
    #expect(await transport.count() == 0)
}

@Test func cancellationReturnsBeforeALateTransportCompletion() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let call = Task { try await client.call("hash/command", kind: .command) }
    let id = await transport.next()
    call.cancel()
    // Transport deliberately ignores cancellation. Caller must still finish.
    do { _ = try await call.value; Issue.record("Cancelled call succeeded") } catch is CancellationError {} catch { throw error }
    await transport.finish(id)
    let next = Task { try await client.call("hash/query", kind: .query) }
    let nextID = await transport.next()
    #expect(nextID == 1)
    await transport.finish(nextID)
    let object = try JSONSerialization.jsonObject(with: try await next.value!) as! [String: Any]
    #expect(object["revision"] as? Int == 1)
    #expect(await transport.count() == 2)
}

@Test func shutdownRejectsPendingAndFutureCalls() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let call = Task { try await client.call("hash/query", kind: .query) }
    let id = await transport.next()
    await client.shutdown()
    do { _ = try await call.value; Issue.record("Shutdown call succeeded") } catch { #expect(error as? RemoteError == .closed) }
    await transport.finish(id)
    do { _ = try await client.call("hash/query", kind: .query); Issue.record("Closed client accepted work") } catch { #expect(error as? RemoteError == .closed) }
    #expect(await transport.count() == 1)
}

@Test func concurrentCallersReceiveTheirOwnCompletion() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let tasks = (0..<16).map { index in Task { try await client.call("hash/query", kind: .query, argument: Data(String(index).utf8)) } }
    var ids: [Int] = []
    for _ in 0..<16 { ids.append(await transport.next()) }
    for id in ids.reversed() {
        let request = await transport.request(id)
        let payload = URLComponents(url: request.url, resolvingAgainstBaseURL: false)!.queryItems!.first!.value!
        let padded = payload.replacingOccurrences(of: "-", with: "+").replacingOccurrences(of: "_", with: "/") + String(repeating: "=", count: (4 - payload.count % 4) % 4)
        let input = String(data: Data(base64Encoded: padded)!, encoding: .utf8)!
        let number = String(input.dropFirst().dropLast())
        await transport.finish(id, .init(status: 200, body: Data("{\"type\":\"result\",\"data\":\"[{\\\"_\\\":1},\(number)]\"}".utf8)))
    }
    for (index, task) in tasks.enumerated() { #expect(try JSONSerialization.jsonObject(with: try await task.value!, options: .fragmentsAllowed) as? Int == index) }
    #expect(await transport.count() == 16)
}

@Test func errorsAndRedirectsRemainDistinct() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let fixtures: [(UInt16, String, RemoteError)] = [
        (502, "Bad gateway", .http(502, "")),
        (200, #"{"type":"error","error":{"status":409,"message":"Revision conflict"}}"#, .remote(409, "Revision conflict")),
        (200, #"{"type":"result","data":"[{\"redirect\":1},\"/sign-in\"]"}"#, .redirect("/sign-in")),
        (200, "broken", .invalidWire),
        (200, #"{"type":"result","data":"[{\"_\":1},{\"self\":1}]"}"#, .unsupportedValue)
    ]
    for (status, body, error) in fixtures {
        let call = Task { try await client.call("hash/query", kind: .query) }
        let id = await transport.next()
        await transport.finish(id, .init(status: status, body: Data(body.utf8)))
        do { _ = try await call.value; Issue.record("Expected \(error)") } catch let actual { #expect(actual as? RemoteError == error) }
    }
}
