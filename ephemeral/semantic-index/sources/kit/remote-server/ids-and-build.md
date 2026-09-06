# Remote function ids and build-time transform (kit 3.0.0-next.25, pinned source)

**Purpose.** Exactly how kit derives the `<hash>/<name>` id that the client bundle
bakes into every remote-function call, so a Go server can compute the same ids from
the file tree without running a Node build. Also how `.remote.*` files are rewritten
for the client (the "stubs") and for the server (the `__` internals init).

All facts below are SOURCE-DERIVED from the pinned clone under the token cache root.

## Key facts

### The id: `hash(file) + '/' + exportName`

- **Hash function: djb2 over UTF-16 code units, iterated from the END of the string,
  `>>> 0`, rendered base-36.**
  `reference/kit/packages/kit/src/utils/hash.js:L5-L22`
  ```js
  let hash = 5381;
  let i = value.length;
  while (i) hash = (hash * 33) ^ value.charCodeAt(--i);
  return (hash >>> 0).toString(36);
  ```
  Note `hash * 33` is done in JS double arithmetic then `^` coerces to int32 each
  step; the final `>>> 0` makes it uint32. In Go: `h := int32(5381)`; per char
  `h = int32(int64(h)*33) ^ int32(c)` is NOT exactly equivalent because JS's `*`
  is on doubles (53-bit mantissa) before the `^` ToInt32 — but since `h` is always
  int32 after the first `^`, `h*33` fits in 2^37 < 2^53, so `ToInt32(h*33)` equals
  `int32(int64(h)*33)` (truncation mod 2^32). So Go: `h = (int32(int64(h)*33)) ^ int32(c)`,
  then `strconv.FormatUint(uint64(uint32(h)), 36)`.
- **Hash input string: `posixify(path.relative(root, id))`** where `root` is the Vite
  root (absolute, posixified; `vite_config.root` or `process.cwd()`), and `id` is the
  absolute module path of the `.remote.*` file **including extension**. Example:
  `src/lib/todos.remote.ts`, `src/routes/blog/data.remote.js`.
  `reference/kit/packages/kit/src/exports/vite/index.js:L648-L656`
  ```js
  const file = posixify(path.relative(root, id));
  const remote = { hash: hash(file), file };
  ```
  root: `reference/kit/packages/kit/src/exports/vite/index.js:L200-L202`
  (`posixify(vite_config.root ? path.resolve(vite_config.root) : process.cwd())`).
  posixify: `reference/kit/packages/kit/src/utils/os.js:L5-L7` (backslash -> slash).
- No `base`, no `appDir`, no query string, no content hash — the id depends ONLY on the
  project-relative path. Renaming/moving the file changes the hash; editing it does not.
- **Export name** is the ES export name, verbatim. Reserved-word exports are aliased in
  the client stub but the id still uses the original name
  (`reference/kit/packages/kit/src/core/env.js:L376-L417`).
- The server-side init writes the same id onto each export:
  `fn.__.id = \`${hash}/${name}\`; fn.__.name = name;`
  `reference/kit/packages/kit/src/exports/internal/server/remote-functions.js:L11-L27`
  and the inline transform duplicates it:
  `reference/kit/packages/kit/src/exports/vite/index.js:L668-L680`.
- Every export MUST be a remote function of type
  `['command','form','prerender','query','query_batch','query_live']`; a `default`
  export throws at init. `...remote-functions.js:L4,L12-L23`.

### Which files count as remote modules

- Pattern: `/[/.]remote\.[^/]+$/` — i.e. `foo.remote.ts`, `foo.remote.js`, or a file
  literally named `remote.<ext>` in any directory (the `[/.]` alternation).
  `reference/kit/packages/kit/src/exports/vite/utils.js:L144`
- Files inside `node_modules` count only if the owning package declares a peer
  dependency on `@sveltejs/kit`. `...vite/utils.js:L153-L164`.
- Feature-gated behind `kit.experimental.remoteFunctions` (default `false`):
  `reference/kit/packages/kit/src/core/config/options.js:L118`; the guard plugin errors
  on any `.remote.*` transform when disabled `reference/kit/packages/kit/src/exports/vite/index.js:L768-L786`.

### Client transform (what the browser bundle contains)

- For the client environment the whole module is REPLACED with:
  `import * as __remote from '<rel>/runtime/client/remote-functions/index.js'`
  followed by one `export const <name> = __remote.<type>('<hash>/<name>');` per export.
  `reference/kit/packages/kit/src/exports/vite/index.js:L700-L757`. `<type>` is one of
  `query | query_batch | query_live | command | form | prerender`.
