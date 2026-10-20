import SwiftUI
#if os(macOS)
import AppKit
#endif

@main struct SKGoApplication: App {
    #if os(macOS)
    @NSApplicationDelegateAdaptor(ApplicationDelegate.self) private var appDelegate
    #endif
    @StateObject private var host: NativeHost
    @StateObject private var application: ApplicationState
    @Environment(\.scenePhase) private var scenePhase
    init() {
        let host = NativeHost()
        _host = StateObject(wrappedValue: host)
        _application = StateObject(wrappedValue: ApplicationState(host: host))
    }
    var body: some Scene {
        WindowGroup {
            if let url = host.url {
                ApplicationView(state: application, url: url, recoveryRevision: host.recoveryRevision)
                    #if os(macOS)
                    .frame(minWidth: 360, maxWidth: .infinity, minHeight: 400, maxHeight: .infinity)
                    #endif
            } else {
                Text(host.failure ?? "Starting application…").padding(24)
            }
        }
        .onChange(of: scenePhase) { _, phase in
            if phase == .background { host.didEnterBackground() }
            else if phase == .active { host.didBecomeActive() }
        }
        #if os(macOS)
        Settings { ApplicationSettingsView(state: application) }
        #endif
    }
}
#if os(macOS)
final class ApplicationDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        NSApp.setActivationPolicy(.regular)
        DispatchQueue.main.async { NSApp.activate(ignoringOtherApps: true) }
    }
    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool { true }
    func applicationWillTerminate(_ notification: Notification) { NativeHost.active?.shutdown() }
}
#endif
