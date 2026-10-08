# Native SKGo applications with a Zig client core

## Value proposition

Build macOS and iOS applications that combine SvelteKit's main interface with
native windows, menus, controls and on-device voice transcription. Application
services remain ordinary Go remote functions. Native components call generated,
typed Swift APIs rather than maintaining a second application API by hand.

A reusable Zig core owns devalue integration, the SvelteKit remote wire protocol,
request state and native query caching. Most of that behavior can be developed
and tested with Zig outside an Apple application build. Swift supplies the Apple
integration and native UI. This is also a useful first experiment toward a
sibling SKZig; it does not port or replace the SKGo server or its goja renderer.
Build speed and portability are motivations to measure, not demonstrated gains.

## End state

An application developer keeps Go services, a normal SvelteKit frontend, native
Swift source and an editable XcodeGen `project.yml` in one application repository.
SKGo's Go commands generate the application interfaces, build the selected server
payload and Zig library, invoke XcodeGen and `xcodebuild`, and produce the selected
Apple package. No step requires opening Xcode's UI. Xcode's SDKs, signing tools
and simulator tools remain dependencies for Apple builds.

The application has these four supported combinations:

| Target | Local application | Hosted application |
| --- | --- | --- |
| macOS | Swift launches and supervises a bundled Go executable containing the web assets | Native client and WKWebView use a configured HTTPS server |
| iOS | A linked Go host serves the bundled web assets from inside the application process | Native client and WKWebView use a configured HTTPS server |

The same generated Swift functions work in every combination. Local hosts use
loopback HTTP and supply their actual origin to the client and WKWebView.
Hosted builds contain no Go server or local web payload. The central interface
remains a WKWebView running the normal SvelteKit client. Native features include
menus and voice capture during an active recording session; Apple's on-device recognizer
produces text that native code submits through remote commands.

`project.yml`, Swift sources, build descriptions and selected dependency pins
are durable source. `.xcodeproj`, generated workspaces, DerivedData, libraries,
XCFrameworks and application packages are disposable build output outside
`ephemeral/` and are excluded from version control. Generated application API
source follows the existing project's generated-source policy; it is never
hand-maintained. Rebuilding after deleting all Apple project output is supported.

The local iOS combination is a required outcome with an early feasibility gate,
not an already demonstrated capability. A blocker there requires revisiting
that hosting design with the user; hosted iOS cannot silently stand in for it.

## Authority and current evidence

The user's current direction establishes Zig as the native client core and
XcodeGen as the source format. Earlier preferences for a Swift-only core are
superseded. The earlier attached shell notes are historical context: their
synthetic native route ID and browser/native annotations are not accepted
requirements. SKGo's one-process Go server, Go-owned endpoints, embedded goja
pool, build-time-only Node and absence of JavaScript application I/O remain.
Adding this requested Zig/Swift client is an explicit exception to the existing
Go-only implementation-language rule for those client modules; generation and
SKGo build orchestration remain Go. The user also explicitly proposed a Zig
"wire client and query cache and shit" earlier in this conversation. The full
message is retained in `ephemeral/worklog/20261007-zig-native-plan.md`. That
authorizes a native port of the ordinary-query client subset in milestone 5,
including its relevant wire/key/update semantics. It is a second, scoped
exception to "never reimplement kit": it does not authorize replacing Kit in
the WebView, reimplementing Svelte reactivity, or redesigning the Go server.

Planning baseline: SKGo origin/main `89208d3`, Polytype v1.4.0, installed
SvelteKit 3.0.0 and its transitive devalue 5.9.4. The fetched Devalue repository
at `e2d2dc6e5c676172e636f204259ae48c3a7b5027` supplies an allocator-aware Zig
0.17.0 flat codec under `zig/`. Its source and README explicitly leave HTTP,
Kit canonicalization and caching to consumers. This plan does not claim a new
client or native application has run.

Before consuming the codec, milestone 1 makes it a directly installable Zig
dependency in `tylergannon/devalue`. Publish a consumer-ready source archive
with package-root `build.zig`/`build.zig.zon`, the codec, licenses and the required
shared conformance fixtures. The archive must build and run its native codec
tests after unpacking, without a sibling repository or checkout-relative corpus.
Its metadata records the codec revision and devalue pin. Packaging may require
an upstream build/fixture-layout change; it does not require changing the wire
profile. Pin the actual published URL and Zig package hash in SKGo's
`build.zig.zon` and generated consumers. No guessed release URL or placeholder
hash satisfies the milestone. Devalue remains the sole codec source and test
owner; SKGo and applications depend on its package rather than checking in
codec copies. This is an explicit cross-repository prerequisite, not an archive
that exists today.

