# Authored example: what a developer writes for a route-colocated Go remote

**Purpose.** Verbatim-ish record of every file a developer authored in the junkyard's
working colocation fixture (the only end-to-end demonstrated case of Go remote functions
living inside a SvelteKit route tree), plus the shape of what the generator emitted. Use it
as the concrete target for skgo's authored surface before deciding what to change.

All paths relative to the token cache root `ephemeral/inspiration/`.

## Fixture layout (candidate as assembled)

`candidate.mjs:assembleCandidate` builds one Go module whose `app/` subtree is the Kit app
(`junkyard/ephemeral/remote-codegen/proof/candidate.mjs:L14-L52`):

```
<GoRoot>/                          go.mod  (module example.com/skgofixture)
  cmd/generated-candidate/main.go  the Go server binary
  remotes/accounts/                ordinary Go package (enum + union case)
  remotes/notes/                   ordinary Go package with //skgo:remote output= directive
  remotes/unused/                  discovered-but-unimported package
  internal/skgoremotes/remotes_gen.go   GENERATED registrar (not in fixture; emitted)
  .skgo/links/<enc> -> app/src/routes/(group)/remotes/[id]   GENERATED symlink
  .skgo/links.json                 GENERATED link inventory
  app/                             the SvelteKit app (oracle copy + overlay)
    vite.config.ts
    vite/skgo-capture.ts
    .skgo/remotes-source.json      GENERATED Stage-A inventory
    .skgo/remotes-kit.json         GENERATED Kit capture (written by the Vite plugin)
    src/lib/documents/document.remote.go       colocated under src/lib (Query+Command+Live)
    src/lib/documents/document_jsonschema.go
    src/lib/documents/document.remote.ts       GENERATED
    src/lib/skgo/types/<pkg-slug>/types.ts     GENERATED TS types per Go package
    src/routes/go.mod                          GENERATED module boundary
    src/routes/(group)/remotes/[id]/messages.remote.go     THE colocated route handler
    src/routes/(group)/remotes/[id]/messages_jsonschema.go
    src/routes/(group)/remotes/[id]/messages_test.go
    src/routes/(group)/remotes/[id]/messages.remote.ts     GENERATED sibling
    src/routes/(group)/remotes/[id]/jsonschema_gen.go      GENERATED (provider write-back)
    src/routes/(group)/remotes/[id]/jsonschema/*.json      GENERATED
    src/routes/(group)/remotes/[id]/+page.svelte
    src/routes/(group)/remotes/[id]/+page.ts
    src/routes/remotes/notes/data.remote.ts    GENERATED (from external package via directive)
```

Note: the Go module root is the *parent* of the app (`GoRoot/app`). That is what made the
route tree reachable by the Go module at all
(`junkyard/ephemeral/route-go-colocation/orientation.md:L14-L16`). The new skgo brief
inverts this (`--src-root ../ --svelte-routes ../app/src/routes`) — see verdicts below.

## 1. The colocated route handler

`junkyard/ephemeral/remote-codegen/fixture/app-overlay/src/routes/(group)/remotes/[id]/messages.remote.go:L1-L43`

```go
// Package messages ... living directly under a route group and a dynamic
// parameter (src/routes/(group)/remotes/[id]/), with no output directive —
// its .remote.ts is the default colocated-sibling placement.
package messages

import (
	"context"
	"sync"

	"github.com/tylergannon/sveltekit-adapter-go/remote"
)

type Message struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type GetMessageInput struct {
	ID string `json:"id"`
}

func getMessage(_ context.Context, in GetMessageInput) (Message, error) { ... }

// GetMessage is the messages route's Query.
var GetMessage = remote.Query("getMessage", getMessage)
```

Key facts:
- Package name is a plain identifier (`messages`) even though the directory is `[id]`; Go
  only cares about the *import path* being legal, and that comes from the symlink.
