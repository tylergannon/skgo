# The adapter is generated now (#44)

Branch `claude/adapter-from-module`. Traps only.

## A version stamped into a tracked file has to be reproducible in every tree

`skgo generate` stamps `const SKGO = { version, adapter }` into the
`skgo-adapter.js` it writes, and `example/generate_test.go`
(`TestNothingGeneratedWasWrittenByHand`) regenerates the whole example in a
sandbox and requires byte-for-byte equality with the tree. So anything stamped
has to come out the same in the sandbox as it does under `just generate`, and
`debug.ReadBuildInfo` does not cooperate by default:

- `go tool skgo` makes skgo the *main* module of the tool binary. Under
  `go.work` its version is `(devel)`; in the sandbox, where `example/go.mod`
  requires `github.com/tylergannon/skgo v0.0.0` with a `replace`, build info
  reports the **require line's placeholder** — `v0.0.0` — not devel.
- The replacement is visible: `bi.Main.Replace != nil` (and `dep.Replace` for a
  dependency). `go version -m <binary>` shows it as a `=>` line under `mod`.
  `internal/adapter.Version` treats a replaced module as devel for that reason.

If a future stamp carries anything else derived from the build (a commit, a
timestamp, a path), the same test will catch it — and it will look like "the
generated tree is stale", which is not what it means.

## `regexp.ReplaceAll` silence, and a length check that lies

The stamp guard was "if the output is the same length as the input, nothing
matched, panic". `'unstamped', 'unstamped'` (9 + 9) against `'v0.0.0'` plus a
12-hex fingerprint (6 + 12) is exactly the same length, so the guard fired on a
substitution that had worked perfectly. Use `re.Match` before replacing.

## Driving a remote function without a browser

For a screenshot of state a command produced, the payload kit's client posts is
just base64url of the devalue JSON with the argument at index 0:

    printf '["hello"]' | basenc --base64url | tr -d '='
    curl -X POST $ORIGIN/_app/remote/<hash>/<name> \
      -H 'Content-Type: application/json' -H "Origin: $ORIGIN" \
      -d '{"payload":"<that>"}'

and `example/e2e/node_modules/.bin/playwright screenshot <url> <file>` takes the
picture. No harness file, no suite run, for a one-off page shot.

## The stale adapter fails at startup now, not later and elsewhere

Reproduced in a scaffolded app: swap `web/skgo-adapter.js` for the pre-#44
vendored copy, `vp build`, `go build`, run — the binary refuses with both
identities named. Before this, the same swap in the field surfaced as
`Could not resolve 'esbuild' in skgo-adapter.js` during the *frontend* build,
which points at nothing.