Tool acquisition follows the existing mise pattern. Native-enabled SKGo and
application checkouts carry `mise.toml` pins for Zig **0.17.0** and
`aqua:yonaskolb/XcodeGen` **2.46.0**. Their `mise.toml` enables the `lockfile`
setting, and the corresponding `mise.lock` carries integrity information.
The fresh-checkout proof verifies that the lock is consulted. Existing Go and web tool requirements remain pinned in their
normal locations. Native commands verify these tools and the selected Apple
SDK instead of falling back to an arbitrary global version. A new developer
starts with mise and a supported, selected full Xcode installation; `mise
install` provides the pinned non-Apple tools. The initial Apple reference
environment is Xcode **26.5**. Signing identities and physical devices are
explicit prerequisites for the proofs that use them. XcodeGen is absent on
this machine today; the implementation milestone must acquire it. A fresh
checkout proof must include tool acquisition, not assume a warm developer PATH.

The codec owns parsed strings and nodes, never borrows retained input, and
requires exclusive access even for some graph reads: `node` and `stringify`
can reorder storage or rebuild indexes. Its documented profile excludes
unpaired UTF-16 surrogates, several built-in tags and async serialization.
Ordinary generated models use the admitted string/number/object/array profile;
unsupported shapes and strings must report explicit errors, not be coerced.
Finite recursively defined model values are distinct from arbitrary cyclic
application models. The generic devalue graph may contain aliases or cycles.

Pinned Kit source establishes:

- Ordinary query: GET, with a base64url UTF-8 devalue argument. Plain-object
  arguments use `__skrao` and UTF-16 key ordering, recursively. Command: POST
  with a JSON envelope containing the differently encoded argument and refresh
  keys. Generic devalue stringification alone does not implement both paths.
- Remote identity comes from Kit's Vite-root-relative module hash and export
  name; generation must agree with the adapter's remotes rather than invent IDs.
- Results use a small JSON envelope around devalue data. HTTP errors and remote
  error envelopes are separate. A remote error can accompany HTTP 200. Model
  JSON codecs or `Codable` are not prerequisites.
- Query identity is function plus canonical argument, independent of page route.
  Native calls can omit page-context headers. There is no synthetic native
  route namespace. Commands carry the actual server origin; origin checks stay
  enabled.
- Refresh updates belong to the receiving client. Sharing an origin does not
  share the WebView's query cache with the native client's cache.
- Kit allows omitted `paths.origin` and derives the self origin from the request;
  SKGo's current scaffold sets an origin explicitly. Dynamic local ports need
  an actual built-application test, not an assumption from configuration alone.

## Component boundaries and repository shape

```text
Go remote signatures -- Polytype grammar -- SKGo generator --> Swift models/calls
                                                               |
Native Swift UI --> Swift async wrapper --> C ABI --> Zig protocol + query core
                                                       |              |
                                                   Zig devalue   transport effects
                                                                      |
                                                        Swift URLSession on Apple
                                                                      |
                                                               HTTP --> SKGo
WKWebView ----------------------------------------------------- HTTP --> SKGo
```

The diagram is the starting architecture to validate, rather than a prescribed
method for an implementer. On Apple platforms the proposed URLSession adapter
executes HTTP effects and owns platform TLS,
cookies and networking integration; Zig owns how a remote request is prepared,
interpreted and applied to client state. Zig is not required to replace Apple's
networking stack to be the core. A standalone Zig CLI transport exercises real
loopback HTTP; deterministic injected transports exercise core state without
Xcode. Hosted HTTPS must be proved through the actual Apple transport. A better
transport or boundary can be adopted on evidence while preserving Zig protocol
ownership, explicit lifetimes, Apple session behavior and the same proof.

One serialized owner controls each core instance and its graph/cache access.
Swift owns HTTP transport; Zig owns protocol encoding/decoding, query keys and
caching. The ABI exchanges serialized byte buffers with explicit buffer
ownership and release. It does not expose Zig's internal graph or require Swift
to manage Zig object lifetimes. Swift copies returned bytes before releasing
them; no borrowed buffer crosses a network await. Request IDs distinguish late
completions from current work. A cross-language object model is excluded unless
a concrete requirement justifies it. Graphs and cache entries remain internal
to Zig and are never assumed thread-safe.

