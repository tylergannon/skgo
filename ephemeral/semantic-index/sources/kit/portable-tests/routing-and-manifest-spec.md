# Routing and manifest specs — route ids, matching, sorting, filesystem scan

## Purpose

skgo's Go router must accept the same request paths, extract the same params, and pick
the same route as kit's, and its route scanner must build the same manifest from
`src/routes` as `create_manifest_data`. These upstream specs are the executable definition:
`utils/routing.spec.js` (pattern compilation, param extraction, path resolution, first-match
lookup), `utils/params.spec.js` (matcher loading — mostly JS), `utils/url.spec.js` (path
normalisation and trailing slash), `core/sync/create_manifest_data/index.spec.js` (route
tree + node indices from fixture directories), `exports/vite/static_analysis/index.spec.js`
(page-option detection), plus small helpers in `utils/http`, `utils/escape`, `utils/error`,
`exports/url`, `exports/hooks/sequence`.

## Key facts (SOURCE-DERIVED)

### `parse_route_id` — route id → regex + param list (`utils/routing.js:L35-L117`)

- `/` and root groups `/(group)` compile to `^\/$` (L39-L42, `root_group_pattern` L7).
- Otherwise segments from `get_route_segments(id)` (`id.slice(1).split('/')` minus `''`
  and `(group)` segments, L123-L136) are compiled and joined, then `/?$` appended:
  - whole-segment rest `[...name(=matcher)?]` → `(?:/([^]*))?`, param
    `{rest:true, optional:false, chained:true}` (L46-L56)
  - whole-segment optional `[[name(=matcher)?]]` → `(?:/([^/]+))?`, `{optional:true, chained:true}` (L58-L68)
  - mixed segment: split on `\[(.+?)\](?!\])`; odd parts are params (`[x+hh]`/`[u+hhhh]`
    escapes decoded and regex-escaped, L78-L80); param regex `^(\[)?(\.\.\.)?([\w-]+)(?:=([\w-]+))?(\])?$`
    (L5); emits `([^]*?)` for rest, `([^/]*)?` for optional, `([^/]+?)` for required;
    `chained` for inline rest is `i === 1 && parts[0] === ''` (L85-L104)
  - static text: `normalize()`d then `%`→`%25`, `/`→`%2[Ff]`, `?`→`%3[Ff]`, `#`→`%23`, rest
    regex-escaped (`escape`, L252-L267; `escape_for_regexp` in `utils/regex.js:L6-L9`)
- Spec table (`utils/routing.spec.js:L10-L120`) — 19 ids with expected `pattern.toString()`
  and params, e.g. `'/blog/[slug]'` → `/^\/blog\/([^/]+?)\/?$/`; `'/blog/[[slug]].json'` →
  `/^\/blog\/([^/]*)?\.json\/?$/` with `chained:false`; `'/[...catchall]'` → `/^(?:\/([^]*))?\/?$/`;
  `'/[x+5b]'` → `/^\/\[\/?$/`. Hyphenated names and matchers allowed (L60-L101).

### `exec` — match groups → params (`utils/routing.js:L173-L246`)

- Iterates params with a `buffered` counter for chained optional params whose matcher
  failed; a chained rest param absorbs buffered values joined by `/` (L188-L195); rest
  gets `''` when unmatched so its matcher can still run (L198-L205); values are
  `decodeURIComponent`-ed (L207); matcher failure on a chained optional increments
  `buffered` and continues, otherwise the route does not match (L209-L222); leftover
  `buffered` → no match (L244).
- Matchers are standard-schema objects; `run_matcher` throws
  `'Async param matchers are not supported'` on a Promise and
  `'Param matcher must return a string, number, boolean, or bigint'` otherwise (L143-L166).
- Spec table (`utils/routing.spec.js:L122-L358`): 44 `(route, path, expected)` rows using
  matchers `matches` (accept) / `doesntmatch` (reject), including `undefined` outcomes and
  the newline path `'/\n'` (L324-L333). Two extra cases: schema transform to number (L360-L368)
  and rejection (L370-L379).

