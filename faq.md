---
title: Frequently asked questions
description: Short answers for SvelteKit developers trying SKGo.
division: skgo
section: Appendix
order: 6010
---

## Can I write server code in TypeScript?

No. Server loads, form actions, remote functions, `+server` routes and the `handle` hook are Go, and `skgo generate` writes the TypeScript modules Kit compiles, with bodies that throw. An authored `hooks.server.ts` or `env.ts` stops generation, and a server module Kit compiled that SKGo did not generate fails the production build. Universal loads in `+page.ts` and `+layout.ts` stay TypeScript, as in Kit. See [Differences from SvelteKit](/docs/skgo/differences).

## Do I need Node in production?

No. The binary embeds the frontend build and renders pages with SvelteKit's own renderer running on goja, a JavaScript engine written in Go, so there is no Node process and no cgo. Node is needed to build the frontend and to run [`skgo dev`](/docs/cli/dev).

## How do I set up a database?

Use any Go database library. Open the connection in `cmd/main.go` or `server.go`, and reach it from loads and remote functions however your Go code is organised: a package value, a field your `handle` hook fills in `Locals`, or a service handed in at construction. Pass the request's context to queries so they are cancelled when the visitor leaves; see [Go and web standards](/docs/skgo/go-standards#context.Context). A pure-Go driver keeps the binary cgo-free.

## How do I use middleware?

Two ways. The `handle` hook in `internal/serverhooks` is a `params.Middleware` with Kit's `handle` semantics, and `params.Sequence` composes several, as Kit's [`sequence`](https://svelte.dev/docs/kit/@sveltejs-kit-hooks#sequence) does; see [Hooks](/docs/skgo/hooks). Because `NewHandler` returns an ordinary `http.Handler`, you can also wrap it in any `net/http` middleware in `cmd/main.go`; see [Server composition](/docs/skgo/server-composition).

## Can I deploy to Vercel, Netlify or Cloudflare?

Not currently. SKGo is a long-running server with a warm renderer pool, not a per-request function; [Project types](/docs/skgo/project-types#Serverless-app) explains, and the site's status page lists these platforms under "Not currently planned". Deploy the binary to a VM or a container platform; see [Deploying](/docs/skgo/deploying).

## Does it work with JavaScript disabled?

Yes, as in Kit. Pages are server-rendered, form actions accept native posts, and a remote `form` posted without JavaScript is answered by Go with the complete document.

## Why `vp` instead of pnpm, npm or Yarn?

Each release of SKGo is built and tested with one frontend toolchain: one version of VitePlus and the pnpm it resolves. `vp install`, `vp build` and `vp run` select that pnpm whatever is installed globally, so a recipe that calls `pnpm` directly is a defect. See [Prerequisites](/docs/skgo/creating-a-project#Prerequisites).

## Which Kit and Svelte versions does it use?

One exact Kit release, with a small project-local patch to its prerender queue that [`skgo kit-patch`](/docs/cli/kit-patch) checks, and one exact Svelte release. `skgo new` scaffolds these versions and `skgo update` brings an application to them. See [Prerequisites](/docs/skgo/creating-a-project#Prerequisites).

## Can I move an existing SvelteKit app to SKGo?

Yes, route by route. See [Migrating from a Node SvelteKit app](/docs/skgo/migrating).

## Other resources

The source, issues and releases are on [GitHub](https://github.com/tylergannon/skgo), and the Go API is on [pkg.go.dev](https://pkg.go.dev/github.com/tylergannon/skgo). The repository's [example application](https://github.com/tylergannon/skgo/tree/main/example) exercises most features, and every code sample in this documentation is compiled code, either a file from it or an example from SKGo's own tests. For questions about SvelteKit itself, see Kit's [FAQ](https://svelte.dev/docs/kit/faq).
