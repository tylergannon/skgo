import SwiftUI
import WebKit

@main
struct NativeProbe: App {
    @Environment(\.scenePhase) private var scenePhase
    @State private var origin: URL?
    @State private var failed = false

    var body: some Scene {
        WindowGroup {
            Group {
                if let origin {
                    Page(url: origin.appendingPathComponent("todos"))
                        .id(origin)
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
                    SKGoHostStop()
                    origin = nil
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
