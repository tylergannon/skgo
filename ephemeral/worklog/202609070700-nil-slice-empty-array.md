# A nil slice on the wire (#45)

Branch `fix-nil-slice-empty-array`. Traps and facts only.

## polytype already answered it, so this was never a design question

`devalue/codegen/encode.go` in polytype v1.0.0-rc.10 (the latest release; the
project is already on it) emits, for a `Slice`, with a comment saying so:

    // A nil required slice encodes as an empty array, never as null.
    out := make([]any, 0, len(src))

So the rule is polytype's own, and skgo was simply not using polytype's encoder
on this path. Only a *transported* type goes through the generated devalue
codec; every other result reaches the wire through skgo's `encodeValue`, which
is a `json.Marshal`/`json.Unmarshal` round trip, and encoding/json writes `null`
for a nil slice. The fix belongs in skgo, and there is nothing to change or
upgrade in polytype.

## The grammar has no map, and refuses a byte slice — so slices are the whole set

`typegrammar/grammar.go` has `Scalar Time Enum Object Pointer Slice Array Ref
Union UnionSlice` and **no Map constructor at all**, so a Go map never appears
in a generated declaration and there is no "nil map crosses as null" bug to
chase. `typegrammar/validate.go` rejects a byte-like slice outright — "byte-like
slices have a base64 wire mapping outside this grammar" — which is why the
rewrite skips a `[]byte`: encoding/json makes it a base64 *string*, and turning
its null into `[]` would invent an array no declaration promised.

## There are two encoders, and both had to be corrected

`encodeValue` (the round trip) and `Transport.walk` (the reflect walk that runs
only when the app declares a transport). Correcting one and not the other would
have made a result gain and lose its nulls depending on whether the app happens
to declare a `transport` hook. `transport_encode_test.go` compares the two
against each other for a set of fixtures, which is what would have caught it.

## `emptyArrays` stops at anything that marshals itself

A `json.Marshaler` or `encoding.TextMarshaler` writes bytes that need not
resemble its fields, so the rewrite must not descend into one — `time.Time`
being the obvious case. It is deliberately conservative about a *pointer*-
receiver marshaler on a value type: encoding/json only uses it when the value is
addressable (it never is, below `json.Marshal(any)`), so skipping such a subtree
can miss a nil slice inside it. That direction is safe; the other would corrupt
the value.

## Traps hit, worth not repeating

**The Bash tool's cwd persists between calls.** A `cd example/e2e` in one
command left the next `cp /tmp/ea.keep emptyarray.go` writing
`example/e2e/emptyarray.go` while the real file stayed broken. The suite then
failed *with the fix supposedly restored*, which reads exactly like the fix not
working. Restore with absolute paths, and check `git diff HEAD -- <file>` before
believing a restore happened.

**The suite is not idempotent against a long-lived server.** `routes.feature`'s
POST scenario asserts one `api-todo` matching its text; the store is in-memory
and survives between runs, so a second suite run against the same process fails
with "Expected 1, Received 2". Restart the server between runs.

**A page whose load reads `.length` on a null 500s rather than showing a
boundary.** With the bug reintroduced, `/empty` did not render a failure panel —
the server-side render threw and Go answered the error page. Either is a fine
failure signal, but a scenario that only looked for `*-failed` would have been
confused by it.

## Screenshot evidence has to be regenerated after a failing run

`shot()` writes `<slug>-final.png` only when a scenario took no shot of its own,
which is what happens when a step fails before reaching one. Those `-final`
files are left behind in `ephemeral/screenshots/` (tracked) after a deliberate
break-it run and will sit there looking like passing evidence of an error page.
Delete the feature's directory and rerun before committing.

## Validation round: four blockers, and a fifth the validator did not see