### `resolve_route` — route id + params → pathname (`utils/routing.js:L294-L338`)

- Replaces `segment_pattern` (escape sequences first, then `[[opt]]`, `[...rest]`, `[name]`,
  L269-L276) per segment; escape sequences become `encode_pathname_chars(decoded)` (`%`,
  `/`, `?`, `#` re-encoded uppercase, L24-L29); missing required → `Missing parameter '<n>' in route <id>`;
  string values starting/ending with `/` throw (L313-L321); number/boolean/bigint
  stringified; empty segments dropped; trailing slash preserved when the id ends with `/`.
- Spec (`L382-L535`): 24 rows incl. `'/[x+2e]well-known/[one]'` → `/.well-known/one`,
  `'/[u+1f600]/[one]'` → `/😀/one`, `'/[x+2f]/[one]'` → `/%2F/one`, param value
  `'[x+2f]'` **not** expanded (L490-L493).

### `find_route` — first match wins (`utils/routing.js:L356-L370`)

- Linear scan over already-sorted routes; spec (`L537-L660`) checks first-match, `null`,
  matcher fall-through (`[slug=word]` then `[slug]`), schema transforms, invalid matcher
  return types (4 error regexes L598-L636), hyphenated matcher names, and
  `decodeURIComponent` of params (`hello%20world` → `hello world`, L653-L658).

### `utils/params.spec.js` (L10-L102)

- `collect_matcher_names` gathers `params[].matcher` into a Set; `validate_param_matchers`
  throws `No matcher found for parameter '<name>'` (also for inherited names like
  `toString`); `load_and_validate_params` loads `params.js`; `normalize_param_definition`
  wraps a function `(param) => value | undefined` into a standard schema (undefined = no
  match, thrown errors propagate, callable schemas pass through). Go: matchers are Go
  functions registered by name; port only the "unknown matcher name fails at build" check.

### `utils/url.spec.js`

- `normalize_path(path, 'ignore'|'always'|'never')` (L123-L152): `/` untouched; `'/foo'`
  → always `/foo/`, never `/foo`; `'/foo/'` → always `/foo/`, never `/foo`.
- `resolve(base, path)` (L11-L75): root-relative, `./`, `../` (clamped at root), protocol-
  relative, absolute, `mailto:`/`data:`, `#foo`, `''` → base, `'.'` → `/a/b/`.
- `relative_pathname(from, to)` (L77-L98): trailing-slash redirects as relative references
  (`/a/b` → `/a/b/` gives `b/`; `/a/b/` → `/a/b` gives `../b`; `%2F` preserved).
- `matches_external_allowlist_entry` (L100-L121): origin+path-prefix match, rejects
  `google.de.evil.com` and `blob:` URLs.
- `make_trackable` / `disable_search` (L154-L267): Proxy-based dependency tracking on
  `event.url` — JS-only, but the messages
  `Cannot access event.url.hash. Consider using \`page.url.hash\` inside a component instead`
  and `Cannot access url.search on a page with prerendering enabled` are user-visible.

### `create_manifest_data` — filesystem → routes/nodes (`core/sync/create_manifest_data/index.js`)

- Walk (L139-L370): per directory, decode `[x+hh]`/`[u+hhhh..]` escapes (lowercase only,
  2 hex for `x`, 4–6 for `u`, L140-L165); reject `][` (`parameters must be separated`),
  unbalanced brackets, `#` in a segment (suggest `[x+23]`), `[[optional]]` after `[...rest]`,
  `[[...rest]]` (L166-L190); `readdirSync().sort()` for determinism (L224-L231); files
  before subdirectories (L233-L367); skip non-`+` files but log
  `Missing route file prefix. Did you mean +<file>?` when the name would match with `+`
  (L239-L258); skip `.test.`/`.spec.`/`.stories.` and matching `.d.ts` (L260-L274);
  `analyze()` classifies by `component_name_pattern` `^\+(?:(page(?:@(.*))?)|(layout(?:@(.*))?)|(error))$`
  and `module_name_pattern` `^\+(?:(server)|(page(?:(@[a-zA-Z0-9_-]*))?(\.server)?)|(layout(?:(@[a-zA-Z0-9_-]*))?(\.server)?))$`
  (L17-L20, L533-L574); `@` on a module file throws
  `Only Svelte files can reference named layouts. Remove '@x' from <file> (at <path>)`;
  duplicates throw `Multiple <type> files found in <routes_base><id> : <a> and <b>`
  (L296-L301) with types `layout component`, `page component`, `<kind> layout module`,
  `<kind> page module`, `endpoint`; `router.type === 'hash'` forbids server files (L286-L289).