- In dev the export→type map is discovered by importing the server module through the
  module runner (`L713-L722`); in prod it comes from `build_metadata.remotes` produced
  by the post-build analyse step (`L724-L732`,
  `reference/kit/packages/kit/src/core/postbuild/analyse.js:L148-L166`).
- The analyse step also records `dynamic: type !== 'prerender' || internals.dynamic`
  per export; non-dynamic prerender exports are replaced in the server chunk with a
  stub that throws `Unexpectedly called prerender function. Did you forget to set { dynamic: true } ?`
  `reference/kit/packages/kit/src/exports/vite/build/remote.js:L44-L85`.

### Server manifest

- Prod: `manifest._.remotes = { '<hash>': () => import('./chunks/remote-<hash>.js') }`
  where the chunk is `export default m` (namespace of the original module).
  `reference/kit/packages/kit/src/core/generate_manifest/index.js:L117-L119`,
  `reference/kit/packages/kit/src/exports/vite/index.js:L627-L640,L684-L695`.
- Dev: same shape, backed by the module runner keyed by the same `hash`.
  `reference/kit/packages/kit/src/exports/vite/dev/index.js:L313-L320`.
- So dev and build compute identical ids; there is no dev/prod divergence in the id.

### Public URL

- Request path: `${base}/${app_dir}/remote/<hash>/<name>[/<extra>]`, where `app_dir`
  defaults to `_app`. Detection strips exactly that prefix:
  `reference/kit/packages/kit/src/runtime/server/remote-functions.js:L588-L604`.
- Prerender file placement: `${outDir}/output/prerendered/data/${appDir}/remote/<hash>/<name>[/<payload>]`
  (see form-and-prerender.md).

## Citations

- `reference/kit/packages/kit/src/utils/hash.js:L5-L22` — djb2.
- `reference/kit/packages/kit/src/exports/vite/index.js:L648-L656` — hash input.
- `reference/kit/packages/kit/src/exports/vite/index.js:L200-L202` — root.
- `reference/kit/packages/kit/src/exports/vite/utils.js:L144,L158-L164` — module pattern.
- `reference/kit/packages/kit/src/exports/internal/server/remote-functions.js:L11-L27` — id assignment.
- `reference/kit/packages/kit/src/exports/vite/index.js:L700-L757` — client stub.
- `reference/kit/packages/kit/src/core/env.js:L376-L417` — reserved-name aliasing.

## Go implementation notes

- Compute `id = base36(uint32(djb2(relpath))) + "/" + exportName` with `relpath`
  relative to the Vite root (the directory holding `vite.config.*`), forward slashes,
  extension included. Verify against a known build: hash of `src/lib/todos.remote.ts`
  must equal what the client stub contains (`__remote.query('<hash>/todos')`).
- skgo's own generator decides which file path a Go remote "lives" at. It must emit the
  TypeScript stub file at that path (so kit's client transform runs on it and produces
  the fetch stubs) AND mount Go handlers at `/_app/remote/<hash>/<name>`. The TS stub
  body is irrelevant on the client (kit replaces the module entirely) — but in dev, kit
  imports the server module to learn export types (`L713-L722`), so the stub must
  export objects whose `__.type` is set (i.e. call kit's real `query(...)`, etc., with
  a body that throws) or dev will produce an empty client module.
- Ignore: chunk emission, treeshaking, `remote_original_by_hash` — server-internal.

## Gotchas

- The hash iterates characters from the end; a naive forward djb2 gives a different
  value. Also `charCodeAt` = UTF-16 code units, not bytes (matters only for non-ASCII
  paths).
- The pattern matches a file literally named `remote.ts` at any depth, not only
  `*.remote.ts`.
- `experimental.remoteFunctions` must be `true` in the `sveltekit()` plugin options
  (kit 3 has no `svelte.config.js`).
- Prerender export type is `'prerender'` but the client-side id/type string in the
  stub is what `__remote.prerender(...)` receives; the server `internals.type` for
  live/batch are `query_live` / `query_batch` (underscore), and `collect_remote_data`
  maps them to single-letter buckets `l`/`q` (see request-handling.md).

## Recipes

- To compute ids in Go: read `utils/hash.js:L5-L22` then `exports/vite/index.js:L648-L656`.
- To know what the client stub will import/export for a given file: `exports/vite/index.js:L700-L757` and `core/env.js:L376-L417`.
- To decide how Go must expose prerender vs dynamic prerender: `core/postbuild/analyse.js:L148-L166` and `exports/vite/build/remote.js:L44-L85`.
