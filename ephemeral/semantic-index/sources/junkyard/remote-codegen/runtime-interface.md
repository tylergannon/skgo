# Junkyard remote-codegen: the Go runtime contract generated code targets

Purpose: the typed binding layer (`go/remote/typed.go`) the generated registrar consumed, and
the wire behaviour beneath it — devalue payload decoding, JSON bridge, `ValidateJSON`
ordering, command refresh, live SSE, and the Kit-side protocol facts learned while making a
real browser talk to Go. The runtime source itself is in the junkyard `go/remote` package
(outside this segment); this leaf records the contract as documented and the fixes reviews
forced.

## Key concepts and decisions

### Declaration constructors (author-facing)

- `remote.Query[In, Out](exportName, func(ctx, In) (Out, error)) QueryBinding[In, Out]`,
  `remote.Command[In, Out](exportName, handler, CommandOptions) CommandBinding[In, Out]`,
  `remote.Live[In, Out](exportName, func(ctx, In, yield func(Out) error) error) LiveBinding[In, Out]`;
  `CommandOptions{RefreshRequested []QueryRef}`; `QueryRef` is opaque, from `QueryBinding.Ref()`.
  `junkyard/ephemeral/remote-codegen/runtime-interface.md:L7-L28`
- These three are the only symbols the scanner resolves by `go/types` identity
  (`PkgPath == ".../remote"`, names `Query`/`Command`/`Live`). `runtime-interface.md:L26-L28`

### Registrar conversion (generated-code-facing)

- `Bind(module string) Binding` on each binding value takes the Kit-observed module identity
  (the `Mapping.Module` string for every export in that TS module) and returns the pre-existing
  generic `remote.Binding`; the registrar's whole join is a slice of `X.Bind(module)` calls
  fed to the unchanged `remote.NewHandler(manifest, bindings, version)`.
  `runtime-interface.md:L30-L51`
- `Bind` exactly once per binding; it mutates the binding's captured module which any
  `QueryRef` reads lazily by pointer, so command/query bind order is free as long as all binds
  precede serving. `runtime-interface.md:L53-L60`,
  `junkyard/ephemeral/remote-codegen/implementation/runtime-worklog.md:L33-L42`

### What the typed layer does (the JSON bridge)

- Input: decoded devalue argument tree -> `encoding/json` marshal -> `In.ValidateJSON([]byte)`
  when `In` implements it (value receiver, detected via `any(in).(jsonValidator)`) ->
  `json.Unmarshal` into `In`. Output: `json.Marshal(out)` (dispatching to any generated owner
  `MarshalJSON`) -> generic tree -> existing `encodeDevalue`.
  `runtime-interface.md:L64-L68`, `runtime-worklog.md:L43-L45`
- Validation applies to the argument's JSON form, never the devalue envelope; responses are not
  validated outbound. `junkyard/ephemeral/remote-codegen/plan.md:L388-L395`
- Enum/union owner codecs need no runtime change: value-receiver `MarshalJSON` and
  pointer-receiver `UnmarshalJSON` are already what `json.Marshal(out)` / `json.Unmarshal(raw,&in)`
  dispatch to. `runtime-interface.md:L78-L88`, `runtime-worklog.md:L69-L86`
- `time.Time` crosses as an RFC3339 string, not a JS `Date`. `plan.md:L396-L397`

### Command refresh semantics

- `remote.Binding` gained one additive field `RefreshAllowed func() map[string]bool` (nil =
  legacy unrestricted refresh); `Handler.refresh` intersects it with the client's requested keys.
  It is a func, not a set, so `Bind` can build it eagerly without caring about ref-resolution
  order. `runtime-interface.md:L69-L73`, `runtime-worklog.md:L39-L42`
- **Fix from review**: originally an unregistered client-requested key (e.g. a Node-owned query
  the client also `.updates()`) aborted the whole command with "requested refresh is not a
  registered query". Contract is intersection, so when an allow-list is present unknown IDs are
  skipped; nil allow-list keeps the strict error. `runtime-worklog.md:L92-L103`,
  `junkyard/ephemeral/remote-codegen/reviews/sol-runtime-01.md:L29-L45`
- A command with no `RefreshRequested` never triggers refresh dispatch even if the client sends
  keys. `runtime-worklog.md:L25-L26`

### Wire facts (Kit remote protocol at 3.0.0-next.25)

- GET query: argument as devalue table, base64url in the `payload` query parameter. POST
  command: `{payload, refreshes}` JSON body. `junkyard/ephemeral/remote-codegen/typeglue-source.md:L171-L173`
  (cites `go/remote/remote.go:133-146`)
- Decoded tree is `nil | bool | string | float64 | []any | map[string]any`. `typeglue-source.md:L174-L175`
- Live: SSE frames `{"type":"result","result":...}`; response encoded with `encodeDevalue`.
  `typeglue-source.md:L181-L184` (cites `remote.go:158-171,212-241`)
