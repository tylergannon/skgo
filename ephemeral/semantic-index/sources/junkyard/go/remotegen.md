# junkyard Go generator: `internal/remotegen`

Source root: `junkyard/go/internal/remotegen/` (package `remotegen`, ~1900 lines incl. tests). Depends only on `golang.org/x/tools/go/packages` and the stdlib.

## Purpose

Two-stage code generator. **Stage A** (`Generate`) discovers `*.remote.go` files, resolves each `remote.Query/Command/Live` call with `go/types`, checks the author registered the In/Out types with go-gen-jsonschema, runs that provider in a staging copy, and writes `.remote.ts` stubs + `types.ts` + inventories. **Stage B** (`StageB`) joins the Stage A inventory with a JSON capture of Kit's real remote ids (written by a Vite plugin during `vp build`/dev) and emits a Go registrar package that mounts one `http.Handler` on `<base>/<appDir>/remote/`.

## Key concepts

### Discovery and scanning (`scan.go`)

- **Discovery is a filesystem walk for `*.remote.go`**, skipping `node_modules`, `testdata`, dot- and underscore-prefixed dirs, and `_test.go` files — `remoteDirs` at `junkyard/go/internal/remotegen/scan.go:L390-L421`. Every directory with at least one such file becomes a `./rel` pattern for `packages.Load` — `L147-L158`.
- **Route-colocated dirs are never loaded directly.** Any dir under `<app>/src/routes` is dropped from the pattern list and its generated symlink under `.skgo/links/` is loaded instead — `L126-L142`. If a link is missing the scan fails with "run skgo remotes generate". `authoredPath` maps a loaded file back through the link to the path the developer wrote — `L211-L217`.
- **Load mode** needs `NeedTypes|NeedSyntax|NeedTypesInfo|NeedDeps` so constructor identity and type arguments can be resolved — `L155-L158`. Any type error in any loaded package aborts generation — `L162-L171`. `package main` is rejected because the registrar must import the package — `L175-L177`.
- **Marker recognition is by resolved package identity, not source text**: `resolveConstructor` looks up `pkg.TypesInfo.Uses[ident]` and checks `fn.Pkg().Path() == cfg.MarkerPkg` and `fn.Name()` in `{Query, Command, Live}` — `L265-L294`. Import aliases and explicit type args (`IndexExpr`/`IndexListExpr`) are handled. Only package-level `var X = remote.Query(...)` declarations are scanned (`scanFile`, `L219-L263`); the var must be exported (`L246-L248`) and export names must be unique per file (`L253-L256`).
- **Types are inferred from the constructor's instantiated result type**, never restated: `pkg.TypesInfo.TypeOf(call).(*types.Named).TypeArgs()` gives `In`, `Out` — `buildBinding` at `L298-L337`. Both must be `*types.Named` with a package (`namedType`, `L339-L345`); anonymous structs and builtins are rejected. Export name must be a string constant and a valid JS identifier — `L303-L313`, `L428-L442`.
- **Command refresh allow-lists** are read syntactically from `remote.CommandOptions{RefreshRequested: []remote.QueryRef{X.Ref()}}` and resolved to `VarRef{PkgPath, VarName}` — `refreshRefs` at `L347-L388`.
- **`Binding` / `SourceModule` structs** are the scan's output — `L74-L106`. `SourceModule.LoadDir` vs `SourceFile` records "loaded through link" vs "authored path".
- **`Config`** holds `GoRoot`, `RemoteRoot` (must be inside GoRoot), `AppDir`, `MarkerPkg` — `L28-L63`.

### Admission (`admit.go`, `registration.go`)

