# mrmime 2.0.1 — the MIME table sirv uses (embed it in Go)

Pinned source: `reference/mrmime/` (package.json `"version": "2.0.1"`, commit
c95e4bf, 2025-02-17). sirv calls `mrmime.lookup(name)` for every
`Content-Type` (`reference/sirv/packages/sirv/index.mjs:L5`, `:L106`), and
sirv's tests assert content-type literally through the same table
(`reference/sirv/tests/helpers.mjs:L10`, `:L90`). Go's `mime.TypeByExtension`
has a different built-in table, appends `; charset=utf-8` to text types, and
reads OS files at init, so it cannot be used if skgo wants to match kit /
adapter-node byte-for-byte.

## Purpose

Record how the mrmime table is derived, its exact lookup rule, the entries
that matter for a SvelteKit deployment, where Go's `mime` package disagrees,
and the recommendation (embed the table, generate it from the pinned file).

## Key facts (SOURCE-DERIVED)

### Where the table lives

- `reference/mrmime/src/$index.ts:L1-L7` is a **template**: `const mimes =
  {};` plus `lookup`. The build (`reference/mrmime/bin/index.ts:L87-L96`)
  replaces `{}` with the generated JSON and writes `index.mjs`, `index.js`
  (not committed), `deno/mod.ts` (committed) and copies it to
  `src/index.ts`. **The committed, readable table is
  `reference/mrmime/deno/mod.ts:L1-L440` (438 entries).**
- Source dataset: `mime-db` pinned to commit `3145b8f`
  (`reference/mrmime/bin/package.json:L5`).

### Derivation rules — `reference/mrmime/bin/index.ts:L19-L85`

- Skip any MIME type matching `/[/](x-|vnd\.)/` (`:L53`, `:L60`) — all
  `*/x-*` and `*/vnd.*` types are dropped, **including their extensions**.
- For each remaining type, every extension in `DB[type].extensions` maps to
  it. On a conflict between two types for one extension, keep by source rank
  `iana: 4` > unranked/mime-db `3` > `apache: 2` > `nginx: 1`; on a tie keep
  the **shorter** type string (`:L21-L50`).
- Keys are emitted sorted (`:L83-L85`). Values never include parameters (no
  charset).

### Lookup rule — `reference/mrmime/deno/mod.ts:L442-L446` (same as `src/$index.ts:L3-L7`)

```
tmp = String(extn).trim().toLowerCase()
idx = tmp.lastIndexOf('.')
return mimes[idx === -1 ? tmp : tmp.slice(idx+1)]
```

Pinned by `reference/mrmime/test/index.ts:L21-L53`: `'TXT'`, `'.txt'`,
`'foobar.txt'`, `'foo/bar.txt'`, `'C:\\hello\\world.html'`, `'  txt  '` all
resolve; non-string `123` → `undefined` (because `'123'` is not a key, not
because of a type check). Edge cases implied by the code: `'file.'` → key
`''` → undefined; a path with no dot (`'foo/bar'`) is looked up whole → undefined;
`'archive.tar.gz'` → `gz`. The `mimes` object is mutable and exported
(`:L71-L78`).

### Entries a SvelteKit deployment hits (from `reference/mrmime/deno/mod.ts`)

