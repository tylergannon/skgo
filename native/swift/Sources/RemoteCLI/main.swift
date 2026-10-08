import SKGoNative
import Foundation
#if canImport(Darwin)
import Darwin
#else
import Glibc
#endif

@main struct RemoteCLI {
    static func main() async {
        do { try await run() }
        catch { FileHandle.standardError.write(Data("\(error)\n".utf8)); exit(1) }
    }
    private static func run() async throws {
        let args = CommandLine.arguments
        guard args.count == 5 else { throw RemoteError.invalidWire }
        let client = RemoteClient(origin: args[1], base: args[2])
        do {
            _ = try await client.call(args[3], kind: .command, argument: Data(#""Native voice transcript: café 😀""#.utf8))
            let result = try await client.call(args[4], kind: .query)
            guard let result, let object = try JSONSerialization.jsonObject(with: result) as? [String: Any], object["text"] as? String == "Native voice transcript: café 😀", object["revision"] as? Int == 1 else { throw RemoteError.invalidWire }
            await client.shutdown()
            print("Native voice transcript: café 😀\nrevision=1")
        } catch {
            await client.shutdown()
            throw error
        }
    }
}
