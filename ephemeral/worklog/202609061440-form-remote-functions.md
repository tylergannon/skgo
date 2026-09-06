# Form remote functions

## A form does not post multipart/form-data

Issue #6 and the task both say a form carries `multipart/form-data`. It does
not, on the path skgo actually serves. Kit's enhanced client — the only one a
CSR app has, since the unenhanced fallback posts to the *page* URL with
`?/remote=<id>` and needs a server-side render to answer — calls
`convert_formdata` on the FormData first and then posts kit's own binary
envelope as `application/x-sveltekit-formdata`
(`runtime/client/remote-functions/form.svelte.js`, the `serialize_binary_form`
call).

`enctype="multipart/form-data"` on the `<form>` is still required, and is
probably where the belief comes from: kit warns in DEV if a form with a file
input lacks it, because it governs the *unenhanced* submission. It says nothing
about what the enhanced one sends.

What Go actually receives, per `runtime/form-utils.js`:

    1 byte  version (0)
    4 bytes header length, little-endian u32
    2 bytes file offset table length, little-endian u16
    N bytes devalue.stringify([data, meta]), File -> [name, type, size, lastModified, index]
    M bytes JSON array of file offsets, in *header* order
            file bodies, concatenated, sorted *smallest first*

The two orders differ, and that is the trap: walking the file bodies in header
order hands each file the other one's bytes. Nothing in the format announces
this — a single-file form works perfectly either way, which is why the e2e
scenario cannot catch it and `TestParseTwoFilesKeepsBytesWithTheirOwnField`
must.

`data` arrives already coerced and already nested: kit's client applies the
`n:`/`b:` prefixes and the dotted/`[]` path grammar before serialising, so Go
receives a POJO, not form fields.

## Kit's server never sends the input back to an enhanced form

`handle_issues` in `app/server/remote/form.js` populates `output.input` only
when `form_data` is non-null, and `deserialize_binary_form` returns null for the
binary path. The comment says why: "if it was a progressively-enhanced
submission, we don't need to return the input — it's already there." A rejected
submission never navigates and kit does not reset an enhanced form, so the
values are still in the controls. Sending an `input` back would be extra wire
traffic that changes nothing.

## `r` is inert in skgo, and will stay inert until there is a server-driven refresh API

Only `form.svelte.js` reads `r`, and only through
`should_refresh = refreshes === null && !response.r`. So `r` matters exactly
when the *client* requested no refreshes but the *server* performed some anyway
— which in kit means the handler called `getPosts().refresh()` itself.

skgo has no such API: `resolveRefreshes` runs only the keys the client sent. So
`r` is set precisely when `refreshes !== null`, where the client has already
decided not to `refreshAll`. It is emitted for wire fidelity and is not
currently load-bearing.

This was found by breaking it: deleting `data["r"] = true` left the
single-flight scenario green. The scenario is still load-bearing for
single-flight — deleting the *refresh resolution* turns it red — but it does not
test `r`, and no scenario can until a handler can refresh a query on its own
initiative.

## polytype cannot describe a File, and `skgo.File` is an alias

Two separate snags, both in the generator.

polytype projects JSON schemas, and a File is not a JSON value. Asked to
declare a struct containing one it fails with `mapNamedType: type byte not
found` on the bytes. There is no user-facing override — `time.Time` is
special-cased inside polytype itself. So a form argument that carries a file is
emitted as an inline TypeScript object literal instead of a declared type;
nested structs without files still go through polytype as before.

Separately: `skgo.File` is a type *alias* for the internal decoder's type, and
since Go 1.23 an alias is its own node in go/types (`*types.Alias`), not the
type it names. A `switch t.(type)` with a `*types.Named` case silently misses
it — the symptom was the File check reporting false for a struct that plainly
contained one. `types.Unalias` at the top of both walks is the fix. Any future
exported alias in this package has the same problem.

## The e2e suite outlives its own data

The example's stores are process-global and the server is long-lived, so
running the suite twice against one process leaves two copies of every message.
`toHaveCount(1)` on a submitted message passes once and then fails forever.
Scenarios must assert what they put there is present, keyed on their own
content, and never assert how many rows the app holds — unless the claim really
is about deduplication.

Related: an empty `<ul>` is zero-height, so `toBeVisible()` on a list that
starts empty fails. `toBeAttached()` is the check for "the list rendered".