**`TestTransportWalkAgreesWithEncodingJSON` was comparing encoding/json with
itself.** `forceWalk` claimed `struct{ never int }`, a type no fixture holds, so
`reaches()` said no for every fixture and `walk` returned `encodeValue(...)`
before the walk began. Renaming every property `structFields` writes
(`obj["BROKEN"+name]`) left it green. It now claims `string` — the one type
every fixture holds somewhere — which is what puts the whole value on the walk;
a transported string comes back out as the same Go string encoding/json writes,
so the comparison stays about structure. Check a "the two encoders agree" test
by breaking the encoder, not by reading it.

**encoding/json's shadowing rule is not "the first promoted field wins".** Both
encoders promoted an embedded struct's fields by walking the struct in
declaration order, so a collision was resolved by whoever wrote last. Real rule:
shallower wins, tagged beats untagged at the same depth, and a tie on both is
dropped from the object entirely. With `embB{embA; X *string}` where `embA` has
`X []string`, encoding/json writes `{"x":null}` and the rewrite turned it into
`{"x":[]}` — the same lie this branch exists to stop, pointing the other way.
`jsonfields.go` is a port of `typeFields` from `$GOROOT/src/encoding/json/
encode.go`; both encoders now ask it which Go value produced a property. It
carries `omitzero` too, which the walk had never handled.

**`go vet` refuses two embedded types with the same json tag.** The ambiguity
fixture had to be built out of untagged fields (`Z []string` beside `Z *string`)
to say the same thing; vet's structtag check only sees tags.

**An argument is not a result.** `queryPayload` ran the refresh argument through
`encodeValue`, so with the empty-array rewrite in place `Refresh(ctx, getTags,
nil)` keyed as `[]` (`W1tdXQ`) where the client had keyed `null` (`W251bGxd`).
The client parks an unknown key as a pre-seed rather than erroring, so the open
page just never updates and nothing anywhere reports it. Arguments take
`roundTripValue`, the raw round trip. The rule: the rewrite is a claim about the
declaration Go generated for a *return* type; the client owns the bytes of an
argument.

**Regenerating a payload golden.** The recipe in
`202609060830-codec-conformance.md` still works verbatim; `devalue.stringify([])`
is `"[[]]"` → `W1tdXQ`, `devalue.stringify(null)` is `"[null]"` → `W251bGxd`.

## Both suites, on ports of their own

Something was already on 5173, so: vite on 5187, Go on 8087, `ORIGIN` matching
at both build time and run time, killed by pid afterwards. dev 56 passed / 0
failed / 0 skipped; prod 64 passed / 0 failed / 0 skipped, on a server restarted
between them (the store is in-memory and the suite is not idempotent). Every
frame the two runs rewrote is byte-identical to the tracked one.

## Rebased onto main after #55 and #47

Every conflict was a screenshot — 29 binary frames both branches had re-taken
(this branch added the `Empty` nav link, #55 changed the account/orders page).
Taking either side and regenerating is the only resolution that means anything:
`git checkout --theirs` on all 29, then both suites on the rebased tree, which
rewrote 24 prod frames and left the 29 dev/transport frames byte-identical.

**No source file conflicted.** The branch owns the encoders (`emptyarray.go`,
`jsonfields.go`, `transport_encode.go`, `encodeValue`/`roundTripValue`,
`queryPayload`) and main's additions to `remote.go` were the adapter fingerprint
check, several hundred lines away.

**The streaming chunk path already gets the correction, in both halves.**
`data.go`'s `chunkLine` and `document.go`'s `unevalChunk` each encode a settled
`Deferred` with `transport.encodeTree(value)` — the same entry point a remote
result uses — so a resolved chunk whose value is a nil slice carries `[]`, not
`null`, with or without a transport hook. Nothing had to be added for #55, and
the reason is that #55 routed the chunk through the one encoder rather than
marshalling it itself.

dev 56 passed / 0 failed / 0 skipped; prod 64 passed / 0 failed / 0 skipped,
vite on 5191 and Go on 8093, `ORIGIN` matching at build and run time, server
restarted between the two runs. No `-final.png` was left behind.
