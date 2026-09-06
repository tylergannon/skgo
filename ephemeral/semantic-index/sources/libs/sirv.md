# sirv 3.0.2 — static-file algorithm that Go must replicate

Pinned source: `reference/sirv/packages/sirv/index.mjs` (package.json
`"version": "3.0.2"`, commit 1135207, 2025-09-03). adapter-node mounts three
sirv instances (client `_app/immutable`, prerendered pages, `static/`); skgo
serves those directories natively from Go, so its handler must reproduce
sirv's lookup order and header set closely enough that kit's own tests and
browser caching behave identically. The whole implementation is 197 lines;
this leaf is a step-by-step reading of it plus what the test suite pins.

## Purpose

Specify, with citations, exactly how sirv resolves a request path to a file
and which headers it sends, so a Go `net/http` handler can be written and
tested against the same table of behaviours without reading Node code again.

## Key facts (SOURCE-DERIVED)

### Construction — `reference/sirv/packages/sirv/index.mjs:L121-L167`

- `dir = path.resolve(dir || '.')` (`:L122`). Missing dir throws ENOENT at
  construction in prod (totalist walk) — `reference/sirv/tests/sirv.mjs:L25-L27`.
- `isNotFound = opts.onNoMatch || is404`; default 404 is `res.statusCode=404;
  res.end()` — **empty body, no Content-Type** (`:L56-L58`, `:L124`).
- `extensions = opts.extensions || ['html','htm']` (`:L127`).
- `gzips = opts.gzip && extensions.map(x => x+'.gz').concat('gz')` →
  `['html.gz','htm.gz','gz']`; `brots` likewise with `.br` (`:L128-L129`).
- `single`: `fallback = '/'`; if `single` is a string, append it **minus its
  last extension** (`'about/index.htm'` → `'/about/index'`), so the fallback
  is itself resolved through the extension list (`:L133-L139`).
