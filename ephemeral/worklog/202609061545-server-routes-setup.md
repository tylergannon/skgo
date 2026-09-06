# server-routes — setup traps and baseline

## A fresh worktree has no `ephemeral/inspiration`

`/ephemeral/inspiration` is gitignored, so it exists only in the root checkout.
A worktree starts without the pinned kit source the whole project treats as the
specification, and an agent told to "read kit at
ephemeral/inspiration/reference/kit" finds an empty directory and quietly
proceeds on priors. Symlink it in first:

    ln -s /Users/tyler/src/skgo/ephemeral/inspiration ephemeral/inspiration

It stays gitignored, so it cannot be committed by accident.

## A fresh worktree has no build, and `example/...` tests need one

`example/web.Build` is an embed of `example/web/build`, so `go test ./example/...`
cannot run until, in order: `pnpm install` in `example/web` and `example/e2e`,
`go generate ./...` from `example/generated`, then `vp build` in `example/web`.
`go vet ./...` on the root package alone passes without any of it, which is
exactly the shape of a check that reports success over an untested tree.

## Baseline, measured 2026-09-06 before any change

- No route in the example app has a `+server.ts`. The endpoint bug is therefore
  not observable in the example app as it stands; a scenario for it needs a
  route added first.
- `skgo.manifest.json` carries `appDir, base, version, nodes, routes, remotes`.
  Its route entries carry `id, pattern, params, page` and **no endpoint field**:
  the adapter never asks kit which routes have a `+server.ts`, so Go cannot know
  an endpoint route from a page route, and `staticHandler.matchesRoute` answers
  an endpoint-only route 200 with the boot document.
- `vp build` writes no `prerendered/` directory: nothing in the example app is
  prerendered today. "A prerendered page is served as prerendered" needs a
  prerendered page to exist before it can be a load-bearing scenario — otherwise
  it passes over an empty directory.
- `build/` holds `client/`, `index.html`, `skgo.manifest.json` and nothing else.
  No `.br` or `.gz` anywhere: the adapter never calls `builder.compress`.

## Ports in use on this machine

Another run holds `127.0.0.1:8321`. The example's own dev stack (`vp dev` on
5173, Go on 8080) was free. Check `lsof -nP -iTCP -sTCP:LISTEN` before binding.