| ext | mrmime | line |
|---|---|---|
| `js`, `mjs` | `text/javascript` | L150, L209 (asserted `reference/mrmime/test/index.ts:L92-L93`) |
| `cjs` | `application/node` | L45 |
| `json` | `application/json` | L151 |
| `map` | `application/json` | L190 |
| `wasm` | `application/wasm` | L388 |
| `svg`, `svgz` | `image/svg+xml` | L350-L351 |
| `webmanifest` | `application/manifest+json` | L392 |
| `woff` / `woff2` | `font/woff` / `font/woff2` | L398-L399 |
| `ttf` / `otf` | `font/ttf` / `font/otf` | L370, L262 |
| `css` | `text/css` | L52 |
| `html`, `htm` | `text/html` | L120-L121 (sirv adds `;charset=utf-8`) |
| `txt` | `text/plain` | L373 |
| `xml` | `text/xml` | L424 (asserted `test/index.ts:L88`) |
| `xhtml` | `application/xhtml+xml` | L420 |
| `md`, `markdown` | `text/markdown` | L196, L192 |
| `csv` | `text/csv` | L53 |
| `yaml`, `yml` | `text/yaml` | L435, L438 |
| `jsonld` | `application/ld+json` | L153 |
| `json5`, `toml` | `application/json5`, `application/toml` | L152, L363 |
| `jsx` | `text/jsx` | L155 |
| `ts` | **`video/mp2t`** (not TypeScript) | L366 |
| `png`, `jpg`/`jpeg`, `gif`, `webp`, `avif`, `apng`, `jxl`, `heic`/`heif`, `bmp`, `tif`/`tiff` | `image/*` | L277, L141-L143, L98, L393, L27, L14, L157, L110-L112, L31, L361-L362 |
| `mp4`, `webm`, `mov`, `ogv` | `video/mp4`, `video/webm`, `video/quicktime`, `video/ogg` | L217, L391, L212, L253 |
| `mp3`, `wav`, `weba`, `m4a`, `oga`/`ogg`/`opus` | `audio/mpeg`, `audio/wav`, `audio/webm`, `audio/mp4`, `audio/ogg` | L216, L389, L390, L182, L251-L252, L261 |
| `pdf`, `zip`, `gz` | `application/pdf`, `application/zip`, `application/gzip` | L270, L439, L106 |
| `rss`, `atom` | `application/rss+xml`, `application/atom+xml` | L302, L20 |
| `manifest`, `appcache` | `text/cache-manifest` | L189, L15 |
| `ics`, `vtt` | `text/calendar`, `text/vtt` | L122, L385 |
| `geojson` | `application/geo+json` | L97 |

**Absent** (because their only mime-db types are `x-`/`vnd.`): `ico`
(`image/vnd.microsoft.icon`, `image/x-icon`), `eot`
(`application/vnd.ms-fontobject`), `flac`, `mkv`, `m3u8`, `tsx`, `jsonc`,
`cur`, `sh`, `br`. sirv therefore serves `favicon.ico` with `Content-Type:
''` — browsers sniff it fine, but a Go port that consults Go's table would
emit `image/x-icon` and differ.

### Go `mime.TypeByExtension` on this machine (macOS, `/etc/apache2/mime.types` present) vs mrmime

Measured with a scratch program (Go stdlib, Darwin 25.6):

| ext | Go | mrmime | differs? |
|---|---|---|---|
| `.js`, `.mjs` | `text/javascript; charset=utf-8` | `text/javascript` | charset suffix |
| `.css` | `text/css; charset=utf-8` | `text/css` | charset |
| `.html`/`.htm` | `text/html; charset=utf-8` | `text/html` (sirv → `text/html;charset=utf-8`) | space after `;` |
| `.txt`, `.csv` | `…; charset=utf-8` | no charset | charset |
| `.json` | `application/json` | same | — |
| `.map` | `""` | `application/json` | missing in Go |
| `.wasm`, `.svg`, `.webp`, `.avif`, `.png`, `.jpg`, `.gif`, `.pdf`, `.mp4`, `.webm`, `.woff`, `.woff2`, `.ttf`, `.otf`, `.gz`, `.zip`, `.mp3`, `.xhtml`, `.rss`, `.atom`, `.ts` | same | same | — |
| `.webmanifest` | `""` | `application/manifest+json` | missing in Go |
| `.cjs`, `.md`, `.yaml`, `.jsx`, `.jsonld` | `""` | present | missing in Go |
| `.xml` | `application/xml` (from `/etc/apache2/mime.types`; Go builtin is `text/xml; charset=utf-8`) | `text/xml` | env-dependent |
| `.ico` | `image/x-icon` | absent (`""`) | Go has it |
| `.eot` | `application/vnd.ms-fontobject` | absent | Go has it |
| `.wav` | `audio/x-wav` | `audio/wav` | vendor form |
| `.manifest`, `.appcache` | `text/cache-manifest; charset=utf-8` | `text/cache-manifest` | charset |

