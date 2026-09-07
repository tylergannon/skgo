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
