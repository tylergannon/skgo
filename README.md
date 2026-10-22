# SKGo

A Go backend for SvelteKit. Write remote functions, server loads, actions and
API routes in Go beside your Svelte components. SKGo generates the TypeScript
types and bindings. In production, SvelteKit's renderer runs inside the Go
process, and your application ships as one binary with no Node runtime or cgo.

**Pre-1.0**, targeting SvelteKit 3.1.0. Application server logic is Go;
components and universal loads remain ordinary SvelteKit.

## Get started

On Apple Silicon macOS:

```sh
brew install --cask tylergannon/skgo/skgo
```

Linux AMD64 and ARM64 binaries and checksums are available from
[GitHub Releases](https://github.com/tylergannon/skgo/releases).

Development requires Go **1.27.2**, [VitePlus](https://viteplus.dev) **1.3.0**,
Node `^22.18.0 || ^24.11.0 || >=26.0.0`, and [just](https://just.systems).
The development server supports macOS ARM64 and Linux AMD64.

```sh
skgo new myapp
cd myapp
go tool skgo dev
```

Open the address printed in the terminal. Svelte edits use Vite HMR; compatible
Go edits update the running application, and other changes rebuild it.

To build and run the production binary:

```sh
just build
./bin/myapp
```

See [Creating a project](creating-a-project.md) for setup and
[Deploying](deploying.md) for production configuration.

## Go to Svelte

Declare a query in `web/src/routes/greeting.remote.go`:

```go
package greeting

import (
    "context"
    "github.com/tylergannon/skgo"
)

func greeting(ctx context.Context) (string, error) {
    return "Hello from Go", nil
}

var _ = skgo.Query(greeting)
```

`skgo generate` writes the typed binding. Call it from `+page.svelte`:

```svelte
<script lang="ts">
    import { greeting } from './greeting.remote';
</script>

<p>{await greeting()}</p>
```

## Learn more

- [Remote functions](remote-functions.md) and [project structure](project-structure.md)
- [Development server and limitations](development-server.md)
- [Go API reference](https://pkg.go.dev/github.com/tylergannon/skgo)
- [Example application](example/)
- [CLI generation plugin](plugins/cli/) and [native application plugin](plugins/native-shell/native-shell.md)
- [Coding agent skill](skills/skgo/SKILL.md)

Pronounced “skay-go”. [MIT licensed](LICENSE).
