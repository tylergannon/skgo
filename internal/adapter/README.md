# @skgo/adapter

The SvelteKit adapter for [skgo](https://github.com/tylergannon/skgo), a
SvelteKit application server written in Go.

The app stays plain SvelteKit. This adapter builds it into the shape a skgo
binary embeds: the client bundle, kit's SPA fallback document, the SSR bundle
the Go process renders pages with, and a manifest naming the routes, remote
functions and server loads Go must answer.

```sh
pnpm add -D "github:tylergannon/skgo#v0.2.0&path:internal/adapter"
```

The tag is the skgo version your `go.mod` requires; pnpm installs the package
straight from that tag's `internal/adapter`. A project made by `skgo new` has
this line already. When the package is on npm the spec becomes
`pnpm add -D @skgo/adapter`.

```js
// vite.config.ts
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import skgo from '@skgo/adapter';

export default defineConfig({
	plugins: [sveltekit({ adapter: skgo(), experimental: { remoteFunctions: true } })]
});
```

The adapter and the Go module are one contract in two languages, released
together under one version. Every build records which adapter wrote it, and a
skgo binary refuses a build written by a different one, naming both — so an
install that has fallen behind fails at startup with something a developer can
act on rather than as an unrelated error later.

## License

[MIT](LICENSE)
