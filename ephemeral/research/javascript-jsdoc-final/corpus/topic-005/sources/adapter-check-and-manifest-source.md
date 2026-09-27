# skgo adapter: manifest identity, hash check, and extension gates (worktree source)
# origin: /Users/tyler/.codex/worktrees/ef7a/skgo (skgo worktree, git HEAD read 2026-09-26)
#           + /Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28
# retrieved: 2026-09-26 (reads only; no build or test was run)
# location: line numbers per excerpt.
# --- verbatim ---

## internal/adapter/skgo-adapter.js lines 16-21 (identity the manifest carries)

```js
// Which skgo this adapter is: the version this package was published at, and a
// fingerprint taken over its own files. The Go that reads the manifest below
// takes the same fingerprint over the copy it embeds and refuses a build whose
// adapter is not its own, so an install that has fallen behind the binary is
// named at startup instead of failing later as something unrelated.
const SKGO = identity();
```

## internal/adapter/skgo-adapter.js lines 67-81 (adapt() order of gates)

```js
		async adapt(builder) {
			rmSync(out, { force: true, recursive: true });

			const generated = readGenerated();
			const { manifest: kit, source } = await readKitManifest(builder);
			const hashes = checkRemoteHashes(kit, generated.remotes);

			const nodes = await readNodes(builder, source, kit);
			const serverIds = nodes.map((node) => node.server);
			checkServerLoads(serverIds, generated.loads, generated.actions);

			const endpoints = checkEndpoints(builder, generated.endpoints);

			builder.writeClient(`${out}/client`);
			checkRemoteIds(`${out}/client`, generated.remotes, hashes);
```

`checkRemoteHashes` runs before anything is written to `out`, so a hash
mismatch aborts the build with `out` already removed (line 68 `rmSync`).

## internal/adapter/skgo-adapter.js lines 634-652 (doc comment, verbatim)

```js
/**
 * Kit's own manifest lists the remote *modules* it compiled, keyed by the hash
 * it derived from each module's path. If that set is not the one `skgo
 * generate` wrote, the two halves of the app were generated from different
 * sources and the build must not succeed.
 *
 * Modules are as far as this check can reach. `generate_manifest` in kit's
 * packages/kit/src/core/generate_manifest/index.js emits
 * `'<hash>': __memo(() => import('./chunks/remote-<hash>.js'))` and nothing
 * else; kit resolves the function half of an id at *runtime*, by looking the
 * name up on the imported module namespace
 * (packages/kit/src/runtime/server/remote-functions.js). There is no
 * per-function list in the manifest to compare against. checkRemoteIds below
 * reads the other build artifact that does carry the names.
```

## internal/adapter/skgo-adapter.js lines 653-673 (the failure text)

```js
function checkRemoteHashes(kit, remotes) {
	if (!kit._ || typeof kit._.remotes !== 'object' || kit._.remotes === null) {
		throw new Error(
			"skgo: kit's generated manifest has no `remotes` map. The adapter cannot tell which remote modules were compiled, so it cannot check them; this build of SvelteKit is not one skgo has been taught to read."
		);
	}
	const built = new Set(Object.keys(kit._.remotes));
	const declared = new Set(remotes.map((id) => id.split('/')[0]));

	const missing = [...declared].filter((hash) => !built.has(hash));
	const extra = [...built].filter((hash) => !declared.has(hash));
	if (missing.length || extra.length) {
		throw new Error(
			'skgo: skgo.remotes.json does not describe the remote modules kit just compiled.\n' +
				(missing.length ? `  generated but not compiled: ${missing.join(', ')}\n` : '') +
				(extra.length ? `  compiled but not generated: ${extra.join(', ')}\n` : '') +
				'  A generated .remote.ts only reaches the build once app code imports it. Run `go generate ./...`.'
		);
	}
	return built;
}
```

Note the hard-coded `.remote.ts` in the remediation sentence: an extension
assumption baked into the message, not into the comparison (the comparison
itself is over hashes only).

## internal/adapter/skgo-adapter.js lines 677-688 (checkRemoteIds' framing)

