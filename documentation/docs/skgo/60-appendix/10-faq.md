---
title: Frequently asked questions
summary: Short answers for SvelteKit developers trying SKGo.
---

## Can I write server code in TypeScript?

No. Server loads, form actions, remote functions, `+server` routes and the `handle` hook are Go, and `skgo generate` writes the TypeScript modules Kit compiles, with bodies that throw. An authored `hooks.server.ts` or `env.ts` stops generation, and a server module Kit compiled that SKGo did not generate fails the production build. Universal loads in `+page.ts` and `+layout.ts` stay TypeScript, as in Kit. See [Differences from SvelteKit](differences).

## Do I need Node in production?

No. The binary embeds the frontend build and renders pages with SvelteKit's own renderer running on goja, a JavaScript engine written in Go, so there is no Node process and no cgo. Node is needed to build the frontend and to run [`skgo dev`](/docs/cli/dev).

## How do I set up a database?

Use any Go database library. Open the connection in `cmd/main.go` or `server.go`, and reach it from loads and remote functions however your Go code is organised: a package value, a field your `handle` hook fills in `Locals`, or a service handed in at construction. Pass the request's context to queries so they are cancelled when the visitor leaves; see [Go and web standards](go-standards#context.Context). A pure-Go driver keeps the binary cgo-free.

## How do I use middleware?

Two ways. The `handle` hook in `internal/serverhooks` is a `params.Middleware` with Kit's `handle` semantics, and `params.Sequence` composes several, as Kit's [`sequence`](https://svelte.dev/docs/kit/@sveltejs-kit-hooks#sequence) does; see [Hooks](hooks). Because `NewHandler` returns an ordinary `http.Handler`, you can also wrap it in any `net/http` middleware in `cmd/main.go`; see [Server composition](server-composition).

## Can I deploy to Vercel, Netlify or Cloudflare?

Not currently. SKGo is a long-running server with a warm renderer pool, not a per-request function; [Project types](project-types#Serverless-app) explains, and the site's status page lists these platforms under "Not currently planned". Deploy the binary to a VM or a container platform; see [Deploying](deploying).

## Does it work with JavaScript disabled?

Yes, as in Kit. Pages are server-rendered, form actions accept native posts, and a remote `form` posted without JavaScript is answered by Go with the complete document.

## Why `vp` instead of pnpm, npm or Yarn?

Each release of SKGo is built and tested with one frontend toolchain: VitePlus %vp% and the pnpm %pnpm% it resolves. `vp install`, `vp build` and `vp run` select that pnpm whatever is installed globally, so a recipe that calls `pnpm` directly is a defect. See [Toolchain](toolchain).

## Which Kit and Svelte versions does it use?

Kit %kit% exactly, with a small project-local patch to its prerender queue that [`skgo kit-patch`](/docs/cli/kit-patch) checks, and Svelte %svelte%. `skgo new` scaffolds these versions and `skgo update` brings an application to them. See [Toolchain](toolchain).

## Can I move an existing SvelteKit app to SKGo?

Yes, route by route. See [Migrating from a Node SvelteKit app](migrating).

## Other resources

The source, issues and releases are on [GitHub](https://github.com/tylergannon/skgo), and the Go API is on [pkg.go.dev](https://pkg.go.dev/github.com/tylergannon/skgo). The repository's [example application](https://github.com/tylergannon/skgo/tree/main/example) exercises most features, and every Go listing in this documentation is cut from it. For questions about SvelteKit itself, see Kit's [FAQ](https://svelte.dev/docs/kit/faq).
