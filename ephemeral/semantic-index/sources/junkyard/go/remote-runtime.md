# junkyard Go runtime: `remote` package

Source root: `junkyard/go/remote/` — `remote.go` (handler + manifest), `codec.go` (devalue subset), `typed.go` (generic `Query/Command/Live` wrappers), and tests. Stdlib only. Header says it implements "the query, command and query.live subset of SvelteKit 3.0.0-next.25" (`junkyard/go/remote/remote.go:L1-L2`).

## Purpose

An `http.Handler` that owns SvelteKit's whole remote prefix (`<base>/<appDir>/remote/`), dispatches by Kit's hashed id to Go handlers, and speaks Kit's wire format: base64url-encoded devalue payloads in, `{"type":"result","data":<devalue string>}` JSON out, SSE frames for `query.live`. The typed layer adds JSON-schema validation and turns `any` trees into named Go types.

## Key concepts

### Wire format and dispatch (`remote.go`)

- **`Manifest`** mirrors the Vite capture (`schemaVersion, mode, base, appDir, digest, mappings[{module, exportName, kind, id}]`) — `junkyard/go/remote/remote.go:L14-L31`. `NewHandler` requires schemaVersion 1, mode `dev|production`, non-empty digest, and *exactly one* binding per mapping (matched by `module#exportName`), with kind agreeing with which of `Query/Command/Live` is set — `L62-L104`. Mapping `id` must be `<segment>/<exportName>` — `L87-L89`.
- **Prefix** = `base + "/" + appDir + "/remote/"` with validation of slashes — `remotePrefix` `L106-L114`. The handler 404s anything outside the prefix or with an unknown id, so unknown ids never reach a domain handler (`L116-L126`, test `remote_test.go:L59-L63`).
- **Headers**: `x-sveltekit-version: <digest>` when configured, `cache-control: private, no-store` — `L127-L130`.
- **Query = GET with `?payload=<base64url devalue>`; Command = POST with JSON body `{"payload": "<base64url>", "refreshes": ["<id>/<payload>", ...]}`** — `L131-L151`. Method mismatch → 405.
- **Query response**: `data = {"_": value, "q": {"<id>/<payload>": {"v": value}}}` — `L161-L164`. **Command response**: `data = {"_": value}` plus, when the command called `RefreshRequested()`, `"r": true, "q": {<requested key>: {"v": ...}}` re-executed server-side after the command — `L166-L171`, `refresh` `L184-L230`. This reproduces Kit's `collect_remote_data` behaviour (comment `L184-L185`).
- **Refresh keys** are `<hash>/<export>/<payload>`; parsing splits on the first two `/` — `L196-L201`. With a `RefreshAllowed` set (typed commands), unknown or non-allow-listed keys are silently skipped (Node-owned queries may legitimately appear); without one (untyped bindings), an unregistered key is an error — `L202-L214`.
- **Errors** are always HTTP 200 with `{"type":"error","error":{"message":...}}` — `writeError` `L266-L268`. There is **no redirect support and no HTTP status mapping** in this subset.
- **`query.live`** is served as `text/event-stream`; each yielded value is devalue-encoded, duplicate consecutive frames are suppressed, frames are `data: {"type":"result","result":<devalue>}\n\n`, and a handler error after the stream started becomes a `{"type":"error"}` frame — `serveLive` `L232-L264`. Handler cancellation is via `r.Context()`; test `typed_test.go:L229-L320` proves 3 frames then stop-on-cancel over a real TCP `httptest.Server` (a `ResponseRecorder` is not goroutine-safe for streaming, `L256-L260`).

### Codec (`codec.go`)

- **Devalue subset only**: table-encoded JSON where index 0 is the root; supports `nil, bool, string, finite float64, []any, map[string]any`. Rejects cycles, NaN/Inf, custom types other than Kit's `__skrao`, and any out-of-range index — `decodeDevalue` `junkyard/go/remote/codec.go:L38-L112`. Provenance note: "Source authority: devalue src/{stringify,parse}.js and Kit runtime/shared.js" (`L3-L5`).
- **`__skrao`**: Kit wraps every plain-object remote argument in a devalue custom type `["__skrao", <index>]` (runtime/shared.js `remote_object`); the decoder unwraps it transparently — `L22-L36`, `L64-L79`. Map/Set/Date/File args are named-and-rejected, not mis-decoded.
- **Payload** is `base64.RawURLEncoding` of the devalue JSON — `decodePayload` `L15-L21`.
- **`encodeDevalue`** builds the table depth-first, sorts map keys for determinism, caps depth at 128, and rejects ints outside JS safe range and non-finite floats — `L114-L181`. Output is the JSON string that goes into `data`/`result`.
- Test vector: native `devalue.stringify({path:'document.json',expectedVersion:1,value:'saved'})` = `[{"path":1,"expectedVersion":2,"value":3},"document.json",1,"saved"]` — `remote_test.go:L156-L163`.

### Typed layer (`typed.go`)