- **One owned type rule: no bare pointer fields.** `*T` projects as non-null in schema/TS but marshals as `null` when nil, so `CheckAdmitted` walks In/Out structs (skipping provider generics and `time`) and rejects any exported pointer field with a message pointing to `jsonschema.Nullable[T]` — `junkyard/go/internal/remotegen/admit.go:L10-L46`, walker `L48-L95`. Everything else is left to the provider to reject (`L17-L19`).
- **The developer owns schema registration.** `LoadRegistrations` loads the same packages with `-tags=jsonschema` and collects the first type argument of every `jsonschema.NewJSONSchemaMethod/Func/Builder[T]` call (matched by resolved package identity `ProviderPkg = github.com/tylergannon/go-gen-jsonschema`) — `junkyard/go/internal/remotegen/registration.go:L13-L94`. `CheckRegistered` fails with a per-binding message telling the author exactly what line to add — `L110-L142`. The registration file is source, never written or inventoried.

### Placement (`placement.go`)

- **Where the `.remote.ts` lands**: a file-level `//skgo:remote output=<app-relative>` directive wins; else a source already under `<app>/src/lib` or `<app>/src/routes` generates its sibling (authored path, `(group)`/`[id]` included); else `src/lib/<rel-to-remote-root>/<name>.remote.ts` — `outputPath` at `junkyard/go/internal/remotegen/placement.go:L77-L103`. Directive parsing (only comments before `package`, at most one) — `L13-L42`.
- **Kit's server-only naming rules are enforced before writing**: output must be under `src/`, end in `.remote.ts`, not be `server.remote.ts` / `*.server.remote.ts`, and not sit in a `server/` directory outside `src/routes` — `checkAppPath` at `L107-L130`. Two sources cannot claim the same output — `L59-L75`.

### Provider run (`provider.go`)

- **go-gen-jsonschema runs in a throwaway staging copy of the module**, never on the working tree, because at the pinned commit `-no-changes` still rewrites `jsonschema_gen.go` — `junkyard/go/internal/remotegen/provider.go:L36-L55`. `stageModule` copies only `go.mod`, `go.sum`, `*.go` and rewrites relative `replace` targets to absolute — `L178-L218`, `L246-L262`.
- **Command**: `go tool gen-jsonschema gen -target ./<rel> --validate --typescript <tsStage>` with `JSONSCHEMA_NO_CHANGES=` — `L90-L102`. Output collected: `jsonschema/*.json`, `jsonschema_gen.go` (written back to the *authored* dir) and `types.ts` (to `src/lib/skgo/types/<pkg-slug>/`) — `L104-L115`.
- **Route packages are materialized as real directories at the link path in the stage** (symlinks are not followed by the copy) so the provider sees the same import path Go compiles under — `L60-L78`, `stagePkgDir` `L220-L244`.
- **Types dir slug**: `TypesRoot = "src/lib/skgo/types"` (not dot-prefixed, because TS include globs skip dot dirs), `PkgSlug` replaces `/` with `~` — `L15-L27`.

### Emission (`emit.go`)

- **The generated `.remote.ts` stub throws** on invocation: `goRemoteOnly(name): never` — `junkyard/go/internal/remotegen/emit.go:L14-L77`. Kinds map to `query('unchecked', ...)`, `command('unchecked', ...)`, `query.live('unchecked', async function* ...)`; types come from a type-only import of the provider's `types.ts` and are re-exported. Header `// Code generated by skgo remotes generate. DO NOT EDIT.` plus `// skgo:module source=... package=...` — `L11-L12`, `L52-L56`. Excerpt of the actual output is in `testdata-fixture.md`.

### Link tree (`linktree.go`)