Application types live once in Go and are projected into Swift. Zig operates on
generic devalue graphs and remote descriptors; this initiative needs no
duplicate application-specific Zig model definitions. Unsupported grammar nodes
produce a source-located generation error. Native function selection is an
explicit generation configuration; new Go annotations are not a prerequisite.

Proposed locations identify ownership, not a prescribed implementation recipe:

| Repository/area | Responsibility |
| --- | --- |
| `tylergannon/devalue/zig` | Canonical generic codec, its profile and shared upstream fixtures; publishes the installable source package used by native clients |
| SKGo `native/core/` | Zig remote protocol, request state and ordinary-query cache; its Zig build/test description |
| SKGo `native/swift/` | Swift package and C boundary wrapper; Apple WebView/session/host integration separated from UI |
| SKGo existing generator/scaffolder/CLI | Go-owned Swift projection, native templates and build orchestration |
| SKGo `example/native/` | Editable XcodeGen specification and actual macOS/iOS example source |
| Application `native/` | Its editable `project.yml`, generated API, shared native features and platform entry points |
| Application Go packages | Reusable server assembly, standalone host entry point, small mobile-host entry point and business logic |

## Phases and milestones

Each milestone describes a developer capability and independent evidence of it.
Implementers map the pinned sources themselves and choose the implementation.
Pure core behavior uses ordinary Zig tests, projection and handler behavior use
Go tests, and the Swift boundary uses Swift tests. WebView behavior needs Kit's
client in an actual WebView. No new acceptance runner or evidence framework is
part of this plan. A missing tool or skipped required check is an unmet outcome.

### Phase A — Establish the core and retire platform uncertainty

**1. A Zig CLI calls one real query and command.** Establish the Zig mise pin
and publish/validate the directly installable Devalue package above, then pin
its URL and package hash and
implement the ordinary remote protocol over loopback HTTP. A supplied literal
transcript reaches an existing SKGo handler and a query reads its typed-shaped
acknowledgement. Independent pinned-Kit fixtures cover different query/command
bytes, undefined/no argument versus null, nested objects, numeric property keys,
non-ASCII keys, base paths, HTTP failures and HTTP-200 error envelopes. Key
ordering cases include UTF-16 versus Unicode scalar ordering. This establishes
the first working path without a native UI or generated application types.
The CLI test uses the production handler, not a second server implementation.

**2. Swift calls the same core safely from the command line.** A small C ABI
and Swift async wrapper can build arguments, perform a query/command and obtain
Swift-owned results. The Zig-to-HTTP interface is injectable; Apple execution
uses URLSession. Boundary tests exercise allocation failure, input lifetime,
release, cancellation before/after dispatch, late completion, owner shutdown and
concurrent Swift callers serialized into one core. Cache eviction and lifetime
tests later extend this same boundary. Cancellation does not promise rollback
of a command already received by the server, and commands are never replayed
automatically. Swift-to-handler calls prove literal input and output, not merely
an encode/decode round trip. Depends on 1.

**3. Local iOS feasibility is demonstrated early, alongside milestone 1.** Acquire
the pinned XcodeGen tool above. An XcodeGen-built probe
links the actual Go application and goja renderer through a small mobile-host
boundary, binds loopback and renders a known SKGo page in WKWebView. Its
host/WebView/lifecycle proof can start immediately without the Zig client;
the page can exercise an existing remote function through Kit's own client.
Prove simulator and physical device execution, foreground startup,
suspend/resume, teardown and behavior under memory pressure. The probe
uses an editable XcodeGen spec and temporary project from its first build.
No Node sidecar, fixed known-port shortcut or Go endpoint in JavaScript is an
acceptable substitute. This gate precedes extensive shell/scaffold work. If
device access or signing is unavailable, the device outcome remains pending.
Go's existing iOS `c-archive` build is the initial host mechanism; gomobile is
optional packaging convenience, not a prerequisite. Once milestone 2 is ready,
extend the same probe with a native Zig-core command/query. Only that extension
depends on 2. The complete milestone requires both parts before 6.

### Phase B — Give application developers typed native clients

