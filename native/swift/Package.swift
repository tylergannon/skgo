// swift-tools-version: 6.0
import PackageDescription
import Foundation

let library = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
    .appendingPathComponent("../core/zig-out/lib").standardizedFileURL.path
let package = Package(
    name: "SKGoNative",
    platforms: [.macOS(.v13), .iOS(.v17)],
    products: [.library(name: "SKGoNative", targets: ["SKGoNative"]),
               .executable(name: "skgo-swift-remote", targets: ["RemoteCLI"])],
    targets: [
        .target(name: "CSKGo", publicHeadersPath: "include"),
        .target(name: "SKGoNative", dependencies: ["CSKGo"],
                linkerSettings: [.unsafeFlags(["-L", library]), .linkedLibrary("skgo_native_core")]),
        .executableTarget(name: "RemoteCLI", dependencies: ["SKGoNative"]),
        .testTarget(name: "SKGoNativeTests", dependencies: ["SKGoNative"])
    ]
)
