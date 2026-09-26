# Topic-001 — Kit remote-module discovery and `.remote.js` handling @ kit@3.0.0-next.28

Evidence root: this directory. `sources/` holds byte-identical copies of pinned
kit files (origin: `/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28`,
version `3.0.0-next.28` per `sources/kit-3.0.0-next.28__package.json:3`, retrieval
2026-09-26; hashes and origin mapping in `sources/PROVENANCE.md`). Line numbers in
citations below are valid for both the pinned tree and the local copy of the same
basename. `clips/` holds verbatim excerpts with interpretation kept separate.

## Question → evidence

**Q1 — exact `remote_module_pattern` regex and accepted extensions**
- `sources/kit-3.0.0-next.28__src-exports-vite-utils.js:144` — `export const remote_module_pattern = /[/.]remote\.[^/]+$/;` (no extension allowlist; any non-slash extension after `remote.`).
- Consumers: `sources/kit-3.0.0-next.28__src-exports-vite-plugins-remote.js:97–102` (transform filter + `is_remote_module` re-check), `sources/kit-3.0.0-next.28__src-exports-vite-index.js:479` (optimizeDeps resolveId filter).
- Narrower guard regex (flag-off path): `...-plugins-remote.js:226–230` over `moduleExtensions` default `['.js', '.ts']` at `sources/kit-3.0.0-next.28__src-core-config-options.js:164`.
- Clip: `clips/clip-01-remote-module-pattern.md` (verbatim utils.js:144–196).
- **Supported** (source facts); the `.remote.js`-vs-`.remote.ts` symmetry claim is supported by `[/.]remote\.[^/]+$`.

**Q2 — node_modules treatment and peer-dependency condition**
- `sources/kit-3.0.0-next.28__src-exports-vite-utils.js:158–164` — posixify, pattern test, `if (!id.includes('node_modules')) return true;` (161), else `can_export_remote_module(path.dirname(id))` (163).
- `...-utils.js:170–193` — upward `package.json` walk; condition at 180: `pkg?.peerDependencies?.['@sveltejs/kit']`; base cases 183–188; memo map 150/171–172/191.
- Clip: `clips/clip-01-remote-module-pattern.md`.
- **Supported.** Note (supported): the guard in Q3 does *not* consult `is_remote_module`.

**Q3 — `plugin_remote` behaviour and role of `plugin_remote_guard`**
- `sources/kit-3.0.0-next.28__src-exports-vite-plugins-remote.js:22` (signature), `:53–55` (active only when flag on, not serviceWorker env).
- Server branch `:110–156` — registers module (`init_remote_functions`, `fn.__.id = hash + '/' + name`, 129–134); prod `emitFile` chunk `remote-<hash>` `:139–150`; virtual entry `\0sveltekit-remote:<hash>` load `:71–95`.
- Client branch `:158–209` — export-type discovery (dev runner 166–173; prod `build_metadata.remotes` 176–183); stub built by `create_exported_declarations(...)` `:185–189` (impl `sources/kit-3.0.0-next.28__src-core-env.js:404–444`); relative import of kit client runtime `:191–194`.
- Guard `:218–237` — active only when flag **off** (`:222–224`), filter `.remote(<moduleExtensions>)$` `:226–230`, throws `error_for_missing_config('remote functions', 'experimental.remoteFunctions', 'true')` `:233`; message text `sources/kit-3.0.0-next.28__src-exports-vite-utils.js:216–233`.
- Clip: `clips/clip-02-plugin-remote-and-guard.md` (verbatim, all ranges above).
- **Supported.**

**Q4 — how `src/exports/vite/index.js` gates externalization; behaviour when flag off**
- Gate: `sources/kit-3.0.0-next.28__src-exports-vite-index.js:468–490` — comment `// externalize .remote.js files to stop dependency tracing during prebundling` (468), `if (kit.experimental.remoteFunctions)` (469), plugin `vite-plugin-sveltekit-setup:optimize-remote-functions` (477) returning `{ id: to_fs(resolved.id), external: 'absolute' }` for remote modules (479–485), inside `plugin_setup.config` (hook begins `:329`).
- Plugin registration: `...-index.js:643–656` — `plugin_remote_guard(...)` at 648, `plugin_remote(...)` at 649–656.
- Flag off (supported): the optimizeDeps externalizer is not added (469 false); `plugin_remote.applyToEnvironment` returns false (`plugins/remote.js:53–55`); `plugin_remote_guard` applies (`:222–224`) and throws the config error on any id ending `.remote.js`/`.remote.ts` (`:226–234`) — it does not test `is_remote_module`.
- **Supported.**

