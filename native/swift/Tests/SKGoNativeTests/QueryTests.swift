import Foundation
import Testing
@testable import SKGoNative

@Test(.timeLimit(.minutes(1))) func completedQueriesPreserveHTTPRemoteAndRedirectErrors() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let fixtures: [(UInt16, String, RemoteError)] = [
        (403, #"{"type":"error","error":{"message":"Forbidden"}}"#, .http(403, "Forbidden")),
        (200, #"{"type":"error","error":{"status":409,"message":"Conflict"}}"#, .remote(409, "Conflict")),
        (200, #"{"type":"result","data":"[{\"redirect\":1},\"/sign-in\"]"}"#, .redirect("/sign-in"))
    ]
    for (status, body, expected) in fixtures {
        let query = try await client.query("hash/query") { $0 }
        let request = await transport.next()
        await transport.finish(request, .init(status: status, body: Data(body.utf8)))
        var state = try await query.snapshot()
        while state.loading { await Task.yield(); state = try await query.snapshot() }
        #expect(state.error == expected)
        do { _ = try await query.value(); Issue.record("Completed error returned a value") }
        catch { #expect(error as? RemoteError == expected) }
        await query.release()
    }
    await client.shutdown()
}

@Test(.timeLimit(.minutes(1))) func retainedCanonicalQueriesShareRequestsAndOwnEvictedResults() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport, cacheCapacity: 1)
    let first = try await client.query("hash/query", argument: Data(#"{"offset":20,"limit":10}"#.utf8)) { $0 }
    let second = try await client.query("hash/query", argument: Data(#"{"limit":10,"offset":20}"#.utf8)) { $0 }
    let request = await transport.next()
    #expect(await transport.count() == 1)
    #expect(try await first.snapshot().loading)
    await transport.finish(request)
    let value = try await first.value()
    #expect(try await second.value() == value)
    #expect(await transport.count() == 1)
    await first.release()
    #expect(try await second.snapshot().ready)
    await second.release()
    let next = try await client.query("hash/query") { $0 }
    let nextID = await transport.next()
    #expect(nextID == 1)
    await transport.finish(nextID)
    #expect(try await next.value() == value)
    #expect(String(data: value!, encoding: .utf8) == #"{"text":"Native voice transcript: café 😀","revision":1}"#)
    await next.release()
    await client.shutdown()
    #expect(String(data: value!, encoding: .utf8)!.contains("café 😀"))
}

@Test(.timeLimit(.minutes(1))) func queryRefreshOrderingAndObservedLoading() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let query = try await client.query("hash/query") { $0 }
    var changes = try await query.changes().makeAsyncIterator()
    #expect(try await changes.next()?.loading == true)
    let first = await transport.next()
    let refresh = Task { try await query.refresh() }
    let second = await transport.next()
    // SKGo and Kit both include the query's own q node. Every arrival applies
    // set(), even after a newer-issued request has settled all pending promises.
    await transport.finish(second, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1,\"q\":2},2,{\"hash/query/\":3},{\"v\":1}]"}"#.utf8)))
    #expect(try await refresh.value == Data("2".utf8))
    let ready = try await query.snapshot()
    #expect(ready.ready && !ready.loading && ready.current == Data("2".utf8))
    // Consume the new response's publication before watching the old one land.
    var observed = try #require(try await changes.next())
    while observed.current != Data("2".utf8) { observed = try #require(try await changes.next()) }
    await transport.finish(first, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1,\"q\":2},1,{\"hash/query/\":3},{\"v\":1}]"}"#.utf8)))
    observed = try #require(try await changes.next())
    while observed.current != Data("1".utf8) { observed = try #require(try await changes.next()) }
    #expect(observed.ready && !observed.loading && observed.error == nil)
    #expect(try await query.value() == Data("1".utf8))
    let failed = Task { try await query.refresh() }
    let third = await transport.next()
    await transport.finish(third, .init(status: 200, body: Data(#"{"type":"error","error":{"status":503,"message":"Unavailable"}}"#.utf8)))
    do { _ = try await failed.value; Issue.record("Refresh succeeded") } catch { #expect(error as? RemoteError == .remote(503, "Unavailable")) }
    let state = try await query.snapshot()
    #expect(state.ready && !state.loading && state.current == Data("1".utf8) && state.error == .remote(503, "Unavailable"))
    await query.release()
    await client.shutdown()
}

@Test(.timeLimit(.minutes(1))) func commandRefreshesApplyFulfilledIgnoredAndUnhandledKeys() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let a = try await client.query("hash/a") { $0 }
    let b = try await client.query("hash/b") { $0 }
    let c = try await client.query("hash/c") { $0 }
    for _ in 0..<3 {
        let request = await transport.next()
        await transport.finish(request, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1},1]"}"#.utf8)))
    }
    #expect(try await a.value() == Data("1".utf8))
    #expect(try await b.value() == Data("1".utf8))
    #expect(try await c.value() == Data("1".utf8))
    let pending = Task { try await c.refresh() }
    let pendingRequest = await transport.next()
    #expect(try await c.snapshot().loading)
    let command = Task { try await client.call("hash/command", kind: .command, updates: [a.update, b.update, c.update, a.update]) }
    let id = await transport.next()
    #expect(String(data: await transport.request(id).body!, encoding: .utf8) == #"{"payload":"","refreshes":["hash/a/","hash/b/","hash/c/"]}"#)
    await transport.finish(id, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1,\"q\":2,\"i\":5},\"ack\",{\"hash/a/\":3},{\"v\":4},2,[6],\"hash/b/\"]"}"#.utf8)))
    #expect(try await command.value == Data(#""ack""#.utf8))
    #expect(try await a.value() == Data("2".utf8))
    #expect(try await b.value() == Data("1".utf8))
    let state = try await c.snapshot()
    #expect(state.ready && !state.loading && state.current == Data("1".utf8) && state.error == .remote(400, "Requested update was not handled by the remote function"))
    #expect(try await pending.value == Data("1".utf8))
    #expect(await transport.count() == 5)
    await client.shutdown()
    await transport.finish(pendingRequest)
}

@Test(.timeLimit(.minutes(1))) func cancellingOneAwaiterKeepsSharedRequestAndAwaitPinsReleasedLease() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport)
    let query = try await client.query("hash/query") { $0 }
    let request = await transport.next()
    let cancelled = Task { try await query.value() }
    cancelled.cancel()
    do { _ = try await cancelled.value; Issue.record("Cancelled waiter succeeded") } catch is CancellationError {}
    let value = Task { try await query.value() }
    try await waitForAwaitingPin(client)
    await query.release()
    await transport.finish(request)
    #expect(try await value.value != nil)
    #expect(await transport.count() == 1)
    await client.shutdown()
}

@Test(.timeLimit(.minutes(1))) func sessionReplacementRejectsOldWaitersAndCompletions() async throws {
    let transport = ControlledTransport()
    let client = RemoteClient(origin: "http://127.0.0.1:8080", transport: transport, cacheCapacity: 1)
    let old = try await client.query("hash/query") { $0 }
    let first = await transport.next()
    let waiter = Task { try await old.value() }
    try await waitForAwaitingPin(client)
    try await client.resetSession(origin: "http://127.0.0.1:9090")
    do { _ = try await waiter.value; Issue.record("Old waiter survived reset") } catch { #expect(error as? RemoteError == .sessionChanged) }
    let current = try await client.query("hash/query") { $0 }
    let second = await transport.next()
    #expect(await transport.request(second).url.absoluteString == "http://127.0.0.1:9090/_app/remote/hash/query")
    await transport.finish(first)
    #expect(try await current.snapshot().ready == false)
    await transport.finish(second, .init(status: 200, body: Data(#"{"type":"result","data":"[{\"_\":1},2]"}"#.utf8)))
    #expect(try await current.value() == Data("2".utf8))
    await old.release()
    #expect(try await current.snapshot().ready)
    await current.release()
}

private func waitForAwaitingPin(_ client: RemoteClient) async throws {
    let deadline = ContinuousClock.now.advanced(by: .seconds(1))
    while await client.queryWaiterCount == 0 && ContinuousClock.now < deadline {
        try Task.checkCancellation()
        await Task.yield()
    }
    try #require(await client.queryWaiterCount == 1)
}