**4. Go signatures generate usable Swift APIs.** Selected ordinary queries and
commands emit models and calls through Polytype's published grammar and existing
remote identity analysis. A developer edits Go and regenerates, with no matching
Zig model edits. Generated consumer compilation and handler round trips cover
required/optional/nullable fields, enums, unions, collections, fixed lengths,
naming collisions, time strings and finite recursive models. Unsupported
shapes/custom transports fail explicitly until deliberately supported. Exact
IDs use strings; counters use bounded numbers; wider numeric domains require
explicit application conventions compatible with the current Go f64 wire.
Go and Swift checks reject out-of-domain numbers before rounding. No full-range
Int64/UInt64 fidelity is claimed. Native selection does not change the server's
authorization. Depends on 2.

Selecting a remote also adopts the finite numeric contract in that function's
shared Go handler, for Kit browser and native callers alike. Unsafe arguments
produce the existing 400 remote error; unrepresentable application results
produce the existing 500 result-encoding error. Application authors choose
string IDs and bounded counters before selecting an existing API. This is one
application value contract rather than a second client-specific server mode.

**5. Native ordinary queries have reusable state in Zig.** Generated Swift
clients can retain an observable query, share canonical keys, refresh, inspect
loading/value/error and release ownership. Query and command single-flight
updates use pinned Kit's key and fulfillment rules, including explicitly ignored
keys and unhandled requested refreshes. Pin the admitted subset against Kit's
cache, query instance and response implementation before building it; do not
advertise the whole reactive resource API. Explicit native retain/release
replaces JavaScript garbage-collection mechanics. Tests prove shared request
counts against supplied fixtures, refresh races, error state, bounded cache
retention and a result remaining readable after cache eviction. Confinement
must include graph reads. Session or server changes discard cache state and
prevent old completions populating the new context. Depends on 4.

**6. A developer can build both native targets reproducibly.** Integrate the
core, Swift wrapper, generated API and basic shell into SKGo's example and
application templates. One Go-owned build gesture produces macOS and iOS
targets using tracked XcodeGen YAML and pinned Zig/devalue inputs. Generated
projects and target-specific libraries are build output. Cover macOS supported
architectures, iOS device and simulator slices separately; simulator success
does not establish device linkage. Deleting all generated Apple artifacts and
rebuilding must work from a fresh application checkout on the documented Apple
host, starting with mise and Xcode but without preinstalled Zig or XcodeGen.
The application's Zig dependency metadata references the published Devalue
archive by URL/hash. Builds do not depend on a sibling checkout, codec source
copies or an unrecorded local path. Ordinary core/Swift
tests run independently of this application build. Depends on 3 and 4; 5 can
develop in parallel.

### Phase C — Deliver all four deployment combinations

**7. The macOS application is self-contained.** Opening the built app starts
and supervises its bundled server, waits for readiness, supplies the actual
origin to both clients and shows the embedded Svelte page. Closing it shuts down
its helper; failure and restart are visible and controlled. Test two instances,
port conflicts, startup failure, helper crash and writable data outside the app
bundle. Build with the appropriate dynamic-origin configuration and keep Go's
origin protection active. Define the local session/bootstrap boundary so another
local process or a website cannot access private app functions merely by
guessing a port; random ports and Origin headers are not authentication.
Prove rejection and successful native/WebView use against the real packaged
host. Depends on 6.

**8. macOS and iOS can use a hosted server.** The same generated API and shell
run without a bundled Go payload against HTTPS. Coordinate WebView and native
authentication, Set-Cookie updates, logout and session replacement with the
actual cookie stores. Verify TLS failures, server unavailability and reconnect.
Sensitive cookies stay outside UI/JavaScript bridges. Commands are not blindly
retried or followed across redirects. Test an already installed client against
a compatible server update and an incompatible one; consume existing deployment
version signals, define the application's compatibility contract and surface
incompatibility before silently displaying corrupt data. A module rename changes
remote identity; generated IDs do not create a stable public API automatically.
Two clients using different login sessions must not share cached values.
Depends on 5 and 6.

**9. Local iOS becomes a supported application target.** Turn the feasibility
path into packaged mobile-host lifecycle support and app templates. The embedded
Go server, Zig library, assets and Swift UI run on a supported physical device;
the app remains usable after background/resume and repeated start/stop cycles.
Pause foreground-only work during suspension, restore a valid session/origin
and discard stale work after host replacement. The server is not advertised as
running while iOS suspends the application. Repeat the local bootstrap and
WebView/native session proof on iOS. Depends on 3, 6 and the shared local hosting
contract in 7; hosted session handling in 8 supplies shared session behavior.

### Phase D — Prove the native feature and distribute it

