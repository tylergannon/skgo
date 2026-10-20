import SwiftUI
import WebKit

@MainActor final class ApplicationWebCoordinator: NSObject, WKNavigationDelegate {
    var url: URL?
    var revision = 0
    func update(_ view: WKWebView, url: URL, revision: Int) {
        if self.url != url || self.revision != revision {
            self.url = url; self.revision = revision
            view.load(URLRequest(url: url))
        }
    }
    func webViewWebContentProcessDidTerminate(_ webView: WKWebView) {
        if let url { webView.load(URLRequest(url: url)) }
    }
    func webView(_ webView: WKWebView, decidePolicyFor navigationAction: WKNavigationAction, decisionHandler: @escaping (WKNavigationActionPolicy) -> Void) {
        guard let target = navigationAction.request.url, let url,
              target.scheme == url.scheme, target.host == url.host, target.port == url.port else {
            decisionHandler(.cancel); return
        }
        decisionHandler(.allow)
    }
    func make() -> WKWebView {
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .nonPersistent()
        let view = WKWebView(frame: .zero, configuration: config)
        view.navigationDelegate = self
        return view
    }
}
#if os(macOS)
struct ApplicationWebView: NSViewRepresentable {
    let url: URL
    let recoveryRevision: Int
    func makeCoordinator() -> ApplicationWebCoordinator { ApplicationWebCoordinator() }
    func makeNSView(context: Context) -> WKWebView { context.coordinator.make() }
    func updateNSView(_ view: WKWebView, context: Context) { context.coordinator.update(view, url: url, revision: recoveryRevision) }
}
#else
struct ApplicationWebView: UIViewRepresentable {
    let url: URL
    let recoveryRevision: Int
    func makeCoordinator() -> ApplicationWebCoordinator { ApplicationWebCoordinator() }
    func makeUIView(context: Context) -> WKWebView { context.coordinator.make() }
    func updateUIView(_ view: WKWebView, context: Context) { context.coordinator.update(view, url: url, revision: recoveryRevision) }
}
#endif
