import SwiftUI
import WebKit
import SKGoNative

@main
struct NativeProbe: App {
    @Environment(\.scenePhase) private var scenePhase
    @State private var origin: URL?
    @State private var failed = false
    @State private var client: RemoteClient?
    @State private var result = ""
    @State private var calling = false

    var body: some Scene {
        WindowGroup {
            Group {
                if let origin {
                    VStack {
                        Button("Run native query and command") { Task { await callNative() } }
                            .disabled(calling || client == nil)
                        if !result.isEmpty { Text(result) }
                        Page(url: origin.appendingPathComponent("todos"))
                            .id(origin)
                    }
                } else if failed {
                    Text("The local server could not start.")
                } else {
                    ProgressView("Starting local server…")
                }
            }
            .task { start() }
            .onChange(of: scenePhase) { _, phase in
                if phase == .active { start() }
                if phase == .background {
                    let previous = client
                    client = nil
                    Task { await previous?.shutdown() }
                    SKGoHostStop()
                    origin = nil
                    result = ""
                    calling = false
                }
            }
        }
    }
    private func start() {
        guard origin == nil else { return }
        guard let bytes = SKGoHostStart() else { failed = true; return }
        defer { free(bytes) }
        origin = URL(string: String(cString: bytes))
        failed = origin == nil
        if let origin { client = RemoteClient(origin: origin.absoluteString, transport: URLSessionTransport(configuration: .ephemeral)) }
    }
    private func callNative() async {
        guard let client else { return }
        calling = true
        let api = NativeAPI(core: client)
        do {
            _ = try await api.whoami()
            let signedIn = try await api.signIn("Native probe")
            try await client.resetSession()
            let received = try await api.whoami()
            guard self.client === client else { return }
            result = "Command: \(signedIn.user); query: \(received.user)"
        } catch {
            guard self.client === client else { return }
            result = "Native call failed: \(error)"
        }
        calling = false
    }
}
private struct Page: UIViewRepresentable {
    let url: URL
    func makeUIView(context: Context) -> WKWebView {
        let view = WKWebView()
        view.load(URLRequest(url: url))
        return view
    }
    func updateUIView(_ view: WKWebView, context: Context) {}
}