- Missing routes dir → single route `{ id:'/', pattern:/^$/ }` (L381-L396); empty routes
  dir → `No routes found...` (L372-L380).
- `prevent_conflicts` (`conflict.js:L2-L53`): normalises ids (groups removed, escapes
  decoded, `[p]`→`<*>`, `[p=m]`→`<m>`, `[[p]]`→`<?*>`, `[...p]`→`<...*>`), expands
  optional permutations, throws `The "<a>" and "<b>" routes conflict with each other`.
- Fallback root layout/error components from `fallback` dir (L400-L410).
- Node index order: all layouts then errors in route order, then leaves (L413-L428) — root
  layout is node 0, root error node 1. `route.page = { layouts: [...], errors: [...], leaf }`
  built by walking parents; `@segment` named-layout references skip up to the route whose
  `segment` matches, else `<component> references missing segment "<name>"` (L435-L477).
  Layout/error arrays contain `undefined` at levels without a layout/error (spec e.g.
  `layouts: [0, 2, undefined, 4], errors: [1, undefined, 3, 5]`, `index.spec.js:L775`).
- Routes without any file are dropped (L481); error nodes get `parent` layout (L484-L496);
  page options computed per node and per endpoint (L498-L506); a route with both
  `+page` and `+server` where either prerenders throws
  `Cannot prerender a route (<id>) with both a \`+page.svelte\` and a \`<+server file>\`` (L508-L516).
- Output routes are `sort_routes(routes)` (L522).

### `sort_routes` (`core/sync/create_manifest_data/sort.js:L18-L131`)

- Compares segment-by-segment after removing non-terminal `[[optional]]` parts
  (`split_route_id`, L135-L142); each segment split into alternating static/dynamic parts.
- Rules at a dynamic position (L84-L119): missing part wins; `[...rest]/x` outranks
  `[...rest]`; rest vs. non-rest: rest wins only if followed by static and the other is not;
  matcher outranks none; `[required]` outranks `[[optional]]`. Static positions: empty
  (shallower) wins, else `sort_static` where `foobar` outranks `foo` (L120-L125, L149-L161).
  Tie → reverse id order (L130).
- Golden order (`index.spec.js:L231-L270`): 28 ids from `'/'` … `'/[required=matcher]'`,
  `'/[required]'`, `'/[...rest]'`; the test shuffles and re-sorts.

### Manifest fixtures (`core/sync/create_manifest_data/index.spec.js`)

