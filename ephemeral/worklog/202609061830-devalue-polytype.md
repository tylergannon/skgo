# devalue moves to polytype

correction to the brief's premise: the two runtimes are not "similar and not
identical" in any way that mattered. `polytype/devalue` at v1.0.0-rc.10 is
line-for-line the same port as the deleted `internal/devalue`, plus a `uint8`
case in `asFloat` and some `//nolint:staticcheck` comments on error strings that
reproduce devalue's own capitalisation. Exported API is identical, symbol for
symbol. The migration was an import rewrite; nothing needed a shim, and no shape
failed to express. If the packages diverge later this paragraph is the thing
that stops being true, so check the diff before assuming it still is:

    diff -ru internal-devalue-at-252e2ce \
      "$(go env GOMODCACHE)/github.com/tylergannon/polytype@<ver>/devalue"

trap: the module cache. `go get` writing `v1.0.0-rc.10` into go.mod proves
nothing about what compiled. `go list -f '{{.Dir}}' github.com/tylergannon/
polytype/devalue` prints the directory the build actually read; that is the
check worth doing, and it is the only one that would have caught a stale cache.

fact: `just generate` under rc.10 produces byte-identical output to rc.9 — the
JSON Schema projection for remote-function arguments is unaffected by the bump.
Verified by regenerating into a clean tree and getting no diff.

## the anchors, and why there are two

`internal/remotearg/kitgolden_test.go` pins bytes produced by *kit's own*
`stringify_remote_arg` / `stringify_command_arg`. It moved to the new import
unchanged and still passes: 23 query goldens, 4 command goldens. That is the
conformance statement that matters, and it is anchored to kit, not to whichever
Go package happens to implement the codec.

`devalue_wire_test.go` (new, repo root) is the layer below it: 38 shapes ×
2 directions, pinned to bytes recorded from the pinned JavaScript devalue 5.9.2
itself. It exists because the kit goldens exercise the *argument* direction and
skip BigInt, RegExp, ArrayBuffer and boxed primitives entirely — shapes both
ports document and neither had measured against the real thing from inside skgo.

