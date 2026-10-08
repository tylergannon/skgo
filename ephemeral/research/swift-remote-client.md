# Swift as a client of skgo remote functions

Research at skgo `085456e103cc36c28e3a0ff61a19c88815f6b44d`, fetched from
`origin/main` on 2026-10-07. Both this worktree and the root checkout were
fast-forwarded to that revision before mapping behavior.

The question is what a native shell needs to call Go remote functions over
HTTP while a WKWebView continues to run the ordinary SvelteKit application.
This document records the existing behavior, conflicts with earlier requests,
and questions still unanswered. It does not establish a new Swift client as
implemented or a native application as working.

## Sources and scope

- [Polytype issue #162](https://github.com/tylergannon/polytype/issues/162),
  open when read, with no comments: Swift models and JSON codecs.
- [Earlier shell notes](/Users/tyler/.codex/attachments/9abf8bde-945b-40e6-b24d-ad47b2186b26/skgo-native-shell-design.md).
  These are context to examine, not authorization to implement their decisions.
- Installed SvelteKit **3.0.0**, verified against the example's frozen lockfile.
  The lockfile applies `skgo-kit-3.0.0-queue.patch`; that patch changes only the
  postbuild queue's completion behavior, not the remote protocol mapped here.
- Kit's installed transitive **devalue 5.9.4**, resolved through Kit's real
  pnpm package directory.
- Published **Polytype v1.4.0**, downloaded through Go's module machinery.
  Its module metadata identifies tag/commit
  `df6f4b6d9912c73ea3af4212f5b3ce270867bf55`. GitHub's current main and the
  v1.4.0 tag both resolved to that commit during research.

The mapped Kit source is the installed package in this worktree, not the old
inspiration directory or documentation for Kit 2. Polytype was examined from
the published module, not assumed equivalent to a local development checkout.

## The remote wire has three layers

| Layer | Current behavior | Swift responsibility if it calls the existing endpoint |
| --- | --- | --- |
| HTTP | Query uses GET; command uses POST; endpoint includes base, app directory, module hash and export name | Construct the same URL and method; manage session, cancellation and errors |
| Protocol envelope | Command body is JSON containing a payload string and refresh keys; response is JSON containing a result-data string or an error | Serialize and parse a small fixed protocol envelope |
| Application values | Arguments and result data are devalue documents, with argument documents additionally UTF-8/base64url encoded | Encode and decode devalue, then map between admitted values and generated Swift models |

Sources: Kit [argument serialization](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/shared.js:261),
[command client](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/command.svelte.js:42),
[response client](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/shared.svelte.js:115),
and [server dispatcher](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:176).

The JSON envelope does not contain the model as an ordinary JSON object.
Foundation's JSON machinery can handle the envelope and the JSON syntax of the
devalue table. It does not by itself interpret table indices, special negative
sentinels, tagged values or reference sharing. Model-level JSON `Codable`
conformance is neither sufficient for that interpretation nor required by this
protocol. A custom devalue Encoder/Decoder could use Codable as an implementation
technique; direct generated value conversions are another possibility. Neither
technique is prescribed by Kit.

### Query

The URL is `${base}/${app_dir}/remote/${id}` with `?payload=...` when an
argument is present. `id` is `${hash(module_path)}/${export_name}`.
The hash is Kit's hash of the Vite-root-relative remote-module path. Neither
the caller's route nor the location of a Swift file determines that identity.

The query argument serializer sorts plain-object keys using JavaScript's
UTF-16 ordering and wraps canonicalized objects with the `__skrao` reducer.
The serializer also has canonical reducers for Maps and Sets, although those
are outside Polytype's ordinary typed model grammar. The payload is a query
cache key as well as a transport encoding.

Sources: Kit [query client](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/query/index.js:28),
[query proxy/cache identity](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/query/proxy.js:35),
[remote Vite transform](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/exports/vite/plugins/remote.js:104),
and [argument reducers](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/shared.js:105).

An ordinary async Swift function can obtain a query's value over this endpoint.
That capability alone does not reproduce Kit's reactive resource API:
`current`, `loading`, `ready`, `error`, reference lifetimes, overrides and
shared cache entries are implemented in Kit's client. A native calling API
needs to state which of those behaviors it actually supplies.

### Command

Kit's client sends:

```json
{"payload":"BASE64URL_DEVALUE_ARGUMENT","refreshes":[]}
```

The command argument serializer does **not** sort objects or install the query
object reducer. It supports File serialization and rejects arbitrary Promises
and regular expressions. A text-only native call has no requirement to create
File payloads merely because Kit's general serializer supports them.

Commands can mutate server state and write cookies. The browser client exposes
a pending count and `.updates(...)` on the returned promise. Those are client
behaviors, not additional fields required for a basic call with no updates.

Sources: Kit [command implementation](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/command.svelte.js:18),
[command argument encoding](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/shared.js:274),
and [server command restrictions](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/app/server/remote/command.js:65).

### Response and failure

A successful envelope is `{"type":"result","data":"DEVALUE_DOCUMENT"}`.
The decoded document carries the direct function result under `_`; it may also
carry query updates under `q`, live-query updates under `l`, prerender data under
`p`, ignored update keys under `i`, an explicit-update marker `r`, and a redirect.

An error envelope carries `type: "error"` and an error object. Runtime errors
can arrive in an HTTP 200 response. HTTP failures also exist: origin rejection
is a plain 403 JSON message. A client that checks only HTTP success would miss
remote failures; one that assumes every response has `data` would miss transport
failures. Kit also reads `x-sveltekit-version` to detect deployments.

Query redirects trigger browser navigation. The command client treats a
redirect as an error. Swift has no Kit navigation controller, so the disposition
of a query redirect remains an application API question, not permission to
follow a URL or replay a mutation automatically.

Sources: Kit [response decoding and update application](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/shared.svelte.js:115),
[server result/error branches](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:324),
and skgo [origin check and dispatch](/Users/tyler/.codex/worktrees/4dc0/skgo/remote.go:732).

### Bytes executed from the current installed Kit source

Kit's own `stringify_remote_arg` and `stringify_command_arg` were executed in
Node with empty application transport maps. Only module import locations and
the virtual transport module were supplied; the serializers were not rewritten.

| Input | Query's decoded payload | Command's decoded payload |
| --- | --- | --- |
| No argument (`undefined`) | Empty payload | Empty payload |
| `null` | `[null]` | `[null]` |
| `{text: "hello"}` | `[["__skrao",1],{"text":2},"hello"]` | `[{"text":1},"hello"]` |
| Object inserted as `{z: 1, a: 2}` | `[["__skrao",1],{"a":2,"z":3},2,1]` | `[{"z":1,"a":2},1,2]` |

For the transcript object the command payload is
`W3sidGV4dCI6MX0sImhlbGxvIl0`; the query payload is
`W1siX19za3JhbyIsMV0seyJ0ZXh0IjoyfSwiaGVsbG8iXQ`.
These are different documents for the same application value. Base64-encoding
ordinary `{"text":"hello"}` would produce neither valid Kit payload.

## Caller context is not a native route namespace

The earlier notes say the client sends route information and propose using the
same field for a synthetic native route ID. The current implementation sends
**pathname and search headers**, not a route ID or a params object:

```text
x-sveltekit-pathname: /items/42
x-sveltekit-search: ?view=details
```

Kit reconstructs the event URL from those headers, then matches the pathname
against its route manifest to derive `event.route.id` and params. With the
pathname header **absent**, Kit skips route resolution; the event starts with
`route.id = null` and empty params. A pathname that matches nothing does not
create a new route ID. An actual matched page is not required to dispatch a
remote endpoint.

Sources: Kit [caller headers](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/shared.svelte.js:96),
[absent-header handling](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:166),
[initial event](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:205),
and [route matching](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/respond.js:370).

skgo now mirrors this with a generated application-wide Params domain for
commands/forms and manifest-owned caller matching. Its query event forbids URL,
route and params access. Query arguments must contain any page-dependent input;
the cache is keyed by function and argument rather than caller route.

Sources: skgo [caller matching](/Users/tyler/.codex/worktrees/4dc0/skgo/remote_caller.go:112),
[request URL normalization](/Users/tyler/.codex/worktrees/4dc0/skgo/middleware.go:271),
and Kit [restricted query event](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/app/server/remote/shared.js:123).

There is consequently an existing representation for a caller with no page.
Sending the root pathname instead would mean “called from the matched root
route,” which is a different statement. If a native control acts on a document,
the document ID can be an explicit argument. If a native control genuinely acts
on the displayed page, supplying that real page's pathname/search follows the
existing protocol. Native component provenance would need its own explicit
application argument or metadata contract; Kit defines no reserved native route
ID namespace. Caller-supplied provenance is not authentication.

## Swift and the web view are distinct consumers

A command's single-flight updates travel in **that command's response**.
Kit applies them to maps held by the receiving JavaScript client. A native
URLSession request does not pass its response through that client. Sharing a
server process, application instance, origin or cookies does not share the
JavaScript query cache.

Kit's command client does not universally call `invalidateAll` or `refreshAll`
after success. With `refreshes: []`, the native caller has requested no query
instances to update. The Go handler may still explicitly refresh a query and
place its value in the response; receiving it in Swift does not deliver it to
the web view.

Sources: Kit [response-local updates](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/shared.svelte.js:139),
[command completion](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/command.svelte.js:66),
and skgo [command response collection](/Users/tyler/.codex/worktrees/4dc0/skgo/remote.go:891).

The notes' one-way native-to-Go flow is sufficient for uploading text and
receiving acknowledgements. If the central page must change when the server
accepts text, the mechanism remains unspecified. Existing Kit concepts include
a page-owned live query or an explicit page refresh. A shell notification that
asks the page to refresh is another application integration, not an automatic
property of remote commands. No WebSocket is required merely to submit text;
`query.live` already uses SSE when server-to-client observations are needed.

Apple exposes a [WebKit cookie store](https://developer.apple.com/documentation/webkit/wkhttpcookiestore)
for the web view and a [URLSession cookie-storage setting](https://developer.apple.com/documentation/foundation/urlsessionconfiguration/httpcookiestorage)
for HTTP sessions. The source research and HTTP tests here did not establish
shared authenticated state between them. That needs an explicit lifecycle and
a native execution test if the application relies on it.

## Loads, forms and other remote kinds

| Surface | What Kit owns | Connection to the stated native use |
| --- | --- | --- |
| Ordinary query | Argument-keyed retrieval and browser resource/cache behavior | Native controls can request typed values without inventing routes |
| Command | Mutation, result, cookies and response-local updates | Start/stop recording and submit transcript text fit the mutation surface |
| Server/universal loads | Route branch, inherited data, tracked dependencies, navigation invalidation and optional streamed data | The WKWebView already runs Kit's client; native chrome has no corresponding page branch identified |
| Remote form | Form controls, validation issues, enhancement, multipart/binary submissions and unenhanced fallback | No native form requirement was supplied |
| `query.batch` | Coalesced payloads and one result per requested argument | No native batching requirement was supplied; this is not a sequence of commands |
| `query.live` | SSE, reconnection and reactive live resources | Could serve a separately identified observation requirement |
| `prerender` | Build-time inputs and static remote responses | No native static-query requirement was supplied |

Sources: Kit [remote dispatcher](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/remote-functions.js:204),
[data branch rendering](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/data/index.js:21),
and [live client](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/client/remote-functions/query-live/index.js).

An HTTP caller can request a route's data endpoint, but that is not evidence
that native components need a load lifecycle. The existing load protocol is
broader than a typed function call and carries branch/navigation concerns the
native request did not name.

## What the published Polytype interfaces provide

[The published grammar API](/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.4.0/grammar/grammar.go:32)
exposes `Load`, `LoadWithConfig`, `Types`, and `Lower`. Caller-selected roots
can use `go/types` values obtained from the consumer's analysis. Lower returns
their reachable definitions and a node for each root. It does not require a
CLI projection or a Declare marker on each selected model.

[The published grammar](/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.4.0/typegrammar/grammar.go:1)
retains resolved field names and documentation, field presence, numeric kinds,
enum mode/members, field-local union discriminators/tags, collection lengths,
named references and productive recursion. `ValidateWithRoots` admits the
graph plus selected roots. A new consumer can walk these exported nodes;
unknown future kinds must be rejected explicitly.

skgo already uses those APIs to produce its
[Go devalue codecs](/Users/tyler/.codex/worktrees/4dc0/skgo/internal/gen/codecs.go:116).
The current integration merges definitions from selected roots across packages
and preserves an ordered root list. The generator's
[package loader](/Users/tyler/.codex/worktrees/4dc0/skgo/internal/gen/packages.go:12)
uses `LoadWithConfig` when an overlay is needed.

There is no Swift projection in the inspected published module. Its existing
devalue code generator emits Go, not Swift. The reusable public boundary is the
grammar graph, not the Go emitter's private templates or internal builder.
A skgo-owned Go generator can consume the graph and emit Swift without importing
Polytype internals or adding a public Polytype interface first. This establishes
an available integration boundary, not a completed Swift projection.

The local Polytype skill's reference text still says recursion is refused and
references form a DAG. That description is stale for v1.4.0: named productive
recursion is admitted by the published grammar. The inline constructor graph
remains finite, and this does not imply cyclic application values are encodable.

## Model semantics differ from a general JSON backend

| Concern | Current skgo/Polytype devalue boundary |
| --- | --- |
| Required field | Must exist and be non-null/non-undefined |
| Optional | Missing or non-null value; present zero/empty values survive |
| Nullable | Required property with null or a value; missing is an error |
| Enum | Membership checked, with resolved field-local value/name modes |
| Sealed union | Object with the resolved discriminator and tag; field-local identity matters |
| Ordinary slice | Nil Go slice encoded as empty array |
| Fixed array | Declared length retained and enforced by typed decoding |
| Pointer | Does not independently admit null; required nil pointer is an encoding error |
| Time | String matching the Go time JSON spelling, not devalue Date or Foundation Date |
| Numbers | Encoder converts every Go numeric kind to float64; no generated BigInt mapping |
| Floating-point specials | devalue has NaN, infinity and negative-zero sentinels; generated float decoding admits non-finite values |
| Unknown properties | Typed model decoder rejects them; the remote response envelope additionally contains framework fields |
| Maps, arbitrary interfaces, byte slices | Not ordinary portable grammar shapes merely because devalue has richer runtime values |
| Custom transport | Kit application reducers/revivers determine custom tags/payloads; Go result handling can bypass ordinary structural encoding |

Sources: published [codec contract](/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.4.0/devalue/codegen/doc.go:14),
[numeric encoding](/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.4.0/devalue/codegen/encode.go:18),
[strict decoder helpers](/Users/tyler/go/pkg/mod/github.com/tylergannon/polytype@v1.4.0/devalue/codegen/codec.go.tmpl:77),
Kit [transport initialization](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/app/internal/transport.js:34),
and skgo [transported result path](/Users/tyler/.codex/worktrees/4dc0/skgo/remote.go:160).

Swift's ability to hold a 64-bit integer does not remove the server encoder's
rounding. Exact full-range 64-bit JSON equivalence, requested in issue #162,
does not describe the existing remote wire. Conversely, rejecting every
non-finite float because ordinary JSON cannot carry it would narrow a devalue
contract that has explicit encodings for those values. Both require an explicit
supported-native contract rather than adopting the JSON issue verbatim.

A devalue response table can share a result between `_` and query-update
entries. A native decoder has to understand repeated references even if its
models use value semantics. Generated Go model encoders build fresh value
objects; general cyclic object graphs and every devalue built-in are a
different scope from finite values admitted by the model grammar.

### Concrete native scenarios and candidate numeric conventions

The user specifically asked to examine conventions for the actual use cases
rather than treat full-width integers as a prerequisite. These are candidate
application contracts, not new mappings already supplied by skgo:

| Application value | Convention compatible with the existing wire | What must be checked |
| --- | --- | --- |
| Recording/session/document ID | Ordinary string field; a numeric database ID can be formatted as decimal text before it reaches the remote model | Exact string round trip; no conversion through Double in Swift or Number in the page |
| Transcript sequence or revision | Go uint32/int32 with corresponding Swift numeric type, scoped to a recording | Overflow bounds and reset behavior; browser ordering and Go validation use the same domain |
| A wider integer that still has a bounded domain | Integral number in the JavaScript safe range, at most 9,007,199,254,740,991 in magnitude | Validate bounds before Go encodes the result and before Swift encodes a call; decoding a rounded value cannot recover the original |
| Absolute timestamp | Existing Go time.Time string wire, or explicitly specified text; integer milliseconds are also within the safe range for present-day dates | Preserve specified units, precision and offsets; avoid treating a full Unix-nanosecond count as a safe Number |
| Elapsed recording position/duration | Bounded integral milliseconds, or fractional seconds when approximate numeric precision is suitable | Consistent units; integer bounds or an explicitly stated floating-point tolerance |
| Transcript text, interim/final status and acknowledgement | Strings, booleans and a bounded revision counter | Partial-result replacement versus append semantics, ordering and exactly which revision the acknowledgement confirms |
| Money or another exact decimal, if later needed | Decimal text or bounded integer minor units | Currency/scale and range must be explicit; binary floating point is not an exact decimal contract |
| Truly arbitrary 64-bit integer | Decimal string is an available ordinary field representation | Parse/range-check at the application boundary; preserve the same representation for both browser and native consumers |

For the identified start/stop/menu actions and transcript submissions, no
supplied scenario demonstrates a need for full-range numeric Int64/UInt64
transport. Opaque strings and bounded counters are expressible with today's
ordinary model fields. This need not introduce custom MarshalJSON methods,
which static lowering cannot translate into Swift, or silently change a field's
encoding for native callers alone.

devalue itself has a BigInt tag, but the inspected generated Go codecs do not
map int64/uint64 to it. Turning those fields into BigInt would change the browser
contract as well as the native codec; it is a different feature from applying
the conventions above. Go-side checks matter as much as Swift-side checks: a
large Go result is rounded by the encoder before a Swift client can inspect it.

The remaining domain decision is which concrete signatures use which
conventions. Boundary fixtures would include the largest allowed counter,
the first refused number, a large decimal ID preserved exactly, the timestamp
precision actually chosen, and interim text replaced by its final revision.
Those are scenarios to establish during implementation, not claims already
verified by this research.

## Audit of issue #162 and the attached requests

| Request | Evidence and unresolved scope |
| --- | --- |
| Put Swift models/codecs in Polytype now | The published grammar already allows a skgo-owned consumer. No second Swift consumer was identified in this research. Extraction is not a prerequisite to calling remotes. |
| Use another projection over the existing grammar | Public selected-root lowering exists; a second Go parser or JSON Schema intermediary is not required. |
| Generate ordinary JSON Codable codecs | Existing remotes require devalue model conversion. A general JSON model boundary is a separate capability; JSON envelope handling remains necessary. |
| Public, constructible Swift models and root bindings | A generated typed caller needs usable declarations and a reliable way to refer to the emitted root types. Exact output API is unimplemented. |
| One deterministic Swift file | Determinism and collision safety are testable generator properties. File count has no wire semantics and is not a Kit constraint. |
| Full portable grammar in the first native client | The issue requests a general backend. The identified app needs text/control commands and queries. Which additional shapes its signatures need has not been established. |
| Exact 64-bit JSON integer round trips | Different from existing devalue numeric encoding; cannot be promised by replacing only the client. |
| Time precision and offsets | Existing typed wire uses strings. No need to select Foundation Date semantics for the initial boundary. |
| Recursive named values | Published grammar admits them. Whether initial native signatures use them remains unknown; Swift representation and executed round trips would be required before claiming support. |
| Enum and field-local union identity | Existing wire carries those registrations. Global per-type Codable identity alone may erase field-specific mappings. |
| Negative validation cases | Load-bearing for supported native shapes. JSON-specific constraints must not be mistaken for devalue semantics. |
| Reject unsupported shapes without partial output | Existing generators reject unsupported source contracts. Native support and custom-transport diagnostics remain to be implemented. |
| Publish a Polytype backend release | Required by the issue's proposed ownership, not by the current public-API integration boundary. |
| Native/browser/both opt-in marker | Not a Kit primitive. Client-generation selection and server access control are separate; omitting a Swift stub does not disable the existing browser endpoint. |
| Fold route paths into Swift function names | The remote module path/export already supplies identity. Namespaces or allocated identifiers can distinguish same-named exports without introducing Swift routing. |
| Synthesize native route IDs | Kit derives route IDs from a matched pathname; absence already means no page caller. The proposed reserved namespace has no existing protocol meaning. |
| Include loads in the native surface | No native page-branch/navigation requirement was identified. The web view already consumes Kit loads. |
| Include forms | No native form-enhancement requirement was identified; Kit's form protocol is not needed to POST a command. |
| Continually POST text | Fits discrete mutations. Partial-result revision, ordering, batching, stop semantics and retry/idempotency are application questions Kit commands do not answer. |

## Shell questions that the client protocol does not settle

1. **Actual versus configured origin.** The example config explicitly fixes an
   origin. Kit 3.0.0 itself also permits an omitted `paths.origin` and derives
   self-origin from the request; its config default is undefined. skgo's
   `RemoteConfig.Origin` origin check runs only when populated. Therefore the
   notes' random-port assumption must be tested with the actual skgo composition
   and built assets. An empty skgo origin is not automatically equivalent to
   Kit's request-derived CSRF behavior. A native client cannot solve a server/
   web-view origin mismatch merely by changing its own header.
   Sources: Kit [origin default](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/core/config/options.js:194),
   [self-origin and remote guard](/Users/tyler/.codex/worktrees/4dc0/skgo/example/web/node_modules/@sveltejs/kit/src/runtime/server/csrf.js:19),
   skgo [origin guard](/Users/tyler/.codex/worktrees/4dc0/skgo/remote.go:735).
2. **Native request headers.** Kit requires matching Origin for non-GET remote
   requests, regardless of trusted form origins. Swift must explicitly supply
   the intended origin for commands. That guard is not an authentication
   mechanism for another native process, which can also set a header.
3. **Mutations and uncertain delivery.** A lost command response does not say
   whether Go ran the command. Kit provides no universal exactly-once guarantee.
   skgo's existing Go Form client deliberately avoids replay after redirects;
   this is an existing related boundary, not proof of a Swift retry policy.
   Source: [Form client](/Users/tyler/.codex/worktrees/4dc0/skgo/remote_form_client.go:51).
4. **macOS and iOS startup.** HTTP removes the per-call language bridge once Go
   is running. It does not establish how the native app starts, embeds, packages,
   suspends or shuts down Go. Go's [mobile binding tool](https://pkg.go.dev/golang.org/x/mobile/cmd/gomobile)
   can emit an Apple XCFramework; no skgo build or native lifecycle using it was
   demonstrated here. A macOS child-process plan does not prove iOS packaging.
5. **WebKit/Foundation networking.** Apple documents
   [local networking ATS configuration](https://developer.apple.com/documentation/bundleresources/information-property-list/nsapptransportsecurity/nsallowslocalnetworking).
   A native loopback request, web-view document load and cookie session need
   execution on the intended OS targets; ordinary Go handler tests prove none
   of those platform behaviors.
6. **On-device speech availability.** Apple's
   [SpeechTranscriber](https://developer.apple.com/documentation/speech/speechtranscriber)
   exposes device and locale availability. With SFSpeechRecognizer,
   [requiresOnDeviceRecognition](https://developer.apple.com/documentation/speech/sfspeechrecognitionrequest/requiresondevicerecognition)
   is honored only when on-device recognition is supported. The text-only server
   contract is independent of which supported Apple transcription API the shell
   uses; continuous transcription on the intended devices remains unproved.
7. **UI after native mutation.** Does a transcript acknowledgement alone meet
   the goal, or must the visible web page and native controls immediately show
   the accepted state? Those are different behaviors to demonstrate.

## Evidence gathered and what it does not establish

Executed the current installed Kit serializers for the four literal values
above. The transport map was empty; custom transported models, files and
browser reactive state were not exercised in that execution.

Ran the existing real-handler Go tests for query arguments/results, error
envelopes, command origin checks, command refreshes, no-update responses,
method mismatches, query caller restrictions, refreshed-query restrictions,
and devalue wire encoding/decoding. The selected root-package tests passed
(`0.399s`). Ran all tests in `internal/remotearg`; they passed (`0.258s`).
These checks test existing Go behavior, not a new Swift client. Some historical
wire goldens were recorded against devalue 5.9.2; the new four-case execution
uses the actual installed 5.9.4 and is not a claim that every historical golden
has been regenerated against this pin.

No Swift generation, Swift-to-handler HTTP round trip, WKWebView session,
native microphone use, complete app build, full `just test`, browser e2e suite,
or native UI inspection was performed. Those claims remain open rather than
being inferred from source mapping or passing Go tests.

The eventual first-client behavioral evidence can be stated independently of
implementation: Swift invokes a generated typed text command at the same
endpoint the browser uses; Go receives the literal supplied text exactly once
in the exercised path and returns a typed acknowledgement; a generated typed
query reads the resulting state; no-argument/null and the admitted model shapes
retain their semantics; errors and cancellation are observable. If web-view
updates are part of the capability, the visible page must show the accepted
literal text through the chosen Kit client mechanism. This is a description of
unperformed evidence, not an acceptance runner or a declaration of completion.