**Q5 — how `src/core/generate_manifest/index.js` records remote metadata**
- `sources/kit-3.0.0-next.28__src-core-generate_manifest-index.js:14–33` — opts include `remotes: RemoteChunk[]` (22, type at `sources/kit-3.0.0-next.28__src-types-internal.d.ts:221–224`).
- `:65` — `const loader = (path) => `__memo(() => import('${path}'))`;`; `:117–119` — `remotes: { '<hash>': __memo(() => import('<relative>/chunks/remote-<hash>.js')) , ... }`.
- Hash origin: `plugins/remote.js:104–107` (`hash(posixify(path.relative(root, id)))`), algorithm `sources/kit-3.0.0-next.28__src-utils-hash.js:5–19` (djb2 → base36). Chunk filename: emit name `remote-<hash>` (`plugins/remote.js:146`) × SSR `chunkFileNames: 'chunks/[name].js'` (`sources/kit-3.0.0-next.28__src-exports-vite-build-index.js:291`).
- Manifest writes: `...-build-index.js:478–487`, `:767–779`, `:840–849`. Dev equivalent: `sources/kit-3.0.0-next.28__src-exports-vite-dev-generate_manifest.js:168–174` (`runner.import(remote.file)` keyed by hash).
- Names/types are **not** in `generate_manifest` output; they land in analysis metadata: `sources/kit-3.0.0-next.28__src-core-postbuild-analyse.js:141–158` (`metadata.remotes.set(remote.hash, exports)`), consumed by the client branch at `plugins/remote.js:176–183`.
- **Supported**, with the correction above: manifest records hash→chunk-loader only.

**Q6 — runtime client resolving a remote by name: module path and hash**
- Generated stub: `plugins/remote.js:185–199` — `export const <name> = __remote.<type>('<hash>/<name>')` with `import * as __remote from '<relative path to runtime_directory>/client/remote-functions/index.js'`.
- Fetch: `sources/kit-3.0.0-next.28__src-runtime-client-remote-functions-query-index.js:12–41`, URL at `:27` `` `${base}/${app_dir}/remote/${id}${payload ? `?payload=${payload}` : ''}` `` where `id = <hash>/<name>`; `remote_request` → `fetch` at `...-shared.svelte.js:114–115`.
- Server side of the same id: `sources/kit-3.0.0-next.28__src-runtime-server-remote-functions.js:165–175` (`id.split('/')` → `remotes[hash]()`), prefix helpers `:588–604`, form-post variant `:541–544`; the `remotes[hash]` loader imports `chunks/remote-<hash>.js` (`generate_manifest/index.js:117–119`) or, in dev, the original file (`dev/generate_manifest.js:168–174`).
- **Inference (labelled):** the browser never fetches a remote JS chunk itself; "module path + hash" in the client means the `<hash>/<name>` id carried into the `/remote/` request URL — no source line shows the client importing a `chunks/remote-*.js` asset. The chunk import is server-side.

## Contradictions / tensions

- Pattern asymmetry (supported): `remote_module_pattern` accepts *any* extension (`utils.js:144`) while the flag-off guard only matches configured `moduleExtensions` (`plugins/remote.js:226–230`, default `['.js', '.ts']`, `config/options.js:164`) — so e.g. `.remote.mjs` would be neither guarded nor, by extension, mis-claimed by the guard.
- Q5's framing ("names, hashes, chunks") vs. source: only hashes and chunk paths are in the manifest; names+types go to `metadata.remotes` (`analyse.js:141–158`).

## Unresolved

- No build/runtime was executed in this research pass (disallowed), so no observed output of the manifest or generated stub is captured here — the above is source-derived only.
- Exact hash values for specific app files are not computed (would require running kit's `hash`).
- Whether any kit client code path *other* than the remote-functions fetchers consumes `manifest.remotes` was not exhaustively searched (client grep covered `src/runtime/client/` only).