Recording recipe (the generator stays out of the tree, per AGENTS.md): import
`stringify`/`parse` straight from
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/devalue/index.js`, build
each value in JavaScript, print `{name, devalue: stringify(value)}`, run it with
`mise x -- node` from `example/`. No install step. Transcribe the Go value into
the case by hand — reading the Go value back off the codec is how this test
would turn into polytype agreeing with polytype.

trap that shaped the test: byte-equality round-trip alone (`Parse` then
`Stringify` reproduces the document) is satisfied by any self-consistent codec,
including a wrong one. The emit half — hand-written Go value must produce
devalue's recorded bytes — is what actually bites. Both halves are present;
deleting the emit half would hollow the file out while leaving it green.

## gap found, not fixed here

`example/e2e/playwright.config.ts` sets `screenshot: 'only-on-failure'`, so a
green run leaves no frames to look at — exactly the unexaminable exit code
AGENTS.md says is not evidence. Flipped to `'on'` for this run's screenshots and
reverted, to keep a refactor's diff a refactor's diff. Whoever touches the suite
next should make it permanent.

# transport hook (#18)

Built in the same run on the owner's amendment. The scope note in the original
brief — "do not build #18 here" — was superseded.

## the finding that shaped the whole feature

Registering devalue reducers is not enough and on its own does nothing. skgo's
result path was `json.Marshal` → `json.Unmarshal` into `any` (`encodeValue`),
because devalue refuses an arbitrary Go struct. By the time a reducer ran, a
`Money` was already an anonymous `map[string]any` and there was nothing left to
recognise. **That round trip is the actual cause of #18**, not a missing hook.

So the fix is where results are encoded, not what they are encoded with:

- The adapters (`callAdapter`, `NewLiveQuery`, the form adapter, `NewLoad`,
  `Async`/`Resolved`) now return the **raw Go value**. Encoding moved to
  `Remotes.call`, `Remotes.callLive` and `data.go`'s serializer, which are the
  only places `cfg.Transport` is known.
- `Transport.encodeTree` walks the Go value and leaves a transported value in
  place so a reducer can still see it. It only walks types that *can* reach a
  transported type (`reaches`, cached); everything else still goes through
  `encodeValue` untouched, so an app with no transport is bit-identical to
  before, and that is asserted.

trap this caused: `Parent()` read `n.data` as an already-encoded
`map[string]any`. Two load tests caught it. It now encodes as it merges.

## kit facts that are load-bearing

`runtime/shared.js`: `const all_reducers = { ...encoders, ...remote_fns_reducers }`
— the transport encoders are tried **before** `__skrao`/`__skram`/`__skras`.
Reversed, `__skrao` claims a custom type first (it is a plain object to
`is_plain_object`), the value goes out sorted under no tag, and the refresh key
stops matching the one the client computed. `internal/remotearg.Codecs` puts
them in front for that reason and says so.

`client.js:522` reads `app.hooks.transport` from the **universal** hooks file,
so the browser half is `src/hooks.ts` and nowhere else. skgo's Go half is
therefore `src/hooks.go`, beside it — that is what the generator scans.

## polytype: what devalue/codegen does and does not do

`codegen.Generate` emits a Go encoder/decoder pair per definition, walking the
Go type into the devalue value model. It has **no notion of a transport**: for
`Product{Price Money}` it emits `encProduct` calling `encMoney`, so the nested
custom type is flattened structurally like anything else. Verified by running it
on a fixture, not assumed.

So codegen is used for the **leaf** — `EncodeMoney`/`DecodeMoney` are the
transporter's payload codec, and the strict decoder with its JSON-pointer paths
is worth having — and the outer walk is skgo's. Do not try to make codegen emit
the transported form; it cannot, and the composition above is the reason it does
not need to.

trap: polytype's TypeScript projection is **structural only** (its own skill
says so; there is no external-type escape hatch, no struct tag, no Declare
option). A transported type projected structurally types `price` as
`{ cents: number }`, and `price.format()` then fails `svelte-check` — which it
did, and which is the correct signal. The fix follows a rule the codebase
already had: a struct carrying a `skgo.File` is inlined into the stub rather
than declared, because polytype describes JSON and a File is not JSON. A struct
carrying a transported type is inlined for the same reason, and the transported
type itself is spelled as the class and imported from `src/hooks.ts`.

## for whoever extends this

`Transporter` carries a `reflect.Type`. Kit spells "is this mine?" as `value
instanceof Money` and answers both questions in `encode`; Go needs the type up
front, because `reaches` has to decide whether to walk a value *before* seeing
one. That is not decoration and removing it would silently re-break #18.

## the two breaks, and what each showed

Both new scenarios were broken deliberately and watched go red, with the real
error captured rather than inferred:

- **Server half removed** (`remoteCfg.Transport` unset): the wire drops the tag
  and carries `{"cents":2000}`. Console:
  `TypeError: s(...).price.format is not a function`. That is issue #18's own
  symptom, reproduced from the same page the fix now makes green.
- **Browser half removed** (`transport = {}` in `src/hooks.ts`): the wire still
  carries `["Money",5]` and kit's devalue refuses it —
  `Error: Unknown type Money`. A client with no decoder fails to *parse*; it
  does not quietly receive a plain object. Worth knowing, because it means a
  key spelled differently on the two sides is a loud failure and not a silent
  degradation.

trap for whoever reads a failure screenshot of this page: Svelte's boundary
renders `Internal Error` for both breaks. The message is generic and says
nothing about the cause; the console does. Do not diagnose this page from the
`failed` snippet alone.

## trap: the dev arm replaces the whole config

The suite was green in prod and red in dev on three scenarios, and the cause was
in the example app's own wiring, not in skgo. `NewHandler` sets fields on
`remoteCfg` and then, for the proxied path, does

    remoteCfg = manifest.RemoteConfig("")

which is a fresh value — every field set above it is gone. `Version`, `Dev` and
`CookieOrigin` are re-set inside that arm and so survive by accident of order;
`Transport` set above it did not. The wire silently lost its tag in dev only.

The assignment now sits after the branch. Anything else an app sets on a config
belongs there too, and a config that is rebuilt halfway through a constructor is
a hazard worth removing rather than remembering — a `RemoteConfig` that took the
dev flag as an argument could not have this shape.

This is also the argument for running the suite in both modes rather than
trusting one: the prod run was 37/37 over a page that was broken in dev.