- **Author API**: `remote.Query[In,Out]("exportName", func(ctx, In) (Out, error))`, `remote.Command[In,Out]("name", handler, remote.CommandOptions{RefreshRequested: []remote.QueryRef{Other.Ref()}})`, `remote.Live[In,Out]("name", func(ctx, In, yield func(Out) error) error)` returning `QueryBinding/CommandBinding/LiveBinding` values held in package-level vars — `junkyard/go/remote/typed.go:L21-L32`, `L62-L80`, `L114-L125`. Type params are inferred from the handler, which is what the generator reads back (`remotegen.md`).
- **`Bind(module string) Binding`** is what the generated registrar calls: it attaches Kit's module path and wraps the typed handler into the untyped `Binding` — `L40-L60`, `L82-L112`, `L127-L148`. Command `Bind` derives `RefreshAllowed` from the static `QueryRef` list and calls `effects.RefreshRequested()` automatically when the list is non-empty (`L99-L101`); the typed handler never sees a refresh collector.
- **`QueryRef` late-binds the module**: it stores a pointer to the query's `module` field so a `Ref()` taken before `Bind` still resolves after — `L14-L19`, `L36-L38`, `L107`. (Gotcha: this is why `Bind` has a pointer receiver and mutates.)
- **Decode path**: devalue tree → `json.Marshal` → optional `ValidateJSON([]byte) error` (provided by go-gen-jsonschema `--validate` output, interface at `L8-L11`) → `json.Unmarshal` into `In` — `decodeTypedArg` `L150-L167`. Validation runs *before* the domain handler (test `typed_test.go:L89-L121`). **Encode path**: `json.Marshal(out)` → `json.Unmarshal` into `any` → `encodeDevalue` — `L169-L182`. This double marshal is the bridge between Go struct tags/`MarshalJSON` and the tree codec; `time.Time` crosses as RFC3339 (`typed_test.go:L17-L24`, `L79-L86`).

## Citations

- `junkyard/go/remote/remote.go:L62-L104` — `NewHandler` validation
- `junkyard/go/remote/remote.go:L116-L182` — `ServeHTTP` request/response shape
- `junkyard/go/remote/remote.go:L184-L230` — post-command refresh
- `junkyard/go/remote/remote.go:L232-L264` — SSE for `query.live`
- `junkyard/go/remote/codec.go:L38-L112` — devalue decode incl. `__skrao`
- `junkyard/go/remote/codec.go:L114-L181` — devalue encode
- `junkyard/go/remote/typed.go:L150-L182` — validate/decode/encode bridge
- `junkyard/go/remote/remote_test.go:L16-L64` — command + refresh round-trip test with exact expected devalue
- `junkyard/go/remote/typed_test.go:L123-L193` — refresh allow-list intersection semantics

## Reusable verdict

| Component | Verdict | Why |
|---|---|---|
| `codec.go` devalue subset | REUSE AS-IS | Verified against native devalue output; the `__skrao` unwrapping is a hard-won Kit fact. Extend (Date, Map, Set, BigInt, custom `transport` hook types) only when a test demands it. |
| `ServeHTTP` wire shape (GET payload / POST body / result envelope / SSE) | REUSE WITH CHANGES | The protocol facts are gold. Re-verify against the pinned Kit 3 source (the file says next.25) — particularly `q` key shape, `r:true`, and whether errors/redirects now carry status codes or a `redirect` envelope. Add `form` and `prerender` kinds, `redirect`/`error(status)` mapping. |
| Manifest-driven `NewHandler` with strict 1:1 binding check | REUSE WITH CHANGES | Keep the fail-fast validation. If the new design computes ids in Go, `Manifest` becomes a generated Go table rather than an embedded JSON capture. |
| Typed `Query/Command/Live` wrappers + `Bind` | REDESIGN | The brief wants plain Go funcs plus `var _ = skgo.QueryFunction(getUser)` markers; the generator will emit the per-function `http.Handler` wrappers itself. Lift `decodeTypedArg`/`encodeTypedResult` and the `ValidateJSON` hook into the generated code; drop `QueryRef` pointer tricks. |
| `RefreshAllowed` intersection semantics | REUSE WITH CHANGES | The rule "allow-list ∩ client-requested, unknown keys skipped" is right and tested; re-express it in whatever the new command marker offers. |
| `serveLive` SSE + duplicate suppression | REUSE AS-IS | Small and proven over a real socket. |

## Gotchas

- **Untyped bindings receive `any` trees**: numbers are always `float64`, objects `map[string]any` — see `remote-oracle` `main.go:L157-L163` for the resulting casts. The typed layer exists to avoid that.
- **Errors are 200 + `{"type":"error"}`**, never 4xx/5xx; Kit's client expects this envelope, so do not "fix" it to HTTP status codes without checking Kit's client runtime.
- **A command that returns success but whose refresh query errors fails the whole request** (`L166-L175`); the mutation is not rolled back.
- **`Bind` mutates the binding (`q.module = module`)** and must be called exactly once per binding; calling it twice from two registrars would silently retarget `QueryRef`s.
- **`NewHandler` rejects an empty manifest** (`L72-L74`), so an app with zero Go remotes cannot mount the handler — guard in the registrar.
- **Live streams require `http.Flusher`**; wrapping the handler in middleware that doesn't implement `Flusher` yields a 500 (`L233-L237`).
- `encodeDevalue` rejects `int` outside ±2^53−1 and any non-`int`/`float64` numeric Go type (`int64`, `uint` → "unsupported value") — the typed layer avoids this by round-tripping through `encoding/json` first.
- Payload base64 is **RawURLEncoding** (no padding, URL alphabet); std base64 will fail to decode.

## Recipes

- To decode a Kit remote argument in Go: `junkyard/go/remote/codec.go:L15-L21` then `L38-L112`.
- To produce a Kit-shaped query/command response: `junkyard/go/remote/remote.go:L161-L181` (envelope) and `codec.go:L114` (`encodeDevalue`).
- To implement post-command refresh keyed by client `refreshes`: `junkyard/go/remote/remote.go:L184-L230`.
- To stream `query.live` as SSE with cancellation: `junkyard/go/remote/remote.go:L232-L264`, test harness pattern at `typed_test.go:L256-L320`.
- To validate a decoded argument against a generated JSON schema before unmarshalling: `junkyard/go/remote/typed.go:L150-L167`; the generated `ValidateJSON` shape is in `testdata-fixture.md`.
- To write a handler-level unit test with exact devalue expectations: copy `junkyard/go/remote/remote_test.go:L16-L64`.