```js
 * The module-level check above is blind to an extra export added by hand to an
 * already-generated `.remote.ts`: the module's hash does not change, so both
 * sides still agree, the bundle calls `<hash>/<newName>`, and the Go binary —
 * which was never told about it — answers 404 in the browser.
 *
 * Kit's client carries the full id as a literal. Its vite plugin rewrites every
 * remote module's client half into one `export const <name> =
 * __remote.<type>('<hash>/<name>')` per export
 * (packages/kit/src/exports/vite/index.js), and the client runtime pastes that
 * id straight into `${base}/${app_dir}/remote/${id}`
 * (packages/kit/src/runtime/client/remote-functions/query/index.js).
```

(These two paragraphs corroborate §1-§2 of
`demo-client-and-server-remote-resolution.md`: id literals in the client, no
chunk fetch.)

## internal/adapter/skgo-adapter.js lines 701-731 — checkRemoteIds (verbatim body)

```js
function checkRemoteIds(clientDir, remotes, hashes) {
	const declared = new Set(remotes);
	const called = new Map();

	for (const file of walk(clientDir)) {
		if (!file.endsWith('.js')) continue;
		const source = readFileSync(file, 'utf-8');
		for (const [, hash, name] of source.matchAll(
			/['"`]([A-Za-z0-9]{1,16})\/([A-Za-z_$][A-Za-z0-9_$]*)['"`]/g
		)) {
			if (!hashes.has(hash)) continue;
			called.set(`${hash}/${name}`, file);
		}
	}

	if (called.size === 0 && hashes.size > 0) {
		throw new Error(
			`skgo: kit compiled ${hashes.size} remote module(s) but no remote-function id appears in ${clientDir}. Either nothing imports them, or the ids no longer survive bundling as literals and this check has stopped meaning anything.`
		);
	}

	const undeclared = [...called].filter(([id]) => !declared.has(id));
	if (undeclared.length) {
		throw new Error(
			'skgo: the built frontend calls remote functions that `skgo generate` did not write.\n' +
				undeclared.map(([id, file]) => `  ${id} (in ${file})`).join('\n') +
				'\n  Go answers only the generated ids, so every one of these would 404 in the browser.\n' +
				'  A remote function written by hand in a .remote.ts is not a supported mode: write it in the matching .remote.go and run `go generate ./...`.'
		);
	}
}
```

## internal/adapter/skgo-adapter.js — manifest write (lines 120-140, verbatim)

```js
			write(
				`${out}/skgo.manifest.json`,
				JSON.stringify(
					{
						// Which skgo wrote this. `ReadManifest` refuses a build
						// whose adapter is not the one the reading module carries.
						skgo: SKGO.version,
						skgoAdapter: SKGO.adapter,
						appDir: builder.config.appDir,
						base: builder.config.paths.base,
						version: builder.config.version.name,
						trustedOrigins: builder.config.csrf.trustedOrigins,
						// One entry per node, positionally: the vite-root-relative
						// path of its `+*.server.ts`, or "" for a node that has
						// none. It is how Go finds the load that answers a slot of
						// a route's branch, and it is the same key kit itself
						// records in the node module it builds.
						nodes: serverIds,
						loads: generated.loads,
						actions: [...new Set(generated.actions)].sort(),
						ssr: describeSSR(builder, kit, nodes),
```

Written at the END of `adapt()` (line 120 is after `goja.build`, `writeAppManifest`
and `builder.compress`), so a manifest present in `build/` means the run reached
the end of `adapt()`; a run that threw in `checkRemoteHashes` leaves no
`skgo.manifest.json` (the dir was removed at line 68).

## internal/gen/adapter.go lines 12-44 (checkInstalledAdapter, verbatim, first half)

```go
// checkInstalledAdapter refuses to generate against an `@skgo/sveltekit-adapter` that is
// not this module's.
//
// The unbypassable gate is at startup: the manifest the adapter writes names
// which adapter wrote it, and `skgo.ReadManifest` refuses any other. That check
// cannot be moved earlier, because a developer can swap `node_modules` after
// generating and only the binary reading its own fingerprint against what
// actually built catches that.
//
// This is the convenience in front of it. A mismatched install is the ordinary
// failure — a `pnpm install` that resolved an older release than the `go.mod`
// asks for — and the difference between learning that here, in a second, and
// learning it after a full frontend build and a server start is the whole cost
// of the mistake.
//
// An app with nothing installed yet is not a mismatch and is not reported: the
// vite build says `Cannot find package '@skgo/sveltekit-adapter'` perfectly well, and a
// `go generate` that refused to run before an install would be wrong about a
// tree that is merely in the wrong order.
func checkInstalledAdapter(cfg Config) error {
	dir := filepath.Join(cfg.Web, "node_modules", filepath.FromSlash(adapter.Package))

	installed, err := adapter.FingerprintOf(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("skgo: reading the %s installed in %s: %w", adapter.Package, cfg.Web, err)
	}
	if installed == adapter.Fingerprint() {
		return nil
	}
```

Its failure text (lines 58-69, verbatim):

```go
	return fmt.Errorf(
		"skgo: the %s installed in this app is not the one this skgo publishes.\n"+
			"\tinstalled in %s: %s\n"+
			"\tthis program:  %s\n"+
			"They are one contract in two languages, released together, so the frontend this "+
			"would build could not be served by the program that generated it.\n"+
			"Install the matching one — %s — and run `go generate ./...` again.",
		adapter.Package,
		dir,
		adapter.Identity(version, installed),
		adapter.Identity(adapter.Version(), adapter.Fingerprint()),
		install)
```

This gate compares adapter *fingerprints only*; it says nothing about file
extensions or remote hashes.

## Go symbol locations (grep, current worktree)

    ./internal/adapter/adapter.go:87:func Fingerprint() string
    ./internal/adapter/adapter.go:107:func FingerprintOf(dir string) (string, error)
    ./internal/adapter/adapter.go:203:func Identity(version, fingerprint string) string
    ./internal/gen/gen.go:77:	if err := checkInstalledAdapter(cfg); err != nil {
    ./internal/gen/check.go:27:	if err := checkInstalledAdapter(cfg); err != nil {
    ./remote.go:498:func ReadManifest(build fs.FS) (Manifest, error)

`internal/kithash/kithash.go` lines 1-7 (package doc, verbatim):

```go
// Package kithash reproduces SvelteKit's djb2 string hash.
//
// Ported from `packages/kit/src/utils/hash.js`: the hash walks the string's
// UTF-16 code units backwards, and the result is the unsigned 32-bit value in
// base 36. Remote function ids are `Kit(vite-root-relative posix path)` joined
// to the export name with a slash.
package kithash
```

and `Kit` (lines 15-24):

```go
func Kit(s string) string {
	units := utf16.Encode([]rune(s))

	h := int32(5381)
	for i := len(units) - 1; i >= 0; i-- {
		h = int32(int64(h)*33) ^ int32(units[i])
	}

	return strconv.FormatUint(uint64(uint32(h)), 36)
}
```

`internal/kithash/kithash_test.go` lines 15-16 — the first two goldens, one
`.ts` and one `.js` path, both hashed by extension-sensitive input:

```go
		{"src/lib/todos.remote.ts", "worolc"},
		{"src/routes/data.remote.js", "mxe8u8"},
```

`internal/gen/emit.go` line 745 (how an id reaches `skgo.remotes.json`):

```go
		list.Remotes = append(list.Remotes, kithash.Kit(fn.module)+"/"+fn.name)
```

## kit@3.0.0-next.28 `src/exports/vite/utils.js` — pattern and node_modules rule

Lines 144-193 (verbatim):

```js
export const remote_module_pattern = /[/.]remote\.[^/]+$/;
```

```js
/**
 * Whether `id` is a remote module. Files in node_modules only count if the
 * package they belong to has a peer dependency on `@sveltejs/kit`
 * @param {string} id
 * @returns {boolean}
 */
export function is_remote_module(id) {
	id = posixify(id);
	if (!remote_module_pattern.test(id)) return false;
	if (!id.includes('node_modules')) return true;

	return can_export_remote_module(path.dirname(id));
}
```

`can_export_remote_module` walks up from the file's directory looking for a
`package.json` with `peerDependencies['@sveltejs/kit']`; it returns false at
`node_modules` or the filesystem root (lines 183-188).

`package.json` line: `"version": "3.0.0-next.28"`.

The pattern `[/.]remote\.[^/]+$` accepts `example.remote.ts`, `example.remote.js`,
`example.remote.mjs` — any extension — which is consistent with the jsproof
build compiling `src/routes/example.remote.js`.
