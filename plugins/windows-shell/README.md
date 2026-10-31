# Windows-shell plugin

This independently versioned development plugin generates C# remote clients,
exports the C#/Zig SDK, and creates a Windows 11 x64 WinUI 3 shell with WebView2.
The existing `native-shell` plugin owns Swift and Apple tooling.

Install a prebuilt `windows-shell` artifact for the exact official SKGo host with
`skgo plugin install --prebuilt ARTIFACT --sha256 SHA256`, supplying one
`--select 'src/routes/value.remote.ts#readValue'` per selected ordinary query or
command. `--set out=windows/Generated.cs` overrides the generated file path.
Run `skgo generate --plugins` with the application's normal generation options.
Both shell plugins may be pinned together; each owns its own generated files.
No language switch is needed.

`skgo plugin windows-shell sdk` exports to `windows/SDK`; `--out PATH` selects a
project-relative destination. The destination is disposable SDK output. Keep
application code and generated bindings outside it. Exports preserve compiler
caches and unchanged input timestamps, and reject authored or peer-owned SDK
directories. Apple SDK export remains `skgo plugin native-shell sdk` at
`native/SDK`.

See [the C# SDK instructions](sdk/csharp/README.md) for .NET setup and client APIs.
Both plugins depend on the same explicit `native-client` library version. The
common library maintains Zig/C ABI and model validation once; each plugin
bundles the resources it needs. It is not a third installed plugin.

The Go workspace supports development from this checkout. To build artifacts,
use `cmd/skgo-plugin-build` with this module as `--source`, the intended
executable as `--host`, `--version-symbol main.SkgoVersion`, and an explicit
`--plugin-version`. The existing host builder and loader enforce host identity
and qualify installed peers. The development coexistence check uses a local
versioned host, not certification for an already published executable.

## Native shell

Run `skgo plugin windows-shell setup --identity Company.Application` in your
application. The equivalent add-on is `skgo add --set identity=Company.Application
windows-app`. Setup creates `windows/SKGoShell.csproj`, an
application-owned native `ApplicationView.cs`, WebView components and build
scripts. Setup checks for conflicts before writing and preserves authored files
on repeat calls with the same identity. Generation owns only `Generated.cs`;
it does not rewrite the view, project, manifest, or application scripts.

On macOS or Linux, run `skgo plugin windows-shell prepare`. This exports the
pinned SDK and cross-compiles its Windows x64 DLL using mise and Zig 0.17.0.
Run `mise install` in the exported `windows/SDK` first if the pinned Zig toolchain
is unavailable. Preparation does not compile the WinUI project or start a Go
server. The generated app does not load a Go plugin.

Transfer `windows/` to the Windows machine, including the ignored `SDK/` and
`native/` directories. Exclude `bin/`, `obj/`, `build/` and compiler caches.
Git alone does not transfer the ignored SDK or DLL. Use SSH/SFTP or your existing
file-transfer route; Windows and development-machine filesystems are separate.

On Windows 11 x64, install .NET SDK 10 and Microsoft's winapp CLI (tested with
10.0.403 and 0.7.1). The project pins WindowsAppSDK 2.6.4 and Windows SDK
BuildTools 10.0.28000.2957; NuGet restores their build tools. The C# path does
not require Visual Studio. WebView2 also requires the Microsoft Edge WebView2
Runtime on the target. Run `powershell -ExecutionPolicy Bypass -File
windows/build.ps1`, or add `-Action Run` from an interactive desktop session.
Use `-Configuration Release` for a release build. PowerShell scripts check native
exit codes explicitly.

The project bundles .NET and Windows App SDK runtime files. `build.ps1 -Action
Package -Certificate PATH` creates a signed MSIX in `windows/build/`. Supply a
certificate whose publisher matches `Package.appxmanifest`; its default
publisher is `CN=SKGoDevelopment`. For local testing, use `winapp cert generate
--manifest windows/Package.appxmanifest` and trust that development certificate
with `winapp cert install` on the test machine. Certificates stay outside Git.
Distribution signing, Store submission and automatic updates are application
responsibilities.

SSH builds work from macOS or Linux, but MSIX installation, GUI launches and UI automation must
execute in the logged-in Windows desktop session (for example, an interactive
scheduled task). Inspect the actual window through RDP or the host's desktop
viewer. A successful compilation is not a UI demonstration.

## Application boundaries

`ApplicationView.cs` owns the native controls. Reference the generated
`GeneratedRemote.NativeAPI` with the shared SDK's `RemoteClient` and an
application-owned `HttpClient` configured with `AllowAutoRedirect=false`.
The native API origin and the WebView's `WebOptions.InitialPage` are independent.
The shell does not require a local Go server for a client-only application.
The supported query, command and model subset remains the C# SDK's subset.

`ApplicationWebView` accepts an initial HTTP(S) page and additional allowed
navigation origins. With no additions, only the initial origin is allowed.
Origins compare parsed scheme, IDN host and effective port. Paths are not part
of an origin. Userinfo and non-HTTP(S) destinations are rejected. Every main-frame
navigation, including redirects, is checked. Allowed new-window requests load
in the existing view; other requests are cancelled without launching an external
browser. This is a top-level navigation policy, not a subresource network filter
or a TLS trust override.

`MessageOrigins` is independent and empty by default. `Message` delivers raw JSON
only when both the sender and current page have trusted origins. The app must
validate its own message schema; the shell supplies no privileged RPC methods.
Origin trust includes any same-origin document able to send a message; it does
not distinguish paths or authenticate the author of page content. Navigable
external pages do not receive native API headers or authority simply by being
allowed to navigate.

Subscribe to `Configure` before the control loads to add application-owned
document scripts and WebView2 handlers before the initial navigation. Treat
injected scripts as page code: never put native credentials into them.
`NavigationBlocked` and `Failed` report navigation and engine failures.
`RequestObserved` exposes request URI, method and resource context. It does not
promise request/response bodies, complete traffic capture or coverage of native
HttpClient traffic. Applications can use WebView2's developer tools and APIs for
additional inspection. WebView2 profile storage follows the platform default;
the application owns its retention and sign-in policy.

## Synthetic running example

From this repository, `go run ./plugins/windows-shell/example --prepare
.local/windows-example` prepares a consumer using the same shell setup,
generated typed bindings and Windows DLL. Cross-compile its synthetic service
with `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o
.local/windows-shell-example.exe ./plugins/windows-shell/example`.
Transfer the prepared `windows/` directory and service executable to Windows,
start the service, and run the shell from its desktop session.

The example uses independent loopback ports: native API 48321, browser 48322.
Its native query reads `Windows native query`; the Save button sends
`Windows native command` and displays a fresh confirmed query result. A
synthetic bearer credential stays in native HttpClient configuration. The
browser service rejects any Authorization header. An application document script
updates the page and sends a synthetic event to a native label. Links exercise
redirects to the separately allowed `localhost:48322` origin, popup reuse and
rejected navigation. That second origin is allowed to navigate but cannot send
native messages. This service is a fixture for
the example, not a mandatory local bridge for consumer applications.