- Command refresh results are returned in the same response (Kit's `r`/`q` fields), matching
  Kit's `requested(...).refreshAll()` pattern. `reviews/fable-plan-01.md:L46-L49`
- **`__skrao`**: Kit wraps every plain-object remote argument in its own devalue custom type
  (`runtime/shared.js:85`; reducers at 150-163 emit `["__skrao", <index>]`; revivers at 176-178
  are identity). Without decode support every named-struct argument fails with
  `invalid devalue array index`. Fix was decode-only: recognise the tag, follow the index,
  reject sibling tags `__skram`/`__skras`/`__skraf`. `implementation/generator-progress.md:L122-L131`
- Manifest carries `Mode` (`dev`|`production`) and a `Digest` of Kit's transformed client code;
  dev transform appends `import.meta.hot?.accept()` so digests differ per mode.
  `reviews/fable-plan-01.md:L86-L93`
- Handler mounts the whole Kit remote prefix `base + "/" + appDir + "/remote/"`.
  `implementation/generator-progress.md:L86-L90`

### Live handler contract

- Handler owns typed values and honours ctx cancellation; runtime owns argument decoding,
  duplicate suppression, SSE framing, disconnect. Reconnect deps deferred. `plan.md:L108-L112`
- Cancellation proof: original test polled `httptest.ResponseRecorder` from another goroutine
  (a real data race under `-race`); rewritten to a real `httptest.Server`, read SSE via
  `bufio.Reader`, cancel the client request context, observe completion via a `done` channel.
  `runtime-worklog.md:L104-L112`, `reviews/sol-runtime-01.md:L47-L60`

### The one integration requirement handed to the generator

- A binding must never serve traffic for an input type whose registration exists but whose
  generated `ValidateJSON` is missing/stale (panic-stub only). Runtime cannot detect this; it
  is a generation/build-graph check. Scanning must also work before `jsonschema_gen.go` exists.
  `junkyard/ephemeral/remote-codegen/implementation/fixture-handoff.md:L49-L70`

## Citations (bookmarks)

- Full interface doc: `junkyard/ephemeral/remote-codegen/runtime-interface.md:L1-L88`
- Runtime build log + design decisions: `implementation/runtime-worklog.md:L7-L45`
- Refresh fix + race fix: `implementation/runtime-worklog.md:L88-L116`
- Devalue/JSON layering (four steps): `typeglue-source.md:L162-L191`
- `__skrao` codec change: `implementation/generator-progress.md:L122-L131`
- Five typed tests as a spec: `implementation/runtime-worklog.md:L18-L29`

## Reusable verdict against the new brief

| Element | Verdict | Notes |
|---|---|---|
| Devalue payload decode -> JSON -> `ValidateJSON` -> `Unmarshal`; marshal -> tree -> devalue encode | KEEP | Layering is correct for Kit's transport and independent of SSR. Only question: whether polytype now exposes an importable validator so the JSON round-trip through bytes can be skipped (old provider was CLI-only, `typeglue-source.md:L23-L30`). |
| `__skrao` handling and rejection of `__skram`/`__skras`/`__skraf` | KEEP | Mandatory for any struct argument from a browser. Re-verify tags against the pinned Kit `runtime/shared.js`. |
| Static refresh allow-list intersected with client keys; unknown keys skipped | KEEP | Semantics survive marker functions; only the declaration syntax changes. |
| `Bind(module)` mutating binding + lazy `QueryRef` pointer | REDESIGN | Existed only because binding values were package vars. With marker functions the generator emits wrapper code that knows the module/hash directly; no mutable binding needed. |
| `Manifest{Mode, Digest}` and `version` parameter | KEEP WITH CHANGES | Mode/digest existed to mirror a co-running Kit Node server. With no sidecar, decide what (if anything) Kit's client sends that Go must echo (protocol version header) and drop the rest. Cannot be settled from this segment; read the junkyard `go/remote/remote.go` (other index worker). |
| Live via SSE, ctx cancellation, `httptest.Server` test pattern | KEEP | |
| `time.Time` as RFC3339 string | KEEP WITH CHANGES | Fine for JSON-shaped types; if the new design wants JS `Date`, that is a devalue-level mapping the old codec explicitly excluded (`plan.md:L396-L397`). |
| Handlers receive only `ctx` + named `In`, return `(Out, error)` | KEEP | Matches the marker-function model (`skgo.QueryFunction(getUser)` on a plain func). |
| Error/redirect mapping | NOT COVERED | This segment documents no `error`/`redirect` mapping to Kit's remote error envelope; handler `error` presumably became a generic failure. Must be designed fresh (Kit `error()`/`redirect()` semantics for remotes and loads). |

## Gotchas and traps

- `ValidateJSON` is a value-receiver method on the stub; detect on the value type, not pointer.
  `runtime-worklog.md:L43-L45`
- `json.Unmarshal` must target `&in` (pointer) so generated pointer-receiver `UnmarshalJSON`
  dispatches. `runtime-worklog.md:L77-L83`
- Do not error on client-requested refresh keys you do not own. `reviews/sol-runtime-01.md:L29-L45`
- `httptest.ResponseRecorder` is not concurrency-safe; streaming tests need a real server.
  `reviews/sol-runtime-01.md:L47-L60`
- The registrar recomputes the mount prefix because `Handler` lacks `Prefix()`; expose one.
  `implementation/generator-progress.md:L86-L90`

## Recipes

- To write typed adapters over a generic binding: start at `runtime-interface.md:L62-L76`.
- To reproduce the devalue/JSON bridge step by step: `typeglue-source.md:L169-L184`.
- To handle Kit's argument wrapper tag: `implementation/generator-progress.md:L124-L131`.
- To test command refresh intersection (three keys: allowed, known-disallowed, unregistered):
  `implementation/runtime-worklog.md:L99-L103`.
- To test live cancellation without a race: `implementation/runtime-worklog.md:L104-L112`.