Fixture dirs under `reference/kit/packages/kit/src/core/sync/create_manifest_data/test/samples/`
(32 dirs) plus `test/params.js` and `test/static/`. Cases and expected outputs:
`basic` L58-L105 (node indices 0..5, endpoint `page_options: {}`), deterministic indices
under reversed readdir L107-L130, symlinks L138-L161, `basic-layout` L163-L187
(`layouts:[0,2], errors:[1,undefined]`), missing routes dir L189-L201, `encoding` L203-L229
(patterns `/^\/\]\/?$/`, `/^\/%3[Ff]\/?$/`, `/^\/"\/?$/`, `/^\/😀\/?$/`), sort golden
L231-L270, `rest` L272-L324, `rest-prefix-suffix` L326-L356 (`/^/prefix-([^]*?)/?$/`),
`optional` L358-L449, `nested-optionals` L451-L484, `optional-group` L486-L530,
`optional-adjacent` L532-L566, `group-optional` L568-L602, `hidden-underscore` L604-L610,
`hidden-dot` (only `.well-known`) L612-L618, `multiple-slugs` L620-L633, `invalid-params`
L635-L639, `lockfiles` L641-L658, `missing-prefix` log L660-L672, `custom-extension`
L674-L726, static assets `[{file:'bar/baz.txt',size:14,type:'text/plain'},{file:'foo.txt',size:9,...}]`
L728-L743, `nested-errors` L745-L777, `named-layouts` L779-L853 (16 nodes; `+page@.svelte`
→ `parent_id: ''`, `+page@(special).svelte` → `parent_id: '(special)'`),
`page-without-svelte-file` L855-L900, missing named layout L902-L907, `+page@.js` invalid
L909-L914, params path L916-L931, conflicting groups L933-L938, duplicate layout/page/ts+js
L940-L969, prerendered dual route L971-L976.

### `static_analysis/index.spec.js` (page options from `+page(.server).js`)

- Recognised exports: `ssr`, `prerender`, `csr`, `trailingSlash`, `config`, `entries`
  (`static_analysis/index.js:L7-L13`). Literal `const`/`let` values → object (spec L10-L29);
  any dynamic value, object, arrow function, `export * as ssr`, `export * from` → `null`
  (L31-L63, L74-L82); non-option exports ignored → `{}` (L65-L72); non-reassigned `let`
  in nested scopes still literal (L84-L141); `export { ssr }` from a local `let` literal
  ok, from imports/destructuring → `null` (L143-L195); `load` export special-cased as
  `{ load: null }` (L197-L212).

### Small helper specs

- `utils/http.spec.js:L4-L29`: `negotiate(accept, types)` handles OWS around `,`/`;`, `q`,
  ignores segments without `/`, no catastrophic backtracking on 200k `a`s (100 ms budget);
  `matches_content_type` ignores params and case.
- `utils/escape.spec.js:L4-L19`: `escape_html(str, true)` escapes `"`, `&`, and lone
  surrogates as `&#NNNNN;` — HTML only.
- `utils/error.spec.js:L5-L22`: `get_status` → `HttpError.status`, `SvelteKitError.status`,
  else 500 (also for non-Error values). L24-L41 deprecated accessors — JS-only.
- `exports/url.spec.js:L4-L56`: `is_external_location` true for scheme, `//`, `\\`,
  leading whitespace, `java\tscript:`, `x:foo`, `blob:`; false for `/foo`, `./foo`, `foo`,
  `#hash`, `?query`; `validate_redirect_location` throws
  `Cannot redirect to external URL "<loc>"` for those.
- `exports/hooks/sequence.spec.js:L23-L183`: handlers run outer→inner (`1a 2a 3a 3b 2b 1b`);
  `transformPageChunk` composes innermost-first (`0-3-2-1`); first defined `preload` and
  `filterSerializedResponseHeaders` win. Only the ordering (L23-L55) matters without SSR.

## Citations

- `reference/kit/packages/kit/src/utils/routing.js:L5-L136`, `L143-L246`, `L252-L338`, `L356-L370`
- `reference/kit/packages/kit/src/utils/routing.spec.js:L10-L120`, `L122-L380`, `L382-L535`, `L537-L660`
- `reference/kit/packages/kit/src/utils/regex.js:L6-L9`
- `reference/kit/packages/kit/src/utils/params.spec.js:L10-L102`
- `reference/kit/packages/kit/src/utils/url.spec.js:L11-L267`
- `reference/kit/packages/kit/src/core/sync/create_manifest_data/index.js:L17-L20`, `L121-L523`, `L533-L574`
- `reference/kit/packages/kit/src/core/sync/create_manifest_data/sort.js:L18-L161`
- `reference/kit/packages/kit/src/core/sync/create_manifest_data/conflict.js:L2-L72`
- `reference/kit/packages/kit/src/core/sync/create_manifest_data/index.spec.js:L14-L56` (harness), `L58-L976` (cases)
- `reference/kit/packages/kit/src/exports/vite/static_analysis/index.spec.js:L4-L212`; `index.js:L7-L13`
- `reference/kit/packages/kit/src/utils/http.spec.js:L4-L29`; `utils/escape.spec.js:L4-L19`; `utils/error.spec.js:L5-L41`
- `reference/kit/packages/kit/src/exports/url.spec.js:L4-L56`; `exports/hooks/sequence.spec.js:L23-L183`

