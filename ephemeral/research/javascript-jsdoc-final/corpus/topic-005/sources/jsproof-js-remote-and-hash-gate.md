# JS-only probe (jsproof): JS remote compiles; hash id changes; adapter gate text
# origin: /tmp/skgo-jsdoc-jsproof-20260926 (built artifacts read 2026-09-26)
# retrieved: 2026-09-26 (file reads only; no build or test was run in this session)
# location: paths and line numbers given per excerpt (`cat -n` at retrieval).
# --- verbatim ---

## 1. Kit compiled the .js remote, with a different module hash than the .ts one

`.svelte-kit/output/server/manifest.js` lines 19-21:

```js
		remotes: {
			'2b61k': __memo(() => import('./chunks/remote-2b61k.js'))
		},
```

Compare the demo probe's TS build: `'1ptltty'` for
`src/routes/example.remote.ts` (see `demo-client-and-server-remote-resolution.md`
§4). The hash input is the module path *including its extension*, so
`.ts` → `1ptltty` and `.js` → `2b61k` are two different ids for the same two
functions.

`.svelte-kit/output/server/chunks/remote-2b61k.js` (whole file):

```js
import { t as example_remote_exports } from "./example.remote.js";
//#region \0sveltekit-remote:2b61k
var _sveltekit_remote_2b61k_default = example_remote_exports;
//#endregion
export { _sveltekit_remote_2b61k_default as default };
```

`.svelte-kit/output/server/chunks/example.remote.js` — observed region marker
and call (grep of `region|init_remote_functions|__.id`):

    16://#region src/routes/example.remote.js
    37:init_remote_functions(example_remote_exports, "src/routes/example.remote.js", "2b61k");
    39:	fn.__.id = "2b61k/" + name;

`.svelte-kit/output/server/.vite/manifest.json` lines 105-113:

```json
  "sveltekit-remote:2b61k": {
    "file": "chunks/remote-2b61k.js",
    "name": "remote-2b61k",
    "src": "sveltekit-remote:2b61k",
    "isEntry": true,
    "imports": [
      "_example.remote.js"
    ]
  }
```

## 2. The shipped client carries the NEW ids

`build/client/_app/immutable/nodes/2.DJQfIP-v.js`:

    $ grep -o '2b61k/[a-z]*' … → 2b61k/record, 2b61k/status
    $ grep -o '1ptltty/[a-z]*' … → (no matches)

## 3. `skgo generate` was re-run after the JS files were written

File mtimes (`stat -f '%Sm %N'`):

    2026-09-26 11:56:03  web/src/routes/types.js
    2026-09-26 11:56:32  web/src/routes/example.remote.js
    2026-09-26 11:58:22  web/skgo.remotes.json
    2026-09-26 11:58:22  internal/skgo/skgo_bindings_gen.go
    2026-09-26 11:58:28  web/build/skgo.manifest.json
    2026-09-26 11:58:28  web/build/client/_app/immutable/nodes/2.DJQfIP-v.js
    2026-09-26 11:58:28  web/.svelte-kit/output/server/manifest.js

`web/skgo.remotes.json` now lists `"2b61k/record"`, `"2b61k/status"`, and
`internal/skgo/skgo_bindings_gen.go` now has `Module: "src/routes/example.remote.js"`
(lines 66 and 74) — i.e. the generator's own record of the module path follows
the file extension of the source it found.

## 4. PREVIOUSLY OBSERVED (reported by the operator on 2026-09-26; exit status
## NOT preserved in any file — treat as an unverified prior observation)

- `pnpm run check` passed on this app.
- `record(123)` failed as desired (the JS stub's `command` path rejected the
  wrong argument shape — or the Go handler did; which of the two produced the
  failure is not recorded).
- `pnpm run build` compiled the JS remote (kit's `2b61k` output above is the
  residue of that compile) but the adapter rejected "old versus new remote
  path hash".

No captured stdout/stderr, log file, or test-output file for any of these three
observations exists in `/tmp/skgo-jsdoc-jsproof-20260926`. The only `.log` is
`web/debug-storybook.log`, a Storybook initialisation log (see
`logs-and-absences.md`). Exit codes and the adapter's exact error text are
therefore unresolved.

## 5. The adapter text that matches "old versus new remote path hash"

From the current worktree `internal/adapter/skgo-adapter.js` (this is source,
not a captured run; quoted in `adapter-check-and-manifest-source.md`): the
`checkRemoteHashes` failure message prints

    skgo: skgo.remotes.json does not describe the remote modules kit just compiled.
      generated but not compiled: <hash>
      compiled but not generated: <hash>
      A generated .remote.ts only reaches the build once app code imports it. Run `go generate ./...`.

A `.js`-compiled build checked against a `.ts`-generated `skgo.remotes.json`
would print exactly that with `1ptltty` on one side and `2b61k` on the other.
Inference: that is the failure the operator saw. The literal error string from
the run was not captured, so the inference is not proof.
