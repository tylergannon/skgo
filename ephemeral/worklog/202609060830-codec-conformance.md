# Codec conformance (issues #2, #3)

discovery: kit's `src/runtime/shared.spec.js` pins **no bytes**. Every
`stringify_remote_arg` case asserts `a === b` for two orderings of the same
value, or round-trips through `parse_remote_arg`. There are no expected strings
to copy verbatim; the equivalence cases were already ported in
`remotearg_test.go`. Byte-level goldens had to be generated.

recipe (golden generation): node is available via `mise x -- node` from
`example/` (v24.16.0). `node -e` with `fetch` is broken in the sandbox but plain
script files run. Copy kit's `create_remote_arg_reducers` / `to_sorted` /
`is_plain_object` / `stringify_remote_arg` / `stringify_command_arg` verbatim out
of `ephemeral/inspiration/reference/kit/packages/kit/src/runtime/shared.js` into
a scratch `.mjs`, replace `#app/internal/transport`'s `encoders` with `{}` and
`base64_encode` with `Buffer.from(bytes).toString('base64')`, and import devalue
straight from `ephemeral/inspiration/reference/devalue/index.js` (5.9.2, matches
kit's `^5.9.0`). No install step, no `node_modules` anywhere in this repo. The
scratch script stays out of the tree (AGENTS.md: no `.mjs` harnesses); regenerate
it from this paragraph when goldens need refreshing.

trap: devalue's `stringify_string` does **not** escape non-ASCII — it writes
runes raw. So the UTF-16-vs-UTF-8 ordering divergence reaches the *nested
devalue strings* that `__skram` and `__skras` sort, not only object keys. All
three sort sites needed the comparator, and all three have a golden that fails
under byte order (verified by temporarily reverting the comparator).

discovery (not in #2): sorting is not the last word on object key order.
`to_sorted` inserts keys in sorted order, but JavaScript then re-hoists
canonical array indices to the front in ascending numeric order at enumeration
time, so `{10,2,B,a}` serializes as `2,10,B,a`, not `10,2,B,a`. `devalue`'s
`propertyOrder` already models this and the Go port was already correct; the
`integer-like keys` golden now pins it so a future "simplify the sort" does not
quietly break it.

decision: the comparator lives in `internal/devalue` (`CompareUTF16`,
`SortStringsUTF16`) rather than in `remotearg`, and `devalue.Stringify`'s
`map[string]any` branch uses it too. That keeps a Go map and the equivalent
`*devalue.Object` sorted by JavaScript producing identical bytes. `Stringify`
keeps sorting maps rather than refusing them — a sorted map is exactly what a
query argument's canonical form is — while `remotearg.StringifyCommandArg`
refuses them, because there nothing is reordered and property order is the
caller's.

decision: the command-arg map guard is a `devalue.Reducer` (`ErrCommandMap`,
internal key `__skgomap`, never reaches the wire). Reducers run on every value
the stringifier visits, so nesting inside objects, arrays, Sets and Map keys and
values is covered without a separate walk — same trick as kit's RegExp guard.

note: nothing in the live server calls `StringifyCommandArg` or
`StringifyQueryArg`; `remote.go`/`remote_live.go` only use `devalue.Stringify` on
the response path. Both issues remain inert until Go constructs a refresh key.

note: `go vet ./example/...` fails without `example/web/build` (`dist.go:14`
embeds `all:build`); it needs a prior `vp build`. Library packages vet and test
clean from the root.
