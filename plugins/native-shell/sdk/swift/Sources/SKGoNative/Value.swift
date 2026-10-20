import CSKGo
import Foundation

public enum RemoteError: Error, Sendable, Equatable {
    case allocation, invalidWire, unsupportedValue, closed, cacheFull, releasedQuery, sessionChanged
    case http(Int, String), remote(Int, String), redirect(String)
}
func check(_ status: UInt32) throws {
    switch status { case 0: return; case 1: throw RemoteError.allocation; case 3: throw RemoteError.unsupportedValue; case 4: throw RemoteError.cacheFull; case 5: throw RemoteError.releasedQuery; case 6: throw RemoteError.closed; default: throw RemoteError.invalidWire }
}
func copy(_ buffer: SKBuffer) -> Data {
    guard buffer.len > 0, let ptr = buffer.ptr else { return Data() }
    return Data(bytes: ptr, count: buffer.len)
}
func text(_ buffer: SKBuffer) throws -> String {
    guard let result = String(data: copy(buffer), encoding: .utf8) else { throw RemoteError.invalidWire }
    return result
}
func withBytes<T>(_ data: Data, _ body: (SKBytes) throws -> T) rethrows -> T {
    try data.withUnsafeBytes { bytes in try body(SKBytes(ptr: bytes.bindMemory(to: UInt8.self).baseAddress, len: bytes.count)) }
}
