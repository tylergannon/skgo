import SwiftUI
import SKGoNative
import AppKit

@MainActor final class ApplicationState: ObservableObject {
    @Published var result = "Connecting…"
    init(host: NativeHost) {
        guard let origin = host.url?.absoluteString else { return }
        let client = RemoteClient(origin: origin)
        let api = NativeAPI(core: client)
        host.onShutdown = { Task { await client.shutdown() } }
        Task {
            do {
                let before = try await api.readValue()
                FileHandle.standardOutput.write(Data("\(before)\n".utf8))
                if CommandLine.arguments.count > 1 {
                    _ = try await api.writeValue(CommandLine.arguments[1])
                    let after = try await api.readValue()
                    FileHandle.standardOutput.write(Data("\(after)\n".utf8))
                    result = after
                    NSApplication.shared.terminate(nil)
                } else { result = before }
            } catch {
                result = String(describing: error)
                FileHandle.standardError.write(Data("\(error)\n".utf8))
                if CommandLine.arguments.count > 1 { exit(1) }
            }
        }
    }
}
struct ApplicationView: View {
    @ObservedObject var state: ApplicationState
    let url: URL
    let recoveryRevision: Int
    var body: some View {
        VStack { Text(state.result).padding(); ApplicationWebView(url: url, recoveryRevision: recoveryRevision) }
    }
}
#if os(macOS)
struct ApplicationSettingsView: View {
    @ObservedObject var state: ApplicationState
    var body: some View { Text("No application settings.").padding() }
}
#endif