- `ignores` (only consulted for the SPA fallback): unless `opts.ignores ===
  false`, always push `/[/]([A-Za-z\s\d~$._-]+\.\w+){1,}$/` ("last segment
  looks like `name.ext`"); then `dotfiles ? /\/\.\w/ : /\/\.well-known/`;
  then each user entry as `new RegExp(x, 'i')` (`:L141-L149`).
- `Cache-Control` value `cc`: `opts.maxAge != null` → `public,max-age=<n>`;
  `+ ',immutable'` if `immutable`; else `+ ',must-revalidate'` if `maxAge
  === 0` (`:L151-L153`). `immutable` without `maxAge` → **no header**
  (`reference/sirv/tests/sirv.mjs:L955-L964`). `maxAge: null` → no header
  (`:L916-L925`).
- **Prod cache** (`!opts.dev`): `totalist` walks `dir` recursively; entries
  are skipped when `!dotfiles && /(^\.|[\\+|\/+]\.)/.test(name)` unless the
  relative name matches `/\.well-known[\\+\/]/` (kept always). Each file's
  headers are computed **once** (size, mtime, etag) and stored under
  `'/' + name.normalize().replace(/\\+/g,'/')` — note **Unicode NFC
  normalisation** of the key (`:L155-L165`). Files added after start are
  invisible; mtime/size changes are stale.
- `lookup = dev ? viaLocal(dir + sep, isEtag) : viaCache(FILES)` (`:L167`).

### Candidate list — `toAssume(uri, extns)` `:L15-L29`

Strip one trailing `/`. For each `x` in `extns` (`''` → no suffix, else
`'.' + x`): push `uri + x` **if uri non-empty**, then always push
`uri + '/index' + x`. So with `extns = ['html.br','htm.br','br','html.gz',
'htm.gz','gz','','html','htm']` (brotli+gzip both accepted):

```
/about →  /about.html.br /about/index.html.br /about.htm.br /about/index.htm.br
          /about.br /about/index.br /about.html.gz /about/index.html.gz
          /about.htm.gz /about/index.htm.gz /about.gz /about/index.gz
          /about /about/index /about.html /about/index.html /about.htm /about/index.htm
/      →  /index.html.br /index.htm.br /index.br /index.html.gz /index.htm.gz /index.gz
          /index /index.html /index.htm
```

First hit wins. Consequences pinned by tests: `/about` → `about/index.htm`
(`reference/sirv/tests/sirv.mjs:L209-L218`), `/blog` → `blog.html`
(`:L231-L240`), `extensions:['html']` makes `/about` 404 (`:L248-L260`),
`extensions:['js','css']` lets `/bundle.67329` serve `bundle.67329.js`
(`:L262-L276`). A request for `/about/` (trailing slash) is identical to
`/about`. There is **no redirect** for trailing slashes in either direction.

### Per-request algorithm — `:L169-L196`

1. `pathname = parse(req).pathname` via `@polka/url` (query string removed;
   no dot-segment normalisation). Method is **never checked**: POST/PUT to a
   file path serves the file; HEAD is not special-cased (Node drops the body).
2. Encoding negotiation (`:L172-L175`): `val = req.headers['accept-encoding']
   || ''`; `if (gzips && val.includes('gzip'))` prepend gzips; `if (brots &&
   /(br|brotli)/i.test(val))` prepend brots; then append `extensions`. Order
   therefore `[...brots, ...gzips, '', ...extensions]` — **brotli beats gzip
   when both accepted** (`reference/sirv/tests/sirv.mjs:L802-L821`,
   `:L867-L886`). No q-value parsing: `gzip;q=0` still counts as accepted;
   `includes('gzip')` is case-sensitive, the brotli test is not.
3. If `pathname` contains `%`, `decodeURI` (not `decodeURIComponent`): `%20`
   and `%C3%BC` decode, `%2F` stays literal so `/about%2Findex.htm` is 404
   (`:L153-L179`); malformed sequences are swallowed and the raw path used
   (`:L177-L180`).
4. `data = lookup(pathname, extns) || (isSPA && !isMatch(pathname, ignores)
   && lookup(fallback, extns))` (`:L182`). SPA fallback is skipped for
   asset-like paths (`/404.css`, `/foo/bar/baz.bat` → 404,
   `:L407-L425`), served for `/.hello` when `dotfiles` is off (the "any
   extension" regex needs a char before the dot; `:L592-L604`), and
   `ignores:false` falls back everything (`:L433-L448`).
5. Miss → `next ? next() : isNotFound(req, res)` (`:L183`). adapter-node
   passes `next`, so a miss falls through to the SSR handler — verify in
   adapter-node.
6. ETag short-circuit (`:L185-L188`): `if (isEtag && req.headers['if-none-match']
   === data.headers['ETag'])` → `res.writeHead(304); res.end()`. Strict
   string equality — no list, no `*`, no weak-comparison; the 304 carries
   **none** of the file headers. Applies before Range handling.
7. `if (gzips || brots) res.setHeader('Vary', 'Accept-Encoding')` — emitted
   whenever compression lookup is *enabled*, even when the plain file is
   sent (`:L190-L192`).
8. `setHeaders(res, pathname, data.stats)` user hook (`:L194`).
9. `send(req, res, abs, stats, headers)` (`:L195`).

### Dev lookup — `viaLocal` `:L38-L54`

`abs = normalize(join(dir, name))`; require `abs.startsWith(dir)` (dir has
trailing `sep`, so `/../publicfile.txt` cannot escape by prefix,
`reference/sirv/tests/sirv.mjs:L308-L331`) and `fs.existsSync`; directories
are skipped (`continue`, so `/about` proceeds to `/about/index.htm`).
Headers computed per request with `Cache-Control: no-cache` if etag else
`no-store` (`:L50`; tests `:L685-L694`, `:L742-L756`). `maxAge`/`immutable`
are ignored in dev. **Dev mode does not filter dotfiles**: `viaLocal` has no
dotfile check, and the test "should reject hidden files by default (dev:
true)" (`:L572-L590`) only asserts inside `.catch`, so it passes vacuously
while `/.hello` is actually served. Do not copy that gap into Go.

### Headers — `toHeaders` `:L98-L119`

- `enc = ENCODING[name.slice(-3)]` → `'.br' → 'br'`, `'.gz' → 'gzip'`.
  Content-Type is looked up on the name **with the 3-char suffix stripped**
  when `enc` is set; e.g. `data.js.br` → `text/javascript` + `Content-Encoding:
  br`. Any file ending `.gz`/`.br` gets `Content-Encoding` even when
  requested directly (a `.tar.gz` download would be transparently inflated by
  browsers — trap).
- `Content-Type`: `mrmime.lookup(...) || ''`; exactly `text/html` becomes
  `text/html;charset=utf-8` (**no space** after `;`, `:L107`; tests compare
  literally, `reference/sirv/tests/helpers.mjs:L90-L91`). No charset for
  `text/plain`, `text/css`, `text/javascript` (`reference/sirv/tests/sirv.mjs:L80`,
  `:L652`). Unknown extension (e.g. `.ico`, which mrmime lacks) → empty
  string header value.
- `Content-Length: stats.size` (compressed size for `.br`/`.gz`).
- `Last-Modified: stats.mtime.toUTCString()` (RFC 1123, e.g.
  `Wed, 03 Sep 2025 22:04:22 GMT`). `If-Modified-Since` is **never** checked.
- `ETag: W/"<size>-<mtimeMs>"` only when `etag: true`; `mtimeMs` is
  `Date#getTime()` — integer milliseconds (`:L116`; test `:L714-L724`).
- `Cache-Control: <cc>` added at cache-build time (prod only), `:L161`.
- In `send` (`:L60-L71`): any header already set on `res` (e.g. by
  `setHeaders`) **wins** over the computed one, including `Content-Type`,
  `Cache-Control`, `Last-Modified` (`reference/sirv/tests/sirv.mjs:L1169-L1218`).

### Range — `send` `:L73-L92`

Only when `req.headers.range` exists: `code = 206`; parse by
`range.replace('bytes=','').split('-')`; `end = parseInt(y) || size-1`;
`start = parseInt(x) || 0`. Then `end >= size → end = size-1`; `start >= size
→ 416` with `Content-Range: bytes */<size>` and empty body (`:L83-L87`).
Otherwise `Content-Range: bytes <start>-<end>/<size>`, `Content-Length:
end-start+1`, `Accept-Ranges: bytes`, body via `createReadStream(file,
{start,end})`. Pinned: `bytes=0-10` → 11 bytes (`:L972-L987`), `bytes=2`
(no dash) → `2-<last>` (`:L1040-L1055`), overflow end shrinks
(`:L1089-L1101`), overflow start → 416 (`:L1074-L1087`). Quirks a Go port
must decide on: suffix ranges `bytes=-500` are mis-parsed as `0-500`;
`start > end` is not rejected (negative Content-Length); multiple ranges are
not supported; `If-Range` is ignored; **`Accept-Ranges` is only sent on 206
responses**, never on plain 200 (`:L1103-L1145`).

### Not tested but true

`Vary` is set even when no compressed variant exists; there is no directory
listing; symlinks are followed by `totalist`/`fs.stat`; `HEAD` returns full
headers with `Content-Length` of the would-be body (Node behaviour);
`onNoMatch` may return any status (`:L1260-L1278`).

## Citations

- Construction and cache walk: `reference/sirv/packages/sirv/index.mjs:L121-L167`
- Candidate generation: `:L15-L29`; cache/local lookup: `:L31-L54`
- Request handler: `:L169-L196`; 404: `:L56-L58`
- `send`/Range: `:L60-L96`; headers: `:L98-L119`
- Options surface: `reference/sirv/packages/sirv/index.d.ts:L15-L28`
- Documented lookup order and option semantics: `reference/sirv/packages/sirv/readme.md:L59-L234`
- Tests by suite: basics `reference/sirv/tests/sirv.mjs:L33-L69`; URI encoding `:L73-L181`; index `:L185-L242`; extensions `:L246-L278`; security `:L282-L333`; single `:L337-L427`; ignores `:L431-L546`; dotfiles `:L550-L634`; dev `:L638-L696`; etag `:L700-L758`; brotli `:L762-L823`; gzip `:L827-L888`; maxAge `:L892-L927`; immutable `:L931-L966`; ranges `:L970-L1147`; setHeaders `:L1151-L1256`; onNoMatch `:L1260-L1280`
- Test helper `matches` (asserts content-length, content-type, status, body): `reference/sirv/tests/helpers.mjs:L82-L107`

### Options adapter-node passes (verify in adapter-node)

From memory of `@sveltejs/adapter-node` `src/handler.js` — **verify in
`reference/kit/packages/adapter-node/src/handler.js`**:

- client assets: `sirv(path.join(dir,'client'), { etag: true, gzip: true,
  brotli: true, setHeaders: (res, pathname) => { if
  (pathname.startsWith(`/${manifest.appPath}/immutable/`) && res.statusCode
  === 200) res.setHeader('cache-control', 'public,max-age=31536000,immutable') } })`
  — i.e. immutable cache-control is set per-path via `setHeaders`, not via
  `maxAge`/`immutable` options.
- prerendered pages: `sirv(path.join(dir,'prerendered'), { etag: true,
  maxAge: 0, gzip: true, brotli: true })`, wrapped so only paths present in
  `manifest._.prerendered` (with kit's trailing-slash rewriting) reach sirv.
- `static/`: served by the same client sirv (kit copies `static/` into the
  client output) — verify.
- Precompression only exists when `adapter({ precompress: true })`, which
  writes `.br`/`.gz` siblings for files over a size threshold and a
  whitelist of extensions — verify in `adapter-node/index.js`.

## Go port design

`package static` with a single `Handler` built from a directory walk at
startup (prod) — mirror sirv's "stat once" strategy; kit's output is
immutable per deploy so there is no need for a dev/`viaLocal` path in skgo
(Vite serves dev).

```go
type Options struct {
    Extensions []string   // default {"html","htm"}
    Gzip, Brotli bool
    ETag bool
    MaxAge *int          // nil = no Cache-Control; 0 = ",must-revalidate"
    Immutable bool
    Single string        // "" off; "true" semantics via SingleIndex bool
    Ignores []*regexp.Regexp; IgnoresOff bool
    Dotfiles bool
    SetHeaders func(w http.ResponseWriter, pathname string, fi fs.FileInfo)
}
type entry struct{ abs string; size int64; mtime time.Time; hdr http.Header }
type Handler struct{ files map[string]entry; ... }
func New(dir string, o Options) (*Handler, error)
func (h *Handler) ServeHTTP(w, r)            // 404 with empty body on miss
func (h *Handler) Lookup(r *http.Request) (entry, bool)  // for fall-through to SSR
```

Steps in `ServeHTTP` (1:1 with `:L169-L196`):

1. `pathname := r.URL.Path`? **No** — Go's `net/http` already
   percent-decodes `URL.Path` (and keeps `RawPath` when they differ), which
   turns `%2F` into `/`. Use `r.URL.EscapedPath()` (or `RawPath` when set),
   then apply a `decodeURI`-equivalent: decode every `%XX` **except** the
   reserved set `;/?:@&=+$,#` (that is what `decodeURI` leaves encoded).
   Swallow errors and keep the raw path.
2. Build `extns` from `Accept-Encoding` with sirv's substring tests (not
   `q=` parsing) — or deliberately do proper q parsing and record the
   deviation; kit tests only send `Accept-Encoding: br,gzip`-style values.
3. Candidates via a port of `toAssume`; look up in `files` (keys stored NFC-
   normalised with `golang.org/x/text/unicode/norm` — on macOS APFS/HFS the
   walked names may be NFD; kit CI runs on Linux where they are whatever the
   build wrote, usually NFC).
4. SPA fallback with `ignores` regexes translated to RE2 (they are RE2-safe:
   `[/]([A-Za-z\s\d~$._-]+\.\w+){1,}$`, `/\.\w`, `/\.well-known`).
5. Miss → 404 empty body, or return `false` from `Lookup` so the mux falls
   through to SSR (adapter-node's `next`).
6. `If-None-Match` exact-match → `w.WriteHeader(304)` with **no** headers.
7. `Vary: Accept-Encoding` if gzip||brotli configured.
8. `SetHeaders` hook; then copy computed headers **only where not already
   set** on `w.Header()`.
9. Range: implement sirv's parser verbatim for parity, then decide whether
   to fix suffix ranges (`bytes=-N`) — kit never relies on Range for its own
   assets; a strictly-better RFC 7233 implementation (`http.ServeContent`)
   changes `Accept-Ranges` presence on 200s and the 416 body. Recommendation:
   hand-roll to match sirv exactly, list deviations in a test.
10. Write headers, `WriteHeader(code)`, `io.CopyN(w, f, n)` from `start`.
    For HEAD skip the body but keep `Content-Length` (Go's server does this
    automatically if you set the header and write nothing).

ETag: `fmt.Sprintf("W/\"%d-%d\"", size, mtime.UnixMilli())`.
Last-Modified: `mtime.UTC().Format(http.TimeFormat)` (== JS `toUTCString`).
Content-Type: from the embedded mrmime table (see `mrmime.md`); append
`;charset=utf-8` only for `text/html`; empty string → omit the header
(deviation from sirv's empty value; Node sends `Content-Type: ` — verify, and
note that kit tests never assert on an empty content-type).

### Test list

Port each sirv suite as Go table tests against a copy of
`reference/sirv/tests/public/` (includes `.hello`, `.hello.txt`,
`.well-known/example`, `foo/.world`, `fünke.txt`, `with space.txt`,
`data.js.br`/`.gz` without a plain `data.js`, `index.html.br`/`.gz`):

- basics: direct hits, 404 empty body for `/bundle.js`, `/deeper/bundle.js`.
- encoding: `/fünke.txt` raw and `%C3%BC`; `/with%20space.txt`;
  `/about%2Findex.htm` → 404.
- index: `/` → index.html; `/about` → about/index.htm; `/contact` →
  contact/index.html; `/blog` → blog.html; `/about/` same as `/about`.
- extensions: `['html']` → `/about` 404; `['js','css']` → `/bundle.67329`.
- security: `/../../package.json` 404; `/../publicfile.txt` HEAD 404.
- single: fallback for `/foobar`, `/foo/bar`, `/about/foobar`; custom
  string fallback `about/index.htm`; no fallback for `/404.css`,
  `/foo/bar/baz.bat`; `ignores:false`; regex/string ignores single and
  multiple; `/.hello` falls back when dotfiles off.
- dotfiles: `/.hello`, `/foo/.world`, `/.hello.txt` → 404 by default, 200
  with `Dotfiles`; `/.well-known/example` always 200.
- etag: header format; `If-None-Match` → 304 empty.
- brotli/gzip: `Content-Encoding`, `Content-Type` of underlying file,
  `text/html;charset=utf-8`, brotli preferred, plain `/bundle.67329.js`
  served unchanged with `Vary`.
- cache-control: `public,max-age=100`; `public,max-age=0,must-revalidate`;
  `public,max-age=1234,immutable`; `public,max-age=0,immutable`; none when
  `Immutable` without `MaxAge`.
- ranges: `0-10`, `6-96`, `80-115`, `0-115`, `2`, `0`, overflow start →
  416 `bytes */N`, overflow end shrinks, no `Accept-Ranges`/`Content-Range`
  on subsequent non-range request.
- setHeaders: custom `x-foo`, override `Content-Type`, `Cache-Control`,
  `Last-Modified`; receives pathname and file info.
- onNoMatch / fall-through: miss returns control to caller.

## Gotchas (Node/JS semantics Go lacks or does differently)

- **`net/http` path decoding**: `URL.Path` is fully decoded (`%2F` → `/`),
  and `http.ServeMux` cleans paths and 301-redirects `..`/`//` — do **not**
  mount the static handler under a `ServeMux` pattern that triggers
  cleaning; use a custom root handler and read `EscapedPath()`.
- **`decodeURI` vs `url.PathUnescape`**: Go decodes everything; sirv leaves
  `%2F %3F %23 %25`… encoded. Write the selective decoder.
- **JS `String#normalize()`** (NFC) on cache keys has no stdlib equivalent —
  `x/text/unicode/norm`.
- **Header case**: Node lowercases incoming header names; Go canonicalises
  (`If-None-Match`). `w.Header().Get` is canonical-insensitive — fine.
- **Empty header values**: Node emits `Content-Type: ` for an empty string;
  Go's `Header.Set("Content-Type","")` also emits an empty value but
  `http.ResponseWriter` will **sniff** and add `Content-Type` if the header
  is absent — set it explicitly (or `w.Header()["Content-Type"] = nil` to
  suppress sniffing) to avoid Go's `DetectContentType` diverging from sirv.
- **Content-Length on HEAD**: Go strips the body for HEAD but keeps an
  explicit `Content-Length`; do not `io.Copy` on HEAD (Go returns
  `http.ErrBodyNotAllowed`).
- **304 headers**: Go's `WriteHeader(304)` sends whatever is in `w.Header()`;
  sirv sends none of the file headers. Clear `Content-Type`/`Content-Length`
  before writing 304 if parity matters (browsers are fine either way).
- **`parseInt` semantics** in the Range parser: `parseInt('10abc')` = 10,
  `parseInt('')` = NaN → `|| 0`. Use a lenient leading-digits parser to match.
- **Regex flags**: sirv compiles user `ignores` strings with `'i'`; Go: prefix
  `(?i)`.
- **Directory index without extension**: candidate `'/about/index'` (empty
  extension) is tried — a file literally named `index` is served with
  `Content-Type: ''`.
- `stats.size` for `.br`/`.gz` — Content-Length is the **compressed** size;
  Go must not set `Content-Length` from the original.

## Recipes

- **Immutable assets**: replicate adapter-node by keying on path prefix
  `/_app/immutable/` in the `SetHeaders` hook →
  `cache-control: public,max-age=31536000,immutable` (verify exact string in
  adapter-node; sirv's own `maxAge+immutable` would produce
  `public,max-age=31536000,immutable`, identical bytes).
- **Prerendered pages**: build the `files` map only from paths listed in
  the kit manifest's prerendered set, then run sirv's candidate list with
  `maxAge: 0` → `Cache-Control: public,max-age=0,must-revalidate` and
  weak ETags so revalidation is cheap.
- **Fall-through**: implement `Lookup` separately from `ServeHTTP` so the
  Go mux can try static → prerendered → SSR sidecar, exactly like
  adapter-node's `next()` chain.
- **Precompressed variants**: only look for `.br`/`.gz` siblings if the build
  produced them; keep `Vary: Accept-Encoding` on every response of that
  handler when enabled.
- **Golden headers test**: for each fixture request assert the full header
  map (minus `Date`/`Connection`) against a recorded sirv run — cheapest way
  to catch drift from Go's default header injection (`Content-Type`
  sniffing, `Accept-Ranges` from `http.ServeContent`, etc.).
