# kit Vite plugin: how a `.remote.*` module is transformed for the client (and what it needs from the server build)

Source: `@sveltejs/kit` 3.0.0-next.25 (pinned clone),
`reference/kit/packages/kit/src/exports/vite/index.js` (`plugin_remote`),
`reference/kit/packages/kit/src/exports/vite/utils.js`, `core/env.js`, `utils/hash.js`,
`core/postbuild/analyse.js`, `exports/internal/server/remote-functions.js`.

## Purpose

Explains exactly what the client bundle contains for a remote module, how the `<hash>/<name>`
id is derived (so Go can compute identical ids), and what the *server-side evaluation* of the
module must look like for the transform to succeed — the constraints a generated throwing
stub must satisfy.

## Key facts

### Which files are remote modules

- Pattern `/[/.]remote\.[^/]+$/` on the posixified id: `foo.remote.ts`, `foo.remote.js`,
  `x/remote.ts`, etc. (`reference/kit/packages/kit/src/exports/vite/utils.js:L144`,
  `is_remote_module` `L158-L164`). Files in `node_modules` count only if their package has a
  peer dependency on `@sveltejs/kit` (`L170+`).
- `experimental.remoteFunctions: true` must be set in the `sveltekit({...})` plugin options
  (`exports/vite/index.js:L130-L145`, guard plugin `L768-L785`; default `false` at
  `core/config/options.js:L118`). Without it, importing a remote module errors.
- `.remote.*` files are externalized from Vite dep pre-bundling (`exports/vite/index.js:L454-L476`).

### The id: `hash(file) + '/' + exportName`

- `file = posixify(path.relative(root, id))` where `root` is the Vite root
  (`vite_config.root` resolved, else `process.cwd()`) (`exports/vite/index.js:L200-L202`, `L650-L654`).
  Typical value: `src/lib/todos.remote.ts`.
- `hash` is djb2 over UTF-16 code units iterated **from the last character to the first**,
  32-bit wrapping, then `(hash >>> 0).toString(36)`
  (`reference/kit/packages/kit/src/utils/hash.js:L5-L23`):
  ```js
  let hash = 5381; let i = value.length;
  while (i) hash = (hash * 33) ^ value.charCodeAt(--i);
  return (hash >>> 0).toString(36);
  ```
