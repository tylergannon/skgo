# devalue moves to polytype

correction to the brief's premise: the two runtimes are not "similar and not
identical" in any way that mattered. `polytype/devalue` at v1.0.0-rc.10 is
line-for-line the same port as the deleted `internal/devalue`, plus a `uint8`
case in `asFloat` and some `//nolint:staticcheck` comments on error strings that
reproduce devalue's own capitalisation. Exported API is identical, symbol for
symbol. The migration was an import rewrite; nothing needed a shim, and no shape
failed to express. If the packages diverge later this paragraph is the thing
that stops being true, so check the diff before assuming it still is:

    diff -ru internal-devalue-at-252e2ce \
      "$(go env GOMODCACHE)/github.com/tylergannon/polytype@<ver>/devalue"

trap: the module cache. `go get` writing `v1.0.0-rc.10` into go.mod proves
nothing about what compiled. `go list -f '{{.Dir}}' github.com/tylergannon/
polytype/devalue` prints the directory the build actually read; that is the
check worth doing, and it is the only one that would have caught a stale cache.

fact: `just generate` under rc.10 produces byte-identical output to rc.9 — the
JSON Schema projection for remote-function arguments is unaffected by the bump.
Verified by regenerating into a clean tree and getting no diff.

## the anchors, and why there are two

`internal/remotearg/kitgolden_test.go` pins bytes produced by *kit's own*
`stringify_remote_arg` / `stringify_command_arg`. It moved to the new import
unchanged and still passes: 23 query goldens, 4 command goldens. That is the
conformance statement that matters, and it is anchored to kit, not to whichever
Go package happens to implement the codec.

`devalue_wire_test.go` (new, repo root) is the layer below it: 38 shapes ×
2 directions, pinned to bytes recorded from the pinned JavaScript devalue 5.9.2
itself. It exists because the kit goldens exercise the *argument* direction and
skip BigInt, RegExp, ArrayBuffer and boxed primitives entirely — shapes both
ports document and neither had measured against the real thing from inside skgo.

Recording recipe (the generator stays out of the tree, per AGENTS.md): import
`stringify`/`parse` straight from
`/Users/tyler/src/skgo/ephemeral/inspiration/reference/devalue/index.js`, build
each value in JavaScript, print `{name, devalue: stringify(value)}`, run it with
`mise x -- node` from `example/`. No install step. Transcribe the Go value into
the case by hand — reading the Go value back off the codec is how this test
would turn into polytype agreeing with polytype.

trap that shaped the test: byte-equality round-trip alone (`Parse` then
`Stringify` reproduces the document) is satisfied by any self-consistent codec,
including a wrong one. The emit half — hand-written Go value must produce
devalue's recorded bytes — is what actually bites. Both halves are present;
deleting the emit half would hollow the file out while leaving it green.

## gap found, not fixed here

`example/e2e/playwright.config.ts` sets `screenshot: 'only-on-failure'`, so a
green run leaves no frames to look at — exactly the unexaminable exit code
AGENTS.md says is not evidence. Flipped to `'on'` for this run's screenshots and
reverted, to keep a refactor's diff a refactor's diff. Whoever touches the suite
next should make it permanent.