- Binding is a package-level `var X = remote.Query("name", fn)` — a value, not a marker.
  The string is the TS export name Kit will see. The generator reads these by
  `packages.Load` + type inspection, not by regex.
- Handler signature: `func(context.Context, In) (Out, error)`. Live handlers are
  `func(ctx, In, yield func(Out) error) error`
  (`.../src/lib/documents/document.remote.go:L55-L79`).
- Command with a declared refresh: `remote.Command("saveDocument", fn,
  remote.CommandOptions{RefreshRequested: []remote.QueryRef{GetDocument.Ref()}})`
  (`document.remote.go:L97-L99`).

## 2. The jsonschema registration file (build-tagged stub)

`.../[id]/messages_jsonschema.go:L1-L20`

```go
//go:build jsonschema

package messages

import (
	"encoding/json"
	jsonschema "github.com/tylergannon/go-gen-jsonschema"
)

func (GetMessageInput) Schema() json.RawMessage     { panic("not implemented") }
func (GetMessageInput) ValidateJSON(_ []byte) error { panic("not implemented") }

func (Message) Schema() json.RawMessage     { panic("not implemented") }
func (Message) ValidateJSON(_ []byte) error { panic("not implemented") }

var (
	_ = jsonschema.NewJSONSchemaMethod(GetMessageInput.Schema)
	_ = jsonschema.NewJSONSchemaMethod(Message.Schema)
)
```

- Every named argument and result type gets a stub pair and a registration. The
  `//go:build jsonschema` tag keeps the stubs out of ordinary builds; the provider
  (`go tool gen-jsonschema`) builds with the tag and replaces the stubs with real
  `jsonschema_gen.go` output written back beside the authored file.
- Enum and discriminated-union configuration is authored here too, never inferred
  (`fixture/backend/remotes/accounts/profile_jsonschema.go:L17-L29`):

```go
_ = jsonschema.NewJSONSchemaMethod(
	Profile.Schema,
	jsonschema.WithEnum(Profile{}.Tier),
	jsonschema.WithInterface(
		Profile{}.Payment,
		jsonschema.Discriminator("kind"),
		jsonschema.Impl("credit_card", CreditCard{}),
		jsonschema.Impl("bank_transfer", BankTransfer{}),
	),
)
```

  The browser then observes `payment.kind === "credit_card"` and `tier === "pro"`
  through the Go codec (`proof/run-slice.mjs:L410-L412`).
- Removing a registration is a hard generator error naming the binding
  (`proof/run-slice.mjs:L330-L348`); a bare pointer field (`Author *string`) is also refused
  (`run-slice.mjs:L273-L288`).

## 3. A colocated test in the route package

`.../[id]/messages_test.go:L1-L19` — an ordinary `package messages` test with no build tag.
It exists to prove `skgo test ./...` reaches the symlinked package. Same shape as
`fixture/backend/remotes/notes/notes_test.go:L1-L19`.

## 4. The Svelte page importing generated `.remote.ts`

`.../[id]/+page.svelte:L1-L40`

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { getDocument, saveDocument, watchDocument } from '#lib/documents/document.remote.js';
  import { getProfile } from '#lib/remotes/accounts/profile.remote.js';
  import { getMessage } from './messages.remote.js';          // sibling, generated
  import { getNote } from '../../../remotes/notes/data.remote.js';
  import { page } from '$app/state';

  let messageQuery = $state<ReturnType<typeof getMessage> | undefined>(undefined);
  onMount(() => {
    const id = page.params.id;
    if (!id) throw new Error('Missing route id');
    messageQuery = getMessage({ id });
  });
  async function save() {
    await saveDocument({ path: 'doc.json', title: 'Updated title' }).updates(docQuery);
  }
</script>
<p data-testid="messages-text">{messageQuery?.current?.text ?? 'loading'}</p>
```

- `#lib` alias (kit 3), `$app/state` for `page.params.id`, `.js` extension on `.ts` imports.
- Every remote call is deferred to `onMount` and `+page.ts` sets `export const ssr = false`
  (`+page.ts:L1-L3`) because the junkyard had no Go-backed SSR bridge. The new skgo brief has
  no SSR at all, so this is the permanent shape, not a workaround.
