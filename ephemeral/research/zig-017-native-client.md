# Zig 0.17 and a native remote-function client

Research conducted October 7, 2026, in skgo's existing research worktree. This
is an investigation of protocol, memory and scheduling boundaries, with a small
executed interop probe. It is not a remote client implementation or a claim of
SvelteKit cache parity.

## Verified versions and the two I/O changes

The [official downloads](https://ziglang.org/download/) and
[machine-readable index](https://ziglang.org/download/index.json) list **0.17.0,
released October 1, 2026**, and master as 0.18 development. The machine's installed
Zig is **0.16.0-dev.2915+065c6e794**, not either stable release. A separate official
Apple Silicon 0.17.0 archive was downloaded for source inspection and experiments;
its SHA-256 matched the download index:

```text
b607e9b9234790a008116ae5bdb71c6243b84b9fb42a53a9e70fde41c06c536a
```

There are two changes that older examples often conflate:

- **0.15.1:** the new buffered, non-generic `std.Io.Reader` and `Writer` interfaces.
  Their buffer sits above the vtable; codecs can operate on memory streams as well
  as files/sockets. [0.15.1 release notes](https://ziglang.org/download/0.15.1/release-notes.html#New-stdIoWriter-and-stdIoReader-API)
- **0.16.0, released April 13:** `std.Io` becomes the explicit authority for
  blocking/nondeterministic operations, including networking, clocks, tasks and
  synchronization. The release identifies Threaded as its complete implementation
  and Evented as experimental. Arena allocation becomes lock-free/thread-safe.
  [0.16 release notes](https://ziglang.org/download/0.16.0/release-notes.html#IO-as-an-Interface)

0.17 adds `SafeAllocator` and `BufferFirstAllocator`; it does not make allocator
ownership into language-enforced borrowing. The official 0.17 release notes were
read through a direct HTTP fetch because the web tool could not open that page.
[0.17 release notes](https://ziglang.org/download/0.17.0/release-notes.html)

## What allocators establish

The 0.17 `std.mem.Allocator` is an explicit implementation pointer plus vtable.
Allocation returns raw storage; resize/remap/free require matching size and
alignment. It establishes which allocator pays for and reclaims memory, while
leaving the program responsible for the lifetime of each reference. Passing an
allocator is not ownership transfer, and copying a slice does not copy or retain
its bytes. The vtable uses Zig functions and slices: it is not itself a published
C ABI to hand to Swift.
[0.17 Allocator source](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/mem/Allocator.zig#L17-L91)

`ArenaAllocator` groups allocations for bulk destruction. Its source says the
allocator operations are thread-safe **if its child allocator is thread-safe**;
`deinit` (line 51), `queryCapacity` (line 68), and `reset` (line 112) explicitly
are not thread-safe. Arena allocation being safe concurrently therefore does not
make resetting memory under an active Swift reader safe. An arena also does not
run destructors for foreign resources such as retained Swift objects.
[0.17 ArenaAllocator source](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/heap/ArenaAllocator.zig#L1-L114)

**Lifetime implication:** per-operation scratch and per-result storage are
different ownership domains. Request scratch can expire at operation completion;
a cached result may still be in use by a UI consumer after a refresh or cache
eviction. Putting every result in one resettable request/client arena would
invalidate those consumers. A result arena can instead expire when the final
owner of that result releases it. Its backing allocator must outlive the arena,
including any result retained after the client is closed.

`SafeAllocator` supplies diagnostics, rather than a borrow checker: its opening
comments specify leak reporting, allocation-mismatch checks, double-free and
resize/remap/free-race checks. Detection of writes after free has conditions on
the backing allocator; foreign pointers remain unsafe. The same source states
that producer/consumer acquire-release ordering must be supplied externally.
[0.17 SafeAllocator source, lines 1–29](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/heap/SafeAllocator.zig#L1-L29)

For testing, `std.testing.checkAllAllocationFailures` reruns a supplied operation
with each allocation forced to fail and detects swallowed allocation errors and
unfreed memory. The executed probe used it to check a snapshot construction and
retention lifecycle. This is useful for a decoder whose allocation count depends
on the supplied document.
[0.17 testing source, lines 1145–1190](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/testing.zig#L1145-L1190)

## What std.Io establishes for async lifetimes

`std.Io` provides a replaceable execution authority. `async` may run the operation
to completion immediately; `concurrent` permits the caller to progress before
the operation completes and can fail with `ConcurrencyUnavailable`. A future's
`cancel` requests cooperative cancellation **and waits for completion**, returning
the operation's result. `await` and `cancel` are individually idempotent, but the
Future methods explicitly are not thread-safe. Cancellation is acknowledged at
cancellation points, not an unconditional kill or undo.
[0.17 Io source, Future at lines 1295–1325 and async/concurrent at 2526–2590](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/Io.zig#L1295-L1325)

The async vtable documentation says its context is copied. Inspection of the
implementation shows that this is the argument tuple; pointers/slices inside it
continue to point at their original storage. The tuple copy does not deep-copy a
request buffer. Keeping that storage alive through await/cancel creates a useful
completion boundary for `defer` cleanup. Swift `Task.cancel()` does not
automatically cancel a Zig future, and a synchronous joining cancel must not
accidentally block a UI thread.

0.17 aliases `Io.Evented` to Dispatch on supported Apple targets, but that is
not a blanket completion claim. `Io/Dispatch.zig` still contains explicit panic
stubs for batched network operations, for example `operate` at lines 1713–1716.
An application that uses one of these paths will not obtain a graceful fallback.
The probe exercised **Threaded**, not Dispatch.
[0.17 Dispatch source](https://codeberg.org/ziglang/zig/src/tag/0.17.0/lib/std/Io/Dispatch.zig#L1713-L1716)

**Boundary implication:** Zig can own its tasks, clocks and transport and still
be tested without Apple UI tooling. A Swift-hosted HTTP adapter is not required
to make Zig tests possible. It is a distinct integration choice if native cookie,
authentication and URLSession behavior are desired. In that case std.Io does
not automatically turn URLSession into an Io backend. Likewise, a full custom Io
backend is a broader interface than the few operations a remote client needs.

## Community use: adoption is mixed, and versions matter

These sources are maintained projects or firsthand maintainer documentation.
Their architectural use is evidence; this research did not rebuild each project
on Zig 0.17 or independently verify every project claim.

| Project / source snapshot | Observed use | What it establishes here |
| --- | --- | --- |
| Ghostty main `a60e9e2`, October 7; package minimum Zig **0.16.0** | Native Swift application over a Zig C API; libghostty-vt exposes configurable allocation and ownership rules | A substantial actual Swift/Zig integration, with explicit allocator contracts; no evidence from this snapshot of 0.17 compatibility |
| libxev `9ce8e8e`, May 6; README requires **0.16** | Production-used portable event loop with Zig and C interfaces | A maintained library explicitly continuing outside std.Io rather than adopting it |
| TigerBeetle main `c95d7a5`, October 6; build checks exact **0.16.0** | Large systems application with its own established I/O and testing architecture | Existing substantial Zig use is not evidence that 0.17 is already the community baseline |
| Marionette `1659b88`, October 4; package **0.7.2**, Zig **0.16.x** | Alpha deterministic std.Io backend with scheduler, clock, allocation and selected network/file fault injection | Concrete use of replaceable Io for testing actual clients; 0.17 support not established |
| uuid.zig `2369536`, October 5; package minimum **0.17.0** | Explicit std.Io for time/randomness; explicit allocator for owned outputs | A small current adopter of the allocator/Io pattern, not a measure of ecosystem maturity |

Sources:
[Ghostty package pin](https://github.com/ghostty-org/ghostty/blob/a60e9e2a57f73e1eef2bd1cf2995a467f69e7fb0/build.zig.zon#L5),
[libxev README](https://github.com/mitchellh/libxev/blob/9ce8e8e6ff89e583258a7f8e7adeeeaeae8611bf/README.md#integration-with-zig-016-stdio),
[TigerBeetle build pin](https://github.com/tigerbeetle/tigerbeetle/blob/c95d7a53a3d044741409b03b2f334a8ed033fa3c/build.zig),
[Marionette README](https://github.com/sb2bg/marionette/blob/1659b88aa49071094098d6f5f41a9ac6948aa81c/README.md),
[uuid.zig package pin](https://github.com/muhammad-fiaz/uuid.zig/blob/2369536c2a0fecf41371f80568ad1e980d71ebe8/build.zig.zon).

libxev's maintainer explains that its design predates std.Io, changing it would
require substantial API breaks, and the new interface remains unstable and lacks
some required operations. Its README says it does not implement or accept
std.Io. This is a useful qualification to the assumption that a new standard
interface immediately subsumes established community runtimes.
[libxev's explanation](https://github.com/mitchellh/libxev/blob/9ce8e8e6ff89e583258a7f8e7adeeeaeae8611bf/README.md#integration-with-zig-016-stdio)

Marionette's author showcased its deterministic backend on July 7, with
unmodified mailbox and database examples. Current docs list HTTP, PostgreSQL and
Redis client investigations. They also explicitly delimit simulated operations
and exclude preemptive OS-thread scheduling, the CPU memory model and real DNS.
This is evidence for the testability benefit of Io injection; it is not proof of
Swift callback safety or real transport behavior.
[author's Ziggit discussion](https://ziggit.dev/t/marionette-deterministic-simulation-testing-for-zig-built-on-std-io/16560),
[current project status](https://github.com/sb2bg/marionette/blob/1659b88aa49071094098d6f5f41a9ac6948aa81c/README.md#status)

The [community file-I/O tutorial](https://ziggit.dev/t/file-i-o-basics-0-16/14968)
also emphasizes that a returned slice may merely borrow the caller's buffer,
whereas an allocator-backed result requires explicit freeing. The same lifetime
distinction survives at the FFI boundary. These sources show concrete patterns
and maintenance friction, not a representative survey of all Zig users.

## Swift/Zig memory sharing: concrete available patterns

Ghostty's C-facing allocator interface mirrors Zig's flexibility through its own
explicit C vtable and context. It accepts a default allocator, documents
allocation failure, and requires matching allocation/free ownership. A Swift
host could similarly supply raw-storage allocation callbacks; doing so does
not make allocations Swift ARC objects or reconcile incompatible destructors.
This is an adapter over the allocator interface, not exposing Zig's internal
vtable directly.
[Ghostty allocator header](https://github.com/ghostty-org/ghostty/blob/a60e9e2a57f73e1eef2bd1cf2995a467f69e7fb0/include/ghostty/vt/allocator.h)

Ghostty's formatter exposes both a caller-provided output buffer (including a
size query/retry protocol) and an allocator-backed output requiring
`ghostty_free`. That gives two already-used choices: let Swift own the raw output
storage, or let Zig own it and provide the matching release operation.
[Ghostty formatter header](https://github.com/ghostty-org/ghostty/blob/a60e9e2a57f73e1eef2bd1cf2995a467f69e7fb0/include/ghostty/vt/formatter.h#L165-L214)

The actual Ghostty Swift frontend uses `withCString` for scoped inputs and
`defer { ghostty_surface_free_text(...) }` for owned output, converting text to
Swift String before release. It demonstrates conventional ownership handling;
it does not make every returned model zero-copy.
[Swift frontend, lines 2213–2238](https://github.com/ghostty-org/ghostty/blob/a60e9e2a57f73e1eef2bd1cf2995a467f69e7fb0/macos/Sources/Ghostty/Surface%20View/SurfaceView_AppKit.swift#L2213-L2238)

| Pattern | Allocation/copy behavior | Lifetime condition |
| --- | --- | --- |
| Scoped borrowed input/output | No bridge copy for synchronous inspection | The pointer is consumed within its documented scope; no await or escaping pointer |
| Swift-provided raw output buffer | Zig writes directly into host storage | Fixed capacity or size-query/retry contract; Zig retains no pointer after return |
| Owned result with matching release | Zig owns stable storage; Swift may inspect bytes directly | Swift wrapper releases the Zig owner, not a guessed `free`/`deallocate` |
| Retained immutable result handle | Cache and Swift can share one result graph | Replacement/eviction releases the cache's reference; retained old results remain alive until their last release |
| Host allocator callbacks | Zig uses host-provided raw allocation | Alignment, size, callback context, thread rules and matching deallocation are all explicit |

The retained immutable-result row is an **inference for a possible cache design**,
not an implemented Ghostty or skgo API. A mutable query entry and an immutable
snapshot have different lifetimes: a consumer can hold yesterday's result while
the entry points at a replacement. Keeping the snapshot's arena pinned avoids
per-field frees without allowing cache eviction to invalidate a borrowed string.
Cross-thread ownership additionally needs synchronized publication and either
atomic references or an enforced executor. Thread-safe allocation alone does
not make the cache thread-safe. A destructor must not call back into a destroyed
client, or wait on the same executor currently running that destructor.

Swift/Foundation provides `Data(bytesNoCopy:count:deallocator:)`, including a
custom release hook. It can anchor foreign bytes in a Swift-managed owner.
Converting those bytes into ordinary generated Swift Strings, arrays and structs
can still allocate/copy; sharing bytes does not imply identical native layouts.
For direct structured inspection, a C-compatible representation or accessors
would be needed. Arbitrary Zig structs and Swift model memory cannot simply be
reinterpreted as each other.
[Apple Data deallocation API](https://developer.apple.com/documentation/foundation/data/deallocator)

Swift's current safe-interop documentation also describes bounds/lifetime
annotations and imported Span overloads. Those annotations are additional
Swift-side tooling; a Zig allocator does not automatically generate them.
Their application to a proposed generated C header requires a compiler test.
[Swift safe interoperability](https://www.swift.org/documentation/cxx-interop/safe-interop/)

## The query cache remains a Kit semantic question

The installed example package was independently checked as **Kit 3.0.0** after
reading `ephemeral/sveltekit-current/SKILL.md`. Its query proxy keys entries by
remote identity and serialized argument; its CacheController counts proxies and
manual references, then defers eviction through Svelte `tick`. Query instances
also coordinate overlapping responses, error handling and overrides.

Sources in this worktree:
[query proxy](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/query/proxy.js:35),
[cache controller](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/cache.svelte.js:27),
[query instance](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/query/instance.svelte.js:131).

Manual native ownership can express shared entry/result lifetimes. It is not a
literal replacement for JS finalizers, Svelte effect roots and ticks. Protocol
fixtures and explicit response-order scenarios can be tested in Zig; UI
observation and the lifetime of a Swift view remain host integration. A native
cache also does not become the WKWebView's JavaScript cache merely because both
clients call the same remote endpoint.

## Application models need not be duplicated in Zig

The devalue reference table, scalar tags, objects and arrays are a generic wire
representation. Cache identity and entry ownership similarly do not require a
different Zig struct for each application result. **A generic Zig value graph
behind opaque C handles is compatible with having generated application models
only in Swift.** This is a design inference from the wire and ABI, not a
demonstrated generated client.

Such an interface would expose concrete C-compatible operations for traversing
and constructing values: scalar/tag access, borrowed string bytes, object fields,
array members and result ownership. Generated Swift conversions could handle
the grammar's fields, optionality, collections and supported transport types;
the application developer would continue to define the Go type once. Zig could
keep its internal slice/tagged-union representation private. Swift would import
the C header, not Zig generics or native slices/error unions. The probe proves
the narrower opaque-handle plus pointer/length mechanism, without proving those
model converters.
[Zig's C-library interface documentation](https://ziglang.org/documentation/0.17.0/#Exporting-a-C-Library)

The work does not disappear: it becomes **generated traversal/construction code**
and a small concrete ABI. Conversion may make an FFI call per field/element and
allocate Swift-owned Strings/arrays. Conversely, sending a Swift argument needs
a generated walk into the generic Zig graph or another concrete encoder input
interface. Using ordinary model JSON as an intermediate would add another
representation/conversion; it is not required by this approach. Alias/cycle
support in the generic graph does not establish that a generated Swift value
model can preserve arbitrary object identity or cycles. The admitted Go grammar
and transport mappings must specify what generated models represent.

An alternative is to generate both Zig and Swift typed projections from the
existing Go grammar. That duplicates generated representations, not authored
model definitions, provided conversion is also generated. It can enable typed
Zig operations if a requirement calls for them. A generic codec/cache alone does
not establish that requirement. Native Zig types still need concrete C exports;
generating a Zig struct does not by itself make it a Swift-importable model.

Using imported C structs as the only public model can remove a Swift struct
declaration for simple POD cases. Dynamic strings, arrays and optional/tagged
values expose pointer/length/tag ownership machinery instead of Swift's ordinary
String, Array and Optional API. A Swift wrapper restores that ergonomics but
then introduces the conversions and lifetime work again. The byte-sharing probe
therefore does not settle the public typed-model shape.

Immutable opaque result ownership permits a Swift consumer to inspect generic
graph fields without copying the entire result. A borrowed field remains valid
only while its snapshot owner is retained. Producing an independent ordinary
Swift model generally pays for its own dynamic storage; retaining the Zig graph
may then be unnecessary. Which cost matters depends on actual result sizes and
access patterns, rather than the mere existence of an FFI boundary. Neither
alternative requires hand-maintained mirror types or per-model handwritten
mappings; generation coverage and a real generated conversion test remain to
be established.

## Executed experiment

The retained source is deliberately small and contains no remote client:

- [probe.zig](/Users/tyler/.codex/worktrees/4dc0/skgo/ephemeral/research/zig-017-probe/probe.zig)
  creates immutable bytes in a result-owned arena, exports retain/release and a
  borrowed pointer/length, tests all allocation failures, and tests a concurrent
  Threaded task's joining cancellation.
- [probe.h](/Users/tyler/.codex/worktrees/4dc0/skgo/ephemeral/research/zig-017-probe/probe.h)
  is the explicit C surface.
- [probe.swift](/Users/tyler/.codex/worktrees/4dc0/skgo/ephemeral/research/zig-017-probe/probe.swift)
  creates a snapshot from scoped Swift input, destroys that input, releases the
  initial owner, reads the retained bytes, and wraps the 4096-byte block in
  no-copy Data with a custom Zig release.

Apple Swift **6.3.2**, target arm64-apple-macosx26.0, ran from the command line.
No Xcode UI, project, SwiftUI app or WKWebView was involved. The Apple compiler
and Foundation toolchain remain dependencies of this integration test.

These are the actual commands executed from the worktree. The source initially
lived beside the temporary download; after execution only the three probe source
files were moved to the retained paths above. Downloaded toolchain/source and
compiled output were removed rather than tracked.

```text
ephemeral/downloads/zig-017-research/zig-aarch64-macos-0.17.0/zig test ephemeral/downloads/zig-017-research/probe.zig

ephemeral/downloads/zig-017-research/zig-aarch64-macos-0.17.0/zig build-lib ephemeral/downloads/zig-017-research/probe.zig -target aarch64-macos.26.0 -static -O Debug -femit-bin=ephemeral/downloads/zig-017-research/libprobe.a

swiftc -import-objc-header ephemeral/downloads/zig-017-research/probe.h ephemeral/downloads/zig-017-research/probe.swift ephemeral/downloads/zig-017-research/libprobe.a -o ephemeral/downloads/zig-017-research/swift-probe

ephemeral/downloads/zig-017-research/swift-probe
```

Observed output, exit 0:

```text
1/2 probe.test.snapshot retention and every allocation failure...OK
2/2 probe.test.Threaded Io cancellation joins task...OK
All 2 tests passed.

PASS: Swift scoped input; retained Zig arena; unchanged borrowed address; Data custom release
```

The initial library build used the host's implicit macOS minimum, 26.6.2, and
Swift's linker warned that its deployment minimum was newer than Swift's 26.0.
Rebuilding with the explicit 26.0 target removed the warning; that rebuilt
integration was executed successfully. This is evidence for specifying compatible
deployment targets, not evidence for older OS/iOS packaging.

The pointer-equality assertion independently demonstrated an actual no-copy
4096-byte Data wrapper on this machine, not merely correct contents. The probe
uses serialized, non-atomic retain/release and does not establish cross-thread
destruction. Foundation may choose different storage behavior for other sizes or
operations. It does not test a generated model tree, devalue, canonical arguments,
HTTP, command semantics, cookies, cache eviction races, callback re-entry,
Swift concurrency cancellation or the native/web-view update path. No build-speed
comparison was performed.

## What remains to prove for this proposed client

Independent Zig tests can exercise wire fixtures, canonical argument identity,
shared query entries, explicit reference ownership, response ordering and
allocation-failure cleanup. If Zig owns scheduling, clocks and networking,
std.Io makes that authority injectable; selected simulator coverage is possible
but 0.17/backend compatibility must be checked rather than inherited.

Swift integration still has to prove generated value conversion, result
ownership through refresh/eviction/client shutdown, release on the actual
executor, cancellation races, and transport/session behavior. macOS/iOS
deployment and WKWebView remain platform tests. The allocators give an explicit
way to implement memory sharing; Io gives explicit task completion/cancellation
boundaries. Neither supplies the Swift ownership contract automatically.
