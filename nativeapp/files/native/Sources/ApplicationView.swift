import SwiftUI

// Application-owned slot. Add-ons may replace this exact default file.
@MainActor final class ApplicationState: ObservableObject {
    init(host: NativeHost) {}
}

struct ApplicationView: View {
    @ObservedObject var state: ApplicationState
    let url: URL
    let recoveryRevision: Int
    var body: some View {
        ApplicationWebView(url: url, recoveryRevision: recoveryRevision)
    }
}

#if os(macOS)
struct ApplicationSettingsView: View {
    @ObservedObject var state: ApplicationState
    var body: some View { Text("No application settings.").padding() }
}
#endif
