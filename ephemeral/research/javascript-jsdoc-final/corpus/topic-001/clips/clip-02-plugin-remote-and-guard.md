# Clip 02 — plugin_remote and plugin_remote_guard

Evidence for topic-001 question Q3.
Origin: `/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28/src/exports/vite/plugins/remote.js`
(local copy: `sources/kit-3.0.0-next.28__src-exports-vite-plugins-remote.js`, byte-identical, retrieval 2026-09-26).

## Excerpt — verbatim, lines 22 (signature), 53–55 (activation)

```js
export function plugin_remote(svelte_config, get_config, get_build_metadata, set_remote_metadata) {
		applyToEnvironment(environment) {
			return svelte_config.experimental.remoteFunctions && environment.name !== 'serviceWorker';
		},
```

## Excerpt — verbatim, lines 71–95 (virtual entry module `\0sveltekit-remote:`)

```js
		// prevent other plugins from resolving our remote virtual module
		resolveId: {
			filter: {
				id: prefixRegex('\0sveltekit-remote:')
			},
			handler(id) {
				return id;
			}
		},

		load: {
			filter: {
				id: prefixRegex('\0sveltekit-remote:')
			},
			handler(id) {
				// On-the-fly generated entry point for remote file just forwards the original module
				// We're not using manualChunks because it can cause problems with circular dependencies
				// (e.g. https://github.com/sveltejs/kit/issues/14679) and module ordering in general
				// (e.g. https://github.com/sveltejs/kit/issues/14590).
				const hash_id = id.slice('\0sveltekit-remote:'.length);
				const original = remote_original_by_hash.get(hash_id);
				if (!original) throw new Error(`Expected to find metadata for remote file ${id}`);
				return `import * as m from ${s(original)};\nexport default m;`;
			}
		},
```

## Excerpt — verbatim, lines 97–111 and 118–156 (transform: discovery + server branch)

```js
		transform: {
			filter: {
				id: remote_module_pattern
			},
			async handler(code, id) {
				if (!is_remote_module(id)) return;

				const file = posixify(path.relative(root, id));
				const remote = {
					hash: hash(file),
					file
				};

				if (this.environment.config.consumer === 'server') {
					remotes.push(remote);
					const ms = new MagicString(code);

					// Extra newlines to prevent syntax errors around missing semicolons or comments
					ms.append(
						'\n\n' +
							dedent`
								import * as $$_self_$$ from './${path.basename(id)}';
								import { init_remote_functions as $$_init_$$ } from '@sveltejs/kit/internal/server';

								${dev_server ? 'await Promise.resolve()' : ''}

								$$_init_$$($$_self_$$, ${s(file)}, ${s(remote.hash)});

								for (const [name, fn] of Object.entries($$_self_$$)) {
									fn.__.id = ${s(remote.hash)} + '/' + name;
									fn.__.name = name;
								}
							`
					);

					// Emit a dedicated entry chunk for this remote in SSR builds (prod only)
					if (!dev_server) {
						remote_original_by_hash.set(remote.hash, id);

						if (!emitted_remote_hashes.has(remote.hash)) {
							this.emitFile({
								type: 'chunk',
								id: `\0sveltekit-remote:${remote.hash}`,
								name: `remote-${remote.hash}`
							});
							emitted_remote_hashes.add(remote.hash);
						}
					}

					return {
						code: ms.toString(),
						map: ms.generateMap({ hires: 'boundary' })
					};
				}
```

## Excerpt — verbatim, lines 158–209 (client branch: generated fetch stub)

