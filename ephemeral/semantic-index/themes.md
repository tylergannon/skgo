# themes — cross-cutting facts

## T1. Remote ids are a pure function of the file path
`hash = djb2(UTF-16 units, iterated backwards) → base36` over the vite-root-relative posix
path of the `.remote.*` file; `id = hash + "/" + export`. Same in dev and build. Golden:
`src/lib/todos.remote.ts → worolc`. → `sources/kit/remote-server/ids-and-build.md`,
`sources/kit/remote-client/client-transform.md`. The junkyard's Vite capture plugin
(`sources/junkyard/colocation/fixture-harness.md`) is therefore optional; keep it only as a
drift tripwire in the adapter (`sources/kit/build-adapt/adapter-api.md`).

## T2. devalue is the wire format everywhere
Remote args (base64url of devalue flat JSON, sorted keys for GET kinds, `__skrao/m/s/f`
reducers), remote results (devalue string nested in JSON), `__data.json` nodes, ndjson
chunks, action results — all devalue, with the universal `transport` hook's encoders as
`["<key>", i]` tags. Port devalue first, with its tests. → `sources/libs/devalue.md`,
`sources/kit/portable-tests/devalue-port.md`, `sources/kit/remote-server/serialization.md`,
`sources/kit/server-runtime/data-requests.md`, `sources/kit/client-router/transport-hook.md`.

## T3. CSR-only changes the contract in known places
No SSR means: the shell is kit's fallback page; every server load is a `__data.json` fetch;
non-JS form fallbacks (classic actions without `use:enhance`, `form` remote `?/remote=`)
cannot work; document status on load errors is 200; `prerender` remotes are executed by
Node at build. → `sources/kit/server-runtime/csr-shell.md`, `sources/kit/build-adapt/spa-and-prerender.md`,
`sources/junkyard/app/csr-portability.md`, `sources/kit/server-runtime/actions.md`,
`sources/kit/remote-server/form-and-prerender.md`, `sources/kit/test-apps/no-ssr-guarantees.md`.

## T4. Stubs must be executable by Node but never answer
Kit runs `.remote.*` modules in Node (dev runner / build `analyse`) to learn each export's
kind; the stub's top level is a real `query()/command()` call, only the body throws. Go must
sit in front of `vp dev` for `/_app/remote/*` and `__data.json` or the throwing body 500s.
→ `sources/kit/remote-client/stub-shape.md`, `sources/kit/build-adapt/dev-server.md`.

## T5. Kit 3 breaks kit-2 priors
No `svelte.config.js`; flat `sveltekit({...})`; `#lib` + package.json `imports`;
`$app/tsconfig`; `transport` in universal `hooks.ts`; origin fixed at build (`paths.origin`)
and CSRF always on; TS 6 not 7; `vp`/mise/pnpm devEngines; adapters read `config` not
`config.kit`. → `sources/kit/docs/kit3-facts.md`, `sources/kit/build-adapt/config-kit3.md`,
`sources/junkyard/app/toolchain.md`.

## T6. Route directories vs Go import paths
`[id]`/`(group)` are illegal Go path elements → base32 symlink link tree + a `go.mod`
boundary at `src/routes`; root-route files linked individually. Validated on macOS/Go 1.27;
Linux unverified. Go ignores `_`-prefixed files, so generated Go must not be named
`_data.remote.go`. → `sources/junkyard/colocation/link-tree.md`, `sources/junkyard/go/remotegen.md`.

## T7. Traps the junkyard paid for
Origin/CSRF 403s; `transport` misplaced in `hooks.server.ts`; TS7 breaks sync; `-no-changes`
not read-only in go-gen-jsonschema; registrar bootstrap needing a Node build; `skgo test`
flag bugs; gopls false positives; rejected-generate rollback. → `sources/junkyard/remote-codegen/lessons-and-traps.md`,
`sources/junkyard/colocation/lessons-and-traps.md`, `sources/junkyard/oracle/process-lessons.md`,
`sources/kit/docs/junkyard-plan-facts.md`.

## T8. Proof = Playwright against `BASE_URL`, protocol-agnostic
Count programmatic requests and document loads, assert DOM; never assert on `__data.json`
or remote envelopes. 12 junkyard tasks port unchanged. → `sources/junkyard/app/e2e-suite.md`,
`sources/kit/test-apps/harness-helpers.md`.