- **Why**: `(group)` and `[id]` are illegal Go import path elements; a package under one is unreachable and makes `go list ./...` fail with "invalid char" (proven at `junkyard/go/internal/remotegen/route_test.go:L132-L164`).
- **Scheme**: for each route dir holding `*.remote.go`, a symlink `.skgo/links/<lowercase base32(no pad) of GoRoot-relative path>` → authored dir — `junkyard/go/internal/remotegen/linktree.go:L24-L30`, `L55-L62`, `PlanLinks` `L72-L116`. Base32 is injective and import-path safe (test `linktree_test.go:L38-L62`).
- **Module boundary trick**: writing a minimal `go.mod` (`module skgo-routes-boundary`, same `go` directive as the parent) at `<app>/src/routes/go.mod` ends the enclosing module's package walk so the authored tree is excluded and the package has exactly one identity (through the link) — `boundaryContent` `L148-L162`, `applyBoundary` `L336-L354`. An author-supplied boundary is preserved verbatim and marked `Owned:false` — `L340-L342`, test `linktree_test.go:L110-L132`.
- **`.skgo/links.json`** (`LinkInventoryPath`) is a self-contained GoRoot-relative record `{schemaVersion, linkRoot, boundary{path, owned}, links[{link,target}]}` consumed by `skgo test` — `L34-L52`, `L127-L146`.
- **Safety**: prune only unlinks a symlink that still points where the inventory said (`L358-L388`); `Drift` is read-only for `--check` (`L180-L214`); `LinkSetup.Undo` rolls back exactly what a rejected run created (`L240-L287`, exercised by `route_test.go:L288-L339`). Corrupt inventory is treated as absent (`ReadLinkInventory`, `L390-L416`).

### Orchestration and ownership (`generate.go`)

- **Order**: PlanLinks → Apply (or Drift under `--check`) → Scan → Resolve → CheckAdmitted → LoadRegistrations → CheckRegistered → RunProvider → EmitModule → write `.skgo/links.json` and `<app>/.skgo/remotes-source.json` → diff or apply — `junkyard/go/internal/remotegen/generate.go:L64-L171`.
- **Stage A inventory** `remotes-source.json` lists modules/bindings (type keys as `pkgPath.TypeName`) and every generated file with a sha256 digest — `L16-L51`, `buildInventory` `L175-L215`. The digest is the ownership marker: cleanup removes an obsolete file only if bytes still match — `checkOwned` `L353-L368`; and `apply` refuses to overwrite a pre-existing file that is neither inventoried nor carries a `Code generated by` header — `L303-L351`. Inventory paths are validated as clean, contained relative paths before ever reaching `os.Remove` — `containedRel` `L257-L274`, test `generate_test.go:L32-L59`.
- Writes are atomic (`.skgo-tmp` + rename) — `L333-L339`.

### Stage B (`stageb.go`)

- **`KitCapture`** shape: `{schemaVersion:1, mode: dev|production, base, appDir, origin?, digest, mappings:[{module, exportName, kind, id}]}` — `junkyard/go/internal/remotegen/stageb.go:L21-L40`. "Kit's IDs are authoritative: neither Go nor skgo ever calculates or hashes one" (`L21-L23`). Mapping `id` has the form `<hash>/<exportName>` (validated in the runtime, `remote.go:L87-L89`); the URL is `<base>/<appDir>/remote/<id>`.
- **Join**: keyed by `output-module-path#exportName`; every Stage A binding must have exactly one capture entry with matching kind, and every capture entry must have a Go declaration — `L60-L97`. Tests: `stageb_test.go:L57-L77`.
- **Registrar** is written to `<go-root>/internal/skgoremotes/remotes_gen.go`, embeds the capture JSON as a string constant, imports each declaring package under a deterministic `pkgN` alias, and emits `NewHandler()` (builds `[]remote.Binding{ pkg0.GetDocument.Bind("src/lib/documents/document.remote.ts"), ... }` → `remote.NewHandler(manifest, bindings, version)`) plus `RegisterRemotes(mux)` which does `mux.Handle("<base>/<appDir>/remote/", h)` — `L108-L159`. It is gofmt'd via `go/format.Source` so a template bug fails loudly — `L151-L154`. Build-owned, never committed (`L14`).

## Citations