```js
				// For the client, read the exports and create a new module that only contains fetch functions with the correct metadata

				/** @type {Map<string, RemoteInternals['type']>} */
				const map = new Map();

				// in dev, load the server module here (which will result in this hook
				// being called again with `opts.ssr === true` if the module isn't
				// already loaded) so we can determine what it exports
				if (dev_server) {
					const module = await get_runner(vite, dev_server).import(id);

					for (const [name, value] of Object.entries(module)) {
						const type = value?.__?.type;
						if (type) map.set(name, type);
					}
				}
				// in prod, we already built and analysed the server code before
				// building the client code, so `remotes` is populated
				else if (build_metadata?.remotes) {
					const exports = build_metadata.remotes.get(remote.hash);
					if (!exports) throw new Error('Expected to find metadata for remote file ' + id);

					for (const [name, value] of exports) {
						map.set(name, value.type);
					}
				}

				const { namespace, declarations, reexports } = create_exported_declarations(
					map.keys(),
					(name, ns) => `${ns}.${map.get(name)}('${remote.hash}/${name}')`,
					'__remote'
				);

				const relative = posixify(
					path.relative(path.dirname(id), `${runtime_directory}/client/remote-functions/index.js`)
				);

				let result = `import * as ${namespace} from '${relative}';\n\n${declarations.join('\n')}`;
				if (reexports.length > 0) {
					result += `\nexport { ${reexports.join(', ')} };`;
				}
				result += '\n';

				if (dev_server) {
					result += `\nimport.meta.hot?.accept();\n`;
				}

				return {
					code: result,
					map: null
				};
			}
```

## Excerpt — verbatim, lines 218–237 (plugin_remote_guard)

```js
export function plugin_remote_guard(svelte_config) {
	return {
		name: 'vite-plugin-sveltekit-remote-guard',

		applyToEnvironment() {
			return !svelte_config.experimental.remoteFunctions;
		},

		transform: {
			filter: {
				id: new RegExp(
					`.remote(${svelte_config.moduleExtensions.join('|')})$`.replaceAll('.', '\\.')
				)
			},
			handler() {
				error_for_missing_config('remote functions', 'experimental.remoteFunctions', 'true');
			}
		}
	};
}
```

## Annotation (researcher interpretation — not source text)

- `plugin_remote` is a `transform` plugin filtered on `remote_module_pattern`
  (97–100) that re-checks `is_remote_module(id)` (102). For every matched module
  it computes `file = posixify(path.relative(root, id))` and
  `hash = hash(file)` (104–107; `hash` is djb2 → base36, `src/utils/hash.js:5–19`).
- **Server consumer branch (110–156):** appends registration code that calls
  `init_remote_functions` and stamps every export with
  `fn.__.id = <hash> + '/' + name` (129–134). In non-dev builds it records
  `remote_original_by_hash` and `emitFile({ type: 'chunk', id: '\0sveltekit-remote:<hash>',
  name: 'remote-<hash>' })` (139–150) — one dedicated SSR entry chunk per remote.
- **Virtual module (71–95):** `\0sveltekit-remote:<hash>` loads as
  `import * as m from '<original id>'; export default m;`.
- **Client consumer branch (158–209):** discovers each export's remote type
  (dev: imports the server module through the vite-node runner, 166–173;
  prod: reads `build_metadata.remotes.get(hash)`, 176–183) and emits a stub
  module whose exports are `__remote.<type>('<hash>/<name>')` built by
  `create_exported_declarations` (185–189; implementation `src/core/env.js:404–444`),
  importing kit's client runtime via a **relative path** computed from the
  `.remote.*` file's directory to `<runtime_directory>/client/remote-functions/index.js`
  (191–194). Dev adds `import.meta.hot?.accept()` (201–203).
- `plugin_remote_guard` (218–237) is active only when the flag is **off**
  (`applyToEnvironment` returns `!experimental.remoteFunctions`, 222–224). Its
  transform filter is `` `.remote(${svelte_config.moduleExtensions.join('|')})$` ``
  with `.` escaped (228–230), i.e. by default it matches ids ending `.remote.js`
  or `.remote.ts` (`moduleExtensions` default `['.js', '.ts']`,
  `src/core/config/options.js:164`) — narrower than `remote_module_pattern`.
  On match it throws `error_for_missing_config('remote functions',
  'experimental.remoteFunctions', 'true')` (233), whose message text is built in
  `src/exports/vite/utils.js:216–233`. Unlike `plugin_remote`, the guard does not
  call `is_remote_module`, so the node_modules peer-dep rule does not exempt a
  file from the guard.

**Status:** directly supported by the excerpts; the line-102/229 comparison and
the "guard does not consult is_remote_module" reading are explicit in the code.
