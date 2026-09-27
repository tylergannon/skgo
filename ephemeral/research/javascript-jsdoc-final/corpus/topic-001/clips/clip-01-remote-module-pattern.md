# Clip 01 — remote-module pattern, is_remote_module, and the node_modules peer-dependency rule

Evidence for topic-001 questions Q1 and Q2.
Origin: `/Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.28/src/exports/vite/utils.js`
(local copy: `sources/kit-3.0.0-next.28__src-exports-vite-utils.js`, byte-identical, retrieval 2026-09-26).

## Excerpt — verbatim, utils.js lines 144–193

```js
export const remote_module_pattern = /[/.]remote\.[^/]+$/;

/**
 * A cache of which directories can export remote modules
 * @type {Map<string, boolean>}
 */
const remote_module_cache = new Map();

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

/**
 * @param {string} directory
 * @returns {boolean}
 */
function can_export_remote_module(directory) {
	let cached = remote_module_cache.get(directory);
	if (cached !== undefined) return cached;

	let pkg;

	try {
		pkg = JSON.parse(fs.readFileSync(path.join(directory, 'package.json'), 'utf8'));
	} catch {}

	if (pkg?.peerDependencies?.['@sveltejs/kit']) {
		cached = true;
	} else {
		const parent = path.dirname(directory);

		cached =
			path.basename(directory) === 'node_modules' || parent === directory
				? false // base case
				: can_export_remote_module(parent); // recurse
	}

	remote_module_cache.set(directory, cached);
	return cached;
}
```

## Excerpt — verbatim, utils.js lines 195–196 (sibling patterns, for contrast)

```js
export const server_only_module_pattern = /[/.]server\.[^/]+$/;
export const server_only_directory_pattern = /\/server\//;
```

## Annotation (researcher interpretation — not source text)

- `remote_module_pattern` (line 144) is `/[/.]remote\.[^/]+$/`. It carries no
  extension allowlist: the extension after `remote.` is `[^/]+$`, so `.remote.ts`,
  `.remote.js`, `.remote.mjs` etc. all match as long as the id ends with
  `.remote.<non-slash run>` preceded by `.` or `/`.
- `is_remote_module` (158–164): posixify, test the pattern, then if the id does
  **not** contain `node_modules` → `true` (line 161); otherwise it defers to
  `can_export_remote_module` (163, defined 170–193).
- The peer-dependency condition (line 180) is exactly
  `pkg?.peerDependencies?.['@sveltejs/kit']` — truthy is enough; version ranges
  are not inspected. The walk goes up one directory at a time and stops
  (returns `false`) when the directory basename is `node_modules` or the parent
  is itself (lines 183–188). Results are memoised in `remote_module_cache` (150, 171–172, 191).
- Related but distinct: `server_only_module_pattern = /[/.]server\.[^/]+$/` (195).

**Status:** directly supported by the excerpt above; no inference used.