- Golden values (computed with kit's `hash.js`): `src/lib/todos.remote.ts` → `worolc`;
  `src/routes/data.remote.js` → `mxe8u8`; `src/lib/a.remote.ts` → `txkpmq`.
- Server side sets `fn.__.id = hash + '/' + name` and `fn.__.name = name`
  (`exports/vite/index.js:L677-L680`; `exports/internal/server/remote-functions.js:L18-L27`).

### Client output: the module is replaced wholesale

- For `consumer === 'client'` the transform **discards the original code** and returns
  ```js
  import * as __remote from '<relative path to runtime/client/remote-functions/index.js>';

  export const getTodos = __remote.query('worolc/getTodos');
  export const addTodo = __remote.command('worolc/addTodo');
  ...
  import.meta.hot?.accept();   // dev only
  ```
  (`exports/vite/index.js:L703-L757`; expression builder at `L733-L737`
  `` `${ns}.${map.get(name)}('${remote.hash}/${name}')` ``).
- `map.get(name)` is the server-side `__.type` string, one of
  `'query' | 'query_batch' | 'query_live' | 'command' | 'form' | 'prerender'`
  (`exports/internal/server/remote-functions.js:L4`), which are exactly the named exports of the
  client runtime index (`runtime/client/remote-functions/index.js:L1-L6`:
  `command, form, prerender, query, query_batch, query_live`).
- Export names that are JS reserved words are aliased (`const _0 = ...; export { _0 as for }`)
  (`core/env.js:L376-L417`, reserved list `L296+`); the namespace `__remote` is renamed if an
  export collides (`L380-L384`).
- No user imports, types, schemas, or bodies survive into the client bundle. The client cannot
  see argument validation, so the server is the only validator.

### Where the export-name → type map comes from

- **Dev:** the plugin imports the module in Vite's SSR module runner
  (`get_runner(vite, dev_server).import(id)`) and reads `value?.__?.type` from every export
  (`exports/vite/index.js:L708-L720`). Exports without a `__.type` are silently omitted from
  the client module.
- **Prod:** the server bundle is built first, then `analyse()` loads every remote chunk through
  `manifest._.remotes[hash]()` and records `{ type, dynamic }` per export
  (`exports/vite/index.js:L1201-L1225`; `core/postbuild/analyse.js:L148-L166`); the client
  build then reads `build_metadata.remotes.get(hash)` (`L722-L731`). A missing entry throws
  `'Expected to find metadata for remote file'`.
- Either way, **the module is executed in Node** at build/dev time. Its top level must not throw
  and every export must be a value with `__.type` in the allowed set.

### Server-side transform (what runs in the SSR build / dev runner)

- For non-client consumers the original code is kept and this is appended
  (`exports/vite/index.js:L658-L701`):
  ```js
  import * as $$_self_$$ from './todos.remote.ts';
  import { init_remote_functions as $$_init_$$ } from '@sveltejs/kit/internal/server';
  await Promise.resolve()   // dev only
  $$_init_$$($$_self_$$, "src/lib/todos.remote.ts", "worolc");
  for (const [name, fn] of Object.entries($$_self_$$)) { fn.__.id = "worolc" + '/' + name; fn.__.name = name; }
  ```
- `init_remote_functions` throws if the module has a `default` export or if any export lacks
  `__.type ∈ types` (`exports/internal/server/remote-functions.js:L11-L27`) — message
  `` `${name}` exported from ${file} is invalid — all exports from this file must be remote functions ``.
  Type-only exports are erased by TS and are fine; a re-exported constant is not.
- In prod, each remote module becomes its own server chunk `chunks/remote-<hash>.js` via a virtual
  entry `\0sveltekit-remote:<hash>` that re-exports the module as `default`
  (`exports/vite/index.js:L623-L637`, `L684-L695`; manifest at `core/generate_manifest/index.js:L117-L118`).
- Prerender functions are additionally **executed at build** by kit's prerenderer for every input
  (`core/postbuild/prerender.js:L659-L719`: enqueues `remote_prefix + id [+ '/' + payload]`),
  regardless of the `dynamic` option. `treeshake_prerendered_remotes` then strips non-dynamic
  prerender functions from the server bundle (`exports/vite/build/remote.js:L20+`;
  `analyse.js:L159-L162`).

### `$app/server` factory signatures (what the stub calls)

- `query(fn)` / `query('unchecked', fn)` / `query(schema, fn)`; `query.live(...)` and
  `query.batch(...)` are properties on `query` (`runtime/app/server/remote/query.js:L65`, `L664-L665`).
- `command(...)`, `form(...)`, `prerender(fn, {inputs?, dynamic?})` /
  `prerender('unchecked'|schema, fn, opts)` (`runtime/app/server/remote/prerender.js:L62-L82`;
  `runtime/app/server/remote/index.js:L1-L5` exports `command, form, prerender, query, requested`).
- Each factory attaches `__` via `Object.defineProperty(wrapper, '__', { value: __ })` with
  `{ type, id: '', name: '', ... }` (`query.js:L73-L77`, `L113`; `command.js:L62`; `form.js:L90-L93`;
  `prerender.js:L75-L82`). A validator is created by `create_validator`: no schema → argument must
  be `undefined` (400 otherwise); `'unchecked'` → passthrough; Standard Schema → validated
  (`runtime/app/server/remote/shared.js:L12-L40`).
- Static analysis (`exports/vite/static_analysis/index.js`) does not touch remote exports; no
  `export const prerender/ssr` style options apply to remote modules.

## Citations

- `reference/kit/packages/kit/src/exports/vite/index.js:L130-L145`, `L200-L202`, `L454-L476`, `L596-L765`, `L768-L785`, `L1201-L1225`
- `reference/kit/packages/kit/src/exports/vite/utils.js:L144-L164`
- `reference/kit/packages/kit/src/exports/vite/dev/index.js:L313-L319`
- `reference/kit/packages/kit/src/exports/vite/build/remote.js:L20-L40`
- `reference/kit/packages/kit/src/exports/internal/server/remote-functions.js:L1-L28`
- `reference/kit/packages/kit/src/core/env.js:L296-L300`, `L376-L417`
- `reference/kit/packages/kit/src/core/postbuild/analyse.js:L148-L169`
- `reference/kit/packages/kit/src/core/postbuild/prerender.js:L268`, `L659-L719`
- `reference/kit/packages/kit/src/core/generate_manifest/index.js:L117-L118`
- `reference/kit/packages/kit/src/core/config/options.js:L118`
- `reference/kit/packages/kit/src/utils/hash.js:L5-L23`
- `reference/kit/packages/kit/src/runtime/client/remote-functions/index.js:L1-L6`
- `reference/kit/packages/kit/src/runtime/app/server/remote/{index.js:L1-L5, query.js:L65-L116, L664-L665, command.js:L62, form.js:L90-L93, prerender.js:L62-L82, shared.js:L12-L40}`
- `reference/kit/packages/kit/src/types/internal.d.ts:L215-L219`, `L565-L641`

## Go implementation notes

- Go must compute the same hash from the same relative posix path. Port:
  ```go
  func kitHash(s string) string {
      h := int32(5381)
      u := utf16.Encode([]rune(s))
      for i := len(u) - 1; i >= 0; i-- {
          h = int32(int64(h)*33) ^ int32(u[i])   // JS: ToInt32(hash*33) ^ code
      }
      return strconv.FormatUint(uint64(uint32(h)), 36)
  }
  ```
  Test against the golden values above. The path must be relative to the **Vite root** (the
  directory containing `vite.config.*`), using `/` separators, no leading `./`.
- The generator that writes `X.remote.ts` decides the file path, therefore the hash; keep the
  path stable (renaming the file changes every URL) and derive it from the Go package/name in a
  deterministic way.
- The Go registry needs: `{hash, exportName, type}` per function to mount
  `/_app/remote/<hash>/<name>`; nothing else from the transform is needed at runtime.
- `base` and `appDir` must match the kit config (`paths.base`, `appDir`); read them from the same
  config skgo passes to the adapter.

## Gotchas

- The client transform runs the stub **in Node** (dev: on every transform; prod: during
  `analyse`). Any top-level throw, missing `__`, default export, or non-remote export breaks the
  build with kit's own error messages.
- In dev, kit's dev server answers `/_app/remote/*` itself and would invoke the stub's throwing
  body. skgo's dev flow must put Go in front of Vite for that prefix (or `server.proxy` that
  prefix to Go) — otherwise every remote call 500s with the stub's error.
- Prerender stubs get executed at build by the prerenderer; a throwing body fails the build
  unless `prerender.handleHttpError` is relaxed or the stub can answer (see `stub-shape.md`).
- HMR: `import.meta.hot.accept()` re-runs `query(id)`/`query_live(id)`, which refreshes/reconnects
  every live cache entry for that id (`runtime/client/remote-functions/query/index.js:L13-L22`,
  `query-live/index.js:L14-L23`).
- The `dynamic` flag only affects server tree-shaking, not the client URL shape.

## Recipes

- Verify a generated id from Go without running Vite: `kitHash("src/lib/todos.remote.ts") == "worolc"`.
- Verify the transform end-to-end: run `vite build` on the generated stub and grep the client
  output for `remote/…` calls: each export appears as `__remote.<type>('<hash>/<name>')` in a
  chunk importing `remote-functions/index.js`.