Go's table is also loaded from `/etc/mime.types`, `/etc/apache2/mime.types`,
`/etc/apache/mime.types`, `/etc/httpd/conf/mime.types` and (Linux)
shared-mime-info `globs2` at init, so results vary by host — Debian's
`/etc/mime.types` gives `.js` → `application/javascript`. This is exactly the
non-determinism to avoid.

## Citations

- Template and lookup: `reference/mrmime/src/$index.ts:L1-L7`
- Generated table + lookup: `reference/mrmime/deno/mod.ts:L1-L448`
- Build/derivation: `reference/mrmime/bin/index.ts:L19-L96`; mime-db pin `reference/mrmime/bin/package.json:L5`
- Public types: `reference/mrmime/index.d.ts:L1-L2`
- Lookup behaviours and override assertions: `reference/mrmime/test/index.ts:L11-L95`
- Documentation: `reference/mrmime/readme.md:L28-L88`
- Consumer: `reference/sirv/packages/sirv/index.mjs:L5`, `:L103-L108`; `reference/sirv/tests/helpers.mjs:L82-L99`

## Go port design

Embed the table; do not consult `mime.TypeByExtension`.

- `go:generate` a `mimetable_gen.go` from `reference/mrmime/deno/mod.ts` (or
  a vendored copy of it): parse the object literal (lines matching
  `^\s+"([^"]+)": "([^"]+)",?$`), emit
  `var mimes = map[string]string{ ... }` (438 entries, sorted). Keep the
  source commit in a comment so refresh is mechanical.
- `func Lookup(name string) string` replicating `lookup`: `strings.TrimSpace`,
  `strings.ToLower`, take substring after the last `.`, or the whole string
  when there is no dot; return `""` when unknown (JS `undefined`).
  No charset, no parameters. Leave the `text/html` → `text/html;charset=utf-8`
  rule in the sirv-port layer (`sirv.md`), not here.
- Optionally expose `func Set(ext, typ string)` for parity with the mutable
  `mimes` export (adapter-node does not mutate it — verify).
- Test: golden compare of every entry against the pinned file; the seven
  literal assertions from `reference/mrmime/test/index.ts:L81-L94`
  (`xsl` → `application/xml`, `mp3` → `audio/mpeg`, `wav` → `audio/wav`,
  `x3db` → `model/x3d+fastinfoset`, `x3dv` → `model/x3d-vrml`, `rtf` →
  `text/rtf`, `xml` → `text/xml`, `3gpp` → `video/3gpp`, `jpm` →
  `image/jpm`, `js`/`mjs` → `text/javascript`, `mp4` → `video/mp4`); the
  lookup-format cases; and an explicit "absent" list (`ico`, `eot`, `tsx`)
  so nobody "fixes" them by falling back to Go's table without deciding to.

## Gotchas

- **Go's `net/http` sniffs**: if the handler leaves `Content-Type` unset, Go
  runs `DetectContentType` on the first 512 bytes and adds e.g.
  `text/plain; charset=utf-8` or `image/x-icon`. To reproduce sirv's empty
  content-type for unknown extensions, either set an empty header explicitly
  or delete the key (`w.Header()["Content-Type"] = nil`) before writing.
- **`; charset=utf-8` vs `;charset=utf-8`**: Go's `mime.FormatMediaType` and
  its built-in table use a space; sirv/kit tests use no space. Build the
  string by concatenation.
- **`.ts` is `video/mp2t`** in both tables — a TypeScript source served
  from `static/` would be labelled as video; kit never serves `.ts` at
  runtime so this is only a surprise in tests.
- **Case-insensitivity** applies to the extension only; the table keys are
  lowercase, so `Lookup("A.JSON")` works but a key lookup with `"JSON"`
  does not — always go through `Lookup`.
- **Windows separators**: `lastIndexOf('.')` ignores `\` and `/`, so any path
  works; but a directory containing a dot with an extension-less file
  (`v1.2/README`) yields ext `2/readme` → `""`. Same in Go if ported
  verbatim; matches sirv.
- Kit's own content-type expectations (e.g. tests asserting `text/javascript`
  for `_app/immutable/*.js`, `application/json` for `__data.json` written by
  kit itself) — verify in `reference/kit/packages/kit/test/` before assuming
  the sirv table is the only source of truth; kit sets `Content-Type` for
  dynamic responses itself and that is independent of mrmime.