- The `+page.ts` originally had a `load` forwarding `params.id`; it broke `svelte-check`
  (`Cannot find module './$types'`) under the shared-node_modules candidate layout and was
  removed in favour of `$app/state` (`junkyard/ephemeral/worklog/route-go-colocation.md:L34-L41`).

## 5. External (non-colocated) package with an explicit output directive

`fixture/backend/remotes/notes/notes.remote.go:L1-L2`

```go
//skgo:remote output=src/routes/remotes/notes/data.remote.ts
package notes
```

The directive is a package-doc-position comment; the generator refuses collisions,
`../` escapes, and Kit-reserved `server.remote.ts` names
(`proof/run-slice.mjs:L163-L190`). Route-colocated packages need **no** directive
(`route-go-colocation/core-progress.md:L18-L21`).

## 6. The vite config

`fixture/app-overlay/vite.config.ts:L1-L27`

```ts
import adapter from "@sveltejs/adapter-node";
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";
import { skgoCapture } from "./vite/skgo-capture";

const mode = (process.env.SKGO_MODE ?? "dev") as "dev" | "production";
const origin = process.env.ORIGIN ?? "http://127.0.0.1:4370";
const appDir = process.env.SKGO_APP_DIR ?? "_app";
const base = (process.env.SKGO_BASE ?? "") as "" | `/${string}`;
const capture = process.env.SKGO_CAPTURE ?? path.join(root, ".skgo/remotes-kit.json");

export default defineConfig({
  plugins: [
    sveltekit({
      adapter: adapter(),
      appDir,
      paths: { base, origin },
      compilerOptions: { experimental: { async: true } },
      experimental: { remoteFunctions: true },
    }),
    skgoCapture({ root, out: capture, mode, base, appDir, origin }),
  ],
});
```

Kit 3: no `svelte.config.js`; adapter, `appDir`, `paths.origin`, and `experimental.remoteFunctions`
all live in the `sveltekit()` plugin options. The `skgoCapture` plugin is listed *after*
`sveltekit()` and additionally sets `enforce: "post"` (see fixture-harness.md).

## 7. What the generator emitted

