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
    @State private var lifecycleTask: Task<Void, Never>?
    @State private var generation = UUID()
    private let host = LocalHost()

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
            .task { transition(active: true) }
            .onChange(of: scenePhase) { _, phase in
                if phase == .active { transition(active: true) }
                if phase == .background { transition(active: false) }
            }
        }
    }
    private func transition(active: Bool) {
        if active && origin != nil { return }
        let previous = lifecycleTask
        let previousClient = client
        let token = UUID()
        generation = token
        if !active {
            client = nil
            origin = nil
            result = ""
            calling = false
        }
        // Serialize complete transitions, including client shutdown. A resume
        // cannot start a host that an earlier background transition then stops.
        lifecycleTask = Task {
            await previous?.value
            if active {
                guard generation == token else { return }
                let address = await host.start()
                guard generation == token else { return }
                origin = address.flatMap(URL.init(string:))
                failed = origin == nil
                if let origin {
                    client = RemoteClient(origin: origin.absoluteString,
                        transport: URLSessionTransport(configuration: .ephemeral))
                }
            } else {
                await previousClient?.shutdown()
                await host.stop()
            }
        }
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
// No Go render-pool initialization or shutdown wait executes on the UI actor.
private actor LocalHost {
    func start() -> String? {
        guard let bytes = SKGoHostStart() else { return nil }
        defer { free(bytes) }
        return String(cString: bytes)
    }
    func stop() { SKGoHostStop() }
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
