# @skgo/sveltekit-adapter

The runtime SvelteKit adapter for [skgo](https://github.com/tylergannon/skgo), a
SvelteKit application server written in Go.

The app stays plain SvelteKit. This adapter builds it into the shape a skgo
binary embeds: the client bundle, kit's SPA fallback document, the SSR bundle
the Go process renders pages with, and a manifest naming the routes, remote
functions and server loads Go must answer.

```sh
pnpm add -D '@skgo/sveltekit-adapter@<=VERSION'
```

`VERSION` is the `github.com/tylergannon/skgo` requirement in your `go.mod`
without its `v`: for skgo `v0.3.4`, ask for `<=0.3.4`. A skgo release publishes
this package only when the package itself changes, so the newest version at or
below yours is the one carrying your skgo's adapter, and skgo refuses to start
with any other. If pnpm's minimum release age is enabled, exclude this package;
otherwise pnpm may quietly install an older adapter immediately after a release
that changes it.

```js
// vite.config.ts
import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';
import skgo from '@skgo/sveltekit-adapter';

export default defineConfig({
	plugins: [sveltekit({ adapter: skgo(), experimental: { remoteFunctions: true } })]
});
```

The adapter and the Go module are one contract in two languages, released
together under one version. Every build records which adapter wrote it, and a
skgo binary refuses a build written by a different one, naming both — so an
install that has fallen behind fails at startup with something a developer can
act on rather than as an unrelated error later.

## Kit prerender queue compatibility

Declared prerender `Inputs` require the Kit 3.0.0 queue correction shipped with
skgo. New projects receive the project-local patch configuration during
scaffolding. For an existing project, run `go tool skgo kit-patch --web web
--apply` from the Go application root, then install with the frontend's native
VitePlus command, `cd web && node_modules/.bin/vp install
--no-frozen-lockfile`. Finish by running `go tool skgo kit-patch --web web
--check` from the application root. The check verifies the actual installed
Kit queue source as well as the committed configuration; applying the patch
alone does not install dependencies.

## License

[MIT](LICENSE)