- `junkyard/go/internal/remotegen/scan.go:L113-L206` — `Scan` (discovery + load + per-file scan)
- `junkyard/go/internal/remotegen/scan.go:L265-L337` — marker resolution and type inference
- `junkyard/go/internal/remotegen/admit.go:L20-L46` — bare-pointer rule
- `junkyard/go/internal/remotegen/registration.go:L40-L94` — read `//go:build jsonschema` registrations
- `junkyard/go/internal/remotegen/placement.go:L77-L130` — output path + Kit reserved names
- `junkyard/go/internal/remotegen/provider.go:L46-L118` — staged provider run
- `junkyard/go/internal/remotegen/emit.go:L19-L77` — `.remote.ts` stub template
- `junkyard/go/internal/remotegen/linktree.go:L55-L62`, `L148-L162`, `L289-L331` — link naming, boundary go.mod, Apply
- `junkyard/go/internal/remotegen/generate.go:L64-L171` — Stage A pipeline
- `junkyard/go/internal/remotegen/stageb.go:L45-L160` — capture join + registrar template
- `junkyard/go/internal/remotegen/route_test.go:L191-L245` — end-to-end: generate, `go build ./...`, idempotent regenerate, read-only `--check`

## Reusable verdict

| Component | Verdict | Why |
|---|---|---|
| `remoteDirs` walk + `packages.Load` with full types | REUSE AS-IS | Exactly how the new generator should find `*.remote.go`, `page.server.go`, `layout.server.go`; add the two new suffixes. |
| `resolveConstructor` (identity-based marker matching) | REUSE WITH CHANGES | Same technique works for `var _ = skgo.QueryFunction(getUser)`: match `fn.Pkg().Path()==markerPkg`, then read the *argument* (`call.Args[0]` as `*types.Func` via `TypesInfo.Uses`) instead of the result type's `TypeArgs`. The export name would come from the Go func name rather than a string constant. |
| `buildBinding` In/Out inference via `types.Named` | REUSE WITH CHANGES | Read them from the referenced func's `*types.Signature` (params/results) instead of the wrapper's type args. Keep the "must be a named type" rule. |
| `refreshRefs` | REDESIGN | Tied to `CommandOptions{RefreshRequested}` wrapper values; marker-function style needs another spelling (or drop static allow-lists). |
| `CheckAdmitted` bare-pointer rule | REUSE AS-IS | Real devalue/JSON null mismatch; still true under polytype. |
| `LoadRegistrations`/`CheckRegistered` | REUSE WITH CHANGES | Swap `ProviderPkg` to polytype's import path and marker names. The "developer owns registration, generator only reads it" stance is worth keeping. |
| `placement.go` Kit reserved-name checks | REUSE AS-IS | `server.remote.ts` / `server/` dir rules are Kit facts, and the sibling-placement rule is what colocation means. Extend for `+page.server.ts` next to `page.server.go`. |
| `RunProvider` staging copy | REUSE WITH CHANGES | Keep the staging idea only if polytype also mutates the tree on `--check`; the `stagePkgDir` materialize-link trick is needed whenever the provider is run via `go tool` on a route package. Replace the command line. |
| `EmitModule` throwing stub | REUSE WITH CHANGES | Exactly the brief's "stubs that THROW". Needs a sibling template for `+page.server.ts` (`export const load: PageServerLoad = () => { throw ... }`) and `'unchecked'` should be revisited against the pinned Kit 3 signature. |
| Link tree + `src/routes/go.mod` boundary + `links.json` | REUSE AS-IS | Solves the `[id]`/`(group)` package problem with proven tests (`route_test.go:L132-L186`). Directly matches the brief's "creates symlinks so route directories become importable Go packages". |
| Inventory + digest ownership (`generate.go`) | REUSE AS-IS | Small, tested, prevents clobbering user files; keep even if the file names change. |
| Stage B capture join + registrar | REDESIGN | The registrar shape (import declaring packages, build `[]Binding`, mount one prefix) is right, but the brief wants Go handlers "mounted at exactly the URL paths SvelteKit would use" — i.e. either compute Kit's hash in Go (Stage B becomes part of Stage A) or keep the capture plugin. Note the capture requires a Node build step before Go can even compile the registrar. |