## Go port notes

- `internal/routing`: `ParseRouteID(id) (pattern *regexp.Regexp, params []Param)`,
  `Exec(match []string, params, matchers) (map[string]any, bool)`, `ResolveRoute`,
  `FindRoute`. Go `regexp` (RE2) lacks lookahead but the *generated* patterns use none
  (`[^]` must be rewritten as `[\s\S]` or `(?s:.)`; `[^/]+?` lazy quantifiers are supported
  by RE2). Test by comparing the JS pattern strings after a documented rewrite, or better,
  by matching behaviour on the 44 `exec` rows.
- Matchers: `map[string]func(string) (any, bool)`; `matches`/`doesntmatch` fakes become
  Go closures. Skip the async/return-type error rows.
- `internal/manifest` (route scanner): reuse the upstream fixture directories verbatim —
  copy `test/samples/**`, `test/params.js`, `test/static/**` into `testdata/` (they are
  plain files; note `samples/symlinks` needs a real symlink and `lockfiles` contains
  lockfile-named files). Implement `simplify_node`/`simplify_route` equivalents and assert
  the same node lists, `page: {layouts, errors, leaf}` (with `undefined` → `-1` or
  `*int`), and pattern strings. Port the 12 error-message regexes exactly.
- `sort_routes`: port literally, then run the 28-id golden with a shuffled input.
- Static analysis: Go cannot run acorn. Options: (a) require the adapter/Vite plugin to
  emit page options into the manifest the Go server consumes (kit already computes them at
  build), (b) treat all values as dynamic (`null`) and rely on runtime module info. Mark as
  **JS-only** in `port-map.md`; do not port the parser.

## Gotchas

- `[^]` is a JS-only "any char including newline" class; the `exec` row `'/\n'` (L324-L333)
  exists to prove newlines match. RE2 needs `(?s)` or `[\s\S]`.
- Path decoding: `find_route` receives the *decoded* pathname but static segments are
  compiled against encoded `%2F`, `%3F`, `%23`, `%25` forms because kit's `decode_pathname`
  leaves those untouched (L248-L257). Go must implement the same partial decoder before
  matching.
- `String.prototype.normalize()` (NFC) is applied to static text (L263); Go needs
  `golang.org/x/text/unicode/norm`.
- `readdirSync().sort()` is UTF-16 code-unit order; Go `sort.Strings` is byte order — same
  for ASCII fixture names.
- Node index arrays use `undefined` holes; JSON manifests serialise them as `null`. Choose
  one Go representation and keep it consistent with the client manifest the adapter emits.
- `chained` differs between whole-segment `[[opt]]` (`true`) and inline `prefix[[opt]]`
  (`false`); `exec`'s buffering depends on it.
- Trailing-slash: patterns end in `/?$`, so both `/foo` and `/foo/` match; the actual
  redirect/normalisation is `normalize_path` driven by the `trailingSlash` page option.

## Recipes

- Compile once: `for each route: re := regexp.MustCompile(rewriteJS(pattern))`; keep the
  original JS source string in the manifest for cross-checking against kit's client manifest.
- Validate the scanner: `go test ./internal/manifest -run TestSamples` iterating the copied
  `samples/*` dirs with the expected tables transcribed from `index.spec.js`.
- Trailing slash decision: `normalize_path(path, opt)`; redirect via
  `relative_pathname(from, to)` when a `Location` must be relative.