### The Go server main
`fixture/backend/cmd/generated-candidate/main.go:L1-L30` — hand-written, tiny:

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /healthz", ...)
if err := skgoremotes.RegisterRemotes(mux); err != nil { log.Fatalf(...) }
http.ListenAndServe(*addr, mux)
```

`RegisterRemotes(mux)` is the single generated entry point; it "owns SvelteKit's whole
remote prefix" (`/_app/remote/`). The application keeps its own routes on the same mux.

### The generated registrar (`internal/skgoremotes/remotes_gen.go`)
Not in the fixture (it is generated into the candidate), but the harness pins its shape:
- imports the route package via the link path, e.g.
  `"example.com/skgofixture/.skgo/links/mfyhal3t..."` (`proof/run-slice.mjs:L929-L935`);
- one `<Pkg>.<Binding>.Bind(...)` call per binding, e.g. `GetUnused.Bind(`
  (`run-slice.mjs:L621-L622`), keyed by the Kit-captured ID from `remotes-kit.json`;
- unimported-but-discovered modules are still registered (`run-slice.mjs:L616-L630`).

### Wire format the Go server honoured (useful for tests)
- Query GET: `${remotePrefix}${id}?payload=<base64url(devalue.stringify(arg))>`
  (`run-slice.mjs:L354-L356`, `L623`).
- Command POST: JSON body `{ payload: <base64url devalue>, refreshes: [] }`
  (`run-slice.mjs:L360-L370`); an error response contains `"type":"error"`.
- Stale ID after a rename → 404 (`run-slice.mjs:L710-L713`).

### Kit-assigned identity
Kit hashed the *authored* TS module path: `src/routes/(group)/remotes/[id]/messages.remote.ts`
→ `iyl7i3/getMessage`, identical in dev and production; the Go symlink prefix never appears
(`route-go-colocation/fixture-progress.md:L14-L16`,
`route-go-colocation/validation/final-01.md:L42-L46`).

## Reusable verdicts (against the new brief)

| Technique | Verdict | Reason |
|---|---|---|
| Colocated `*.remote.go` in `src/routes/(group)/[id]/` with a plain package name | **KEEP** | Proven; nothing Kit- or Vite-facing sees the Go file. |
| `var GetMessage = remote.Query("getMessage", getMessage)` value binding | **KEEP WITH CHANGES** | New brief uses marker functions `var _ = skgo.QueryFunction(fn)`. The junkyard's value shape carried the TS export name explicitly and let `Command` reference `Query.Ref()` for refreshes; with a marker the export name must come from the Go func name and refresh declarations need another channel. Keep the handler signatures. |
| Build-tagged `*_jsonschema.go` stubs + explicit `NewJSONSchemaMethod` registrations, enum/union authored | **KEEP** | The generator drives `go tool gen-jsonschema` on them; missing registration is a named error. Nothing to reinvent. |
| `//skgo:remote output=` directive for external packages | **KEEP WITH CHANGES** | Only needed for packages outside the route tree; if the new brief is routes-only, drop it and its three rejection rules. |
| `+page.ts` `ssr = false` + `onMount` for remote calls | **KEEP** | No SSR in the new design; this is simply the shape. |
| `$app/state` `page.params.id` instead of a `load` forwarder | **KEEP** | Avoids the `./$types` resolution trap. |
| `vite.config.ts` shape (kit 3 options inside `sveltekit()`) | **KEEP** | Drop `adapter-node` for the skgo adapter; keep `experimental.remoteFunctions` and `paths.origin`. |
| Hand-written `main.go` calling `RegisterRemotes(mux)` | **KEEP** | Minimal, ordinary `net/http`. |
| TS stubs generated as real Kit remote modules (`.remote.ts` that Kit transforms) | **KEEP WITH CHANGES** | New brief says stubs THROW if executed in JS; the junkyard's generated TS wrapped Kit's `query()`/`command()` so Kit assigned IDs. Whether a throwing stub still gets a Kit ID depends on the capture strategy (see fixture-harness.md). |

## Gotchas
- `+page.ts` `load` + `./$types` breaks `svelte-check` when `node_modules` is a symlink to a
  shared install (`worklog/route-go-colocation.md:L34-L41`).
- `page.params.id` is `string | undefined` in kit 3; guard it or `svelte-check` fails
  (`worklog/route-go-colocation.md:L57-L59`).
- The fixture's `go.mod` uses a relative `replace` to the adapter module that is only valid
  inside the repo; `assembleCandidate` rewrites it to absolute (`candidate.mjs:L20-L28`).
- `go 1.27.1` and the `tool github.com/tylergannon/go-gen-jsonschema/gen-jsonschema`
  directive in `go.mod` (`fixture/backend/go.mod:L3,L22`) — the provider is invoked as
  `go tool gen-jsonschema`, so the tool directive is load-bearing.

## Recipes
- To write a new colocated remote: copy `messages.remote.go:L1-L43` and
  `messages_jsonschema.go:L1-L20`, rename types, add one `NewJSONSchemaMethod` per named type.
- To add an enum or union to a result type: `profile_jsonschema.go:L17-L29`.
- To write a Live (streaming) handler: `document.remote.go:L55-L90`.
- To declare a Command that refreshes a Query: `document.remote.go:L95-L99`.
- To call a remote from the page without SSR: `+page.svelte:L16-L24`, `+page.ts:L3`.
- To wire the generated registrar into a server: `cmd/generated-candidate/main.go:L18-L25`.