## Gotchas

- **Stage B cannot run without Node.** The registrar is generated from a capture only the Vite plugin produces; `go build` of the app is impossible until `vp build`/dev has run. If the new design computes remote ids in Go, this whole coupling disappears — check the pinned Kit source for how `_app/remote/<hash>/<name>` hashes the module path before deciding.
- **The capture keys by `.remote.ts` app-relative path** (`module`) — so Stage A's `Output` placement *is* the join key. Change placement and the capture no longer matches.
- **`go tool gen-jsonschema` at rc.5 rewrites `jsonschema_gen.go` even on `-no-changes`**; that is why staging exists (`provider.go:L40-L45`). Verify whether polytype fixed this before deleting staging.
- **Route dirs under `src/routes` are excluded from the `RemoteRoot` walk and only loaded via links** (`scan.go:L126-L136`) — so `Scan` fails hard if the link tree has not been applied first. `Generate` handles ordering; a standalone scan must call `PlanLinks(...).Apply` first.
- **A route package's import path is the ugly base32 link path** (`example.com/app/.skgo/links/<b32>`); the registrar imports that path. `route_test.go:L92-L95` asserts this. Tooling that renders import paths to humans should map back through `links.json`.
- **`package main` remote packages are rejected**, as are unexported binding vars — the registrar must import them.
- **`.skgo/links` must be created before `packages.Load`**; a rejected run rolls it back (`generate.go:L82-L98`), so partially applied state after a crash is possible only if the process dies mid-Apply.
- **Placement uses authored paths; provider uses link paths** — `Placement.LoadDir` vs `SourceFile`; mixing them up writes generated Go into the symlink dir (test `route_test.go:L112-L129` guards this).
- Go 1.24+ APIs used (`strings.SplitSeq`, `for range N`, `os.CopyFS`); `go.mod` says `go 1.27`.
- Fixture `go.mod` uses a `tool` directive (`tool github.com/tylergannon/go-gen-jsonschema/gen-jsonschema`) and a relative `replace` to the adapter repo (`testdata/mod/go.mod:L20-L22`); `routeFixture` absolutizes it when copying (`route_test.go:L26-L33`).

## Recipes

- To scan `*.remote.go` and get typed bindings: start at `junkyard/go/internal/remotegen/scan.go:L113` (`Scan`), then `L219` (`scanFile`), `L298` (`buildBinding`).
- To adapt marker matching to `var _ = skgo.QueryFunction(getUser)`: replace `resolveConstructor` (`scan.go:L267-L294`) to key on the marker name and resolve `call.Args[0]` via `pkg.TypesInfo.Uses`; take In/Out from `fn.Type().(*types.Signature)`.
- To create the symlink tree for `[id]`/`(group)` packages: `junkyard/go/internal/remotegen/linktree.go:L74` (`PlanLinks`) then `L289` (`Apply`); boundary go.mod content at `L150-L162`.
- To read who registered which schema types: `junkyard/go/internal/remotegen/registration.go:L40-L94`.
- To emit a throwing TS stub: `junkyard/go/internal/remotegen/emit.go:L19-L77`.
- To emit the Go registrar (mux mount + Bind calls): `junkyard/go/internal/remotegen/stageb.go:L108-L159`.
- To implement safe generated-file ownership and `--check` drift: `junkyard/go/internal/remotegen/generate.go:L175-L215` (inventory), `L276-L301` (diff), `L303-L368` (apply/checkOwned).
- To run a `go tool` codegen without touching the working tree: `junkyard/go/internal/remotegen/provider.go:L46-L118`.
- To see the full end-to-end expectation (generate → `go build ./...` → idempotent → `--check` clean): `junkyard/go/internal/remotegen/route_test.go:L191-L245`.