**10. Native voice becomes a real application feature.** Swift owns microphone
access and Apple's on-device transcription; Go receives text through generated
commands. Start, pause/stop, interim/final text, sequence numbers and acknowledged
revisions have an explicit application contract. A bounded pending queue and
server-side idempotency handle ambiguous acknowledgements without duplicating
transcript text; this belongs to the application, not generic command replay.
Permission denial, unavailable on-device recognition, locale support, disconnect
and app suspension have usable UI states. Real macOS and iOS runs verify speech
and acknowledgements; deterministic tests use literal text to verify ordering
and replacement semantics. On iOS, continuous recording is subject to the
supported foreground lifecycle; on macOS, verify an active recording session
also works when the app loses focus. An always-on iOS background capability is
not presumed. Depends on 4 and the
corresponding deployment milestones 7–9.

**11. Native actions and the visible page agree.** The example's WebView shows
the literal text accepted from native controls. Choose and prove the app's
existing Kit refresh/live-query mechanism or a narrow native notification that
asks the page to refresh; the native cache is not automatically the page cache.
Test authentication changes, page navigation and recording transitions while
commands complete. Test the real WebView and inspect what it renders, in local
and hosted modes. Core tests also verify native single-flight updates independently
of the page. Depends on 5 and 10.

**12. A fresh developer can ship and run the application.** Complete Go-owned
scaffolding/build commands and application-facing instructions for the four
combinations. Produce a macOS distribution with signed nested helpers and
notarization, and an iOS archive/distribution using the project's signing
configuration. Verify the actual distributed app on a clean machine/device,
including launch, a native query/command, WebView rendering and voice behavior
where supported. Assets must come from the package in local mode; installed
Node, source paths and a development server must not be accidental dependencies.
Release requires the relevant existing Go handler suite, Kit browser suites and
native tests with no required skips, plus looking at the running application.
Record unavailable credentials/devices as unfinished distribution outcomes,
not a passed packaging milestone. Depends on 7–11.

## Execution order and scope

Begin milestone 1 and the independent host/WebView part of milestone 3 now.
Advance to the Swift boundary as soon as the first core path works; then connect
it to the existing iOS probe. The client work should be useful while hosting is
being proved. A failure of the C boundary changes the boundary design; a failure of
local iOS embedding changes that hosting design. Neither is concealed by adding
a new runtime or silently dropping a requested target.

The end state includes ordinary queries/commands and the native query subset
described above. Native forms, a Swift page-load/router framework, query.batch,
native query.live subscriptions, prerender clients, optimistic overrides, a
generic plugin framework, Unix-socket bridges and an SKGo server port to Zig
are separate work. The WebView may continue using the existing full Kit features.
Native support for arbitrary devalue values/custom transports is also separate
from the generated finite-model contract. No upstream Polytype expansion is
assumed necessary; a demonstrated missing grammar boundary would be raised
there separately. Existing browser/Go behavior remains covered by its normal
tests as native capability is added.

## Source anchors

- Installed Kit: `example/web/node_modules/@sveltejs/kit/src/runtime/shared.js`,
  `runtime/client/remote-functions/{shared.svelte.js,command.svelte.js,query/}`,
  `runtime/server/{remote-functions.js,csrf.js}` and
  `exports/vite/plugins/remote.js`.
- SKGo: `internal/gen/codecs.go`, `internal/gen/clients.go`, `internal/gen/gen.go`,
  `internal/remotearg/`, remote dispatch/origin/caller handlers and app scaffolds.
- [Zig Devalue profile and ownership](https://github.com/tylergannon/devalue/blob/e2d2dc6e5c676172e636f204259ae48c3a7b5027/zig/README.md)
  and `zig/src/root.zig` at that revision.
- Published Polytype `grammar/` and `typegrammar/` at v1.4.0.
- [XcodeGen project specification](https://github.com/yonaskolb/XcodeGen/blob/master/Docs/ProjectSpec.md).
- [XcodeGen 2.46.0 release](https://github.com/yonaskolb/XcodeGen/releases/tag/2.46.0)
  and [mise tool acquisition](https://mise.jdx.dev/dev-tools/).
- [Go mobile build/bind tooling](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile).
- [Apple application lifecycle](https://developer.apple.com/documentation/uikit/managing-your-app-s-life-cycle),
  [WebView cookie store](https://developer.apple.com/documentation/webkit/wkhttpcookiestore),
  [on-device speech availability](https://developer.apple.com/documentation/speech/sfspeechrecognizer/supportsondevicerecognition).
