# Sprint 001: Bare Go Server Fronting SvelteKit (Dev Proxy + Embedded Prod Build)

## Pyramid Index

- **L0**: Stand up the core `skgo` library and `example/` repository shape, implementing a dual-mode Go HTTP server that proxies to `vp dev` (with WebSocket HMR forwarding) under Air and Overmind in development, and serves compiled SvelteKit 3 client assets and the prerendered SPA shell from an `embed.FS` in production, validated end-to-end via `playwright-bdd` scenarios against both modes.
- **L1**:
  - **Repository & Module Shape**: Root Go module `github.com/tylergannon/skgo`, `example` module with `cmd/`, `web/`, and `e2e/`; strict separation keeping SvelteKit free of Go-isms.
  - **Dual-Mode HTTP Core**: `internal/proxy` for dev mode (`httputil.ReverseProxy` with HTTP/1.1 WebSocket upgrade for Vite HMR) and `internal/static` for prod mode (sirv-style static file server from `embed.FS` with immutable caching, precompression negotiation, and root SPA shell catch-all fallback).
  - **Pinned Toolchain & Web App**: Pinned SvelteKit 3 (`3.0.0-next.25`), Svelte 5 (`5.56.10`), Vite 8 (`8.2.2`), Vite+ (`0.3.0`), TypeScript (`6.0.3`), Node (`24.16.0`), and `@sveltejs/adapter-node` (`6.0.0-next.10`) with `paths.origin` fixed at build time, `paths.relative: false`, `#lib` imports, and `export const ssr = false; export const prerender = true;` in the root layout.
  - **Dev Orchestration**: `example/Procfile` running Overmind (`web: mise x -- vp dev`, `server: air -c .air.toml`) with `.air.toml` configured to build and run `./cmd` with `--proxy http://127.0.0.1:5173`.
  - **Acceptance & Proof**: `example/e2e` package running `playwright-bdd` against `BASE_URL` in both dev (Go+Overmind) and prod (bare binary) modes, proving DOM component rendering, client-side navigation without document reload, dynamic deep linking, and 404 client error fallback; Go unit tests for proxy and static handlers.
  - **Risks & Guardrails**: Safe `embed.FS` layout with checked-in placeholders to ensure clean builds on fresh checkouts, explicit WebSocket proxying verification, and exact `paths.origin` alignment.
- **L2**:
  - Repo shape and boundaries → §[Architecture](#architecture) and §[Files Summary](#files-summary).
  - Pinned versions and toolchain configuration → §[Dependencies](#dependencies) and §[Phase 1: Skeleton & Toolchain](#phase-1-project-skeleton-modules--toolchain-setup).
  - Static file algorithm & SPA shell fallback → §[Architecture: Static Serving Engine](#static-serving-engine-internalstatic) and §[Phase 2: Core Library](#phase-2-core-go-library-skgo).
  - Dev proxy & WebSocket HMR tunneling → §[Architecture: Dev Reverse Proxy Engine](#dev-reverse-proxy-engine-internalproxy) and §[Phase 2: Core Library](#phase-2-core-go-library-skgo).
  - Process management via Overmind & Air → §[Architecture: Process Orchestration](#process-orchestration-overmind--air) and §[Phase 3: Example CLI & Tooling](#phase-3-example-cli--dev-tooling-configuration).
  - Playwright-BDD test scenarios & verification → §[Implementation Plan: Phase 4](#phase-4-playwright-bdd-e2e-acceptance-suite) and §[Definition of Done](#definition-of-done).
  - Embed layout & open decisions → §[Open Questions](#open-questions) and §[Risks & Mitigations](#risks--mitigations).

---

## Overview

Sprint 001 establishes the foundational runtime for `skgo`: a bare Go server fronting a real SvelteKit application in both development and production modes. Prior attempts stalled by attempting to construct extensive deductive proof systems and complex SSR sidecars before demonstrating a basic working server. This sprint executes on the vision established in `ephemeral/brief/2026-09-05-skgo-vision.md` and refined in `ephemeral/plans/2026-09-05-skgo-build-plan.md` and `ephemeral/sprints/drafts/SPRINT-001-INTENT.md`: build a working server first, verify it with real browser tests, and defer remote functions, server loads, and code generation to subsequent sprints.

In production, Go owns the socket and the process. There is no Node.js process, no SSR runtime, and no JavaScript server execution. Go serves compiled client assets (`build/client`) and prerendered outputs (`build/prerendered`) embedded directly into the Go executable via `embed.FS`, applying sirv-equivalent HTTP caching, weak ETags, and precompressed asset negotiation. Dynamic client-side routes receive the prerendered SPA boot shell, allowing SvelteKit's client-side router to boot and hydrate seamlessly.

In development, Go acts as a transparent reverse proxy to the Vite+ dev server (`vp dev`), tunneling HTTP requests and WebSocket upgrades (HMR). Air monitors Go source files and triggers instant recompilation and restart of the Go server, while Overmind coordinates the Vite and Air processes under a unified `Procfile`.

Verification is achieved through automated `playwright-bdd` scenarios executed against both the development proxy and the standalone production binary via `BASE_URL`.

---

## Use Cases

### UC-1: Development Workflow with Hot Module Replacement
- **Actor**: Full-stack developer developing SvelteKit frontend and Go backend services.
- **Workflow**:
  1. Developer navigates to `example/` and runs `overmind start`.
  2. Overmind launches `web` (`mise x -- vp dev --host 127.0.0.1 --port 5173`) and `server` (`air -c .air.toml`).
  3. Air compiles `example/cmd/main.go` and launches the Go binary listening on `:8080` with `--proxy http://127.0.0.1:5173`.
  4. Developer visits `http://127.0.0.1:8080` in a browser. The Go server proxies the initial request and client bundle from Vite.
  5. The browser establishes an HMR WebSocket connection to `ws://127.0.0.1:8080/`. The Go reverse proxy upgrades the connection to WebSocket and tunnels frames to `127.0.0.1:5173`.
  6. Developer edits `example/web/src/routes/+page.svelte`. Vite pushes an HMR update through the WebSocket tunnel; the browser reflects the UI update without a full reload.
  7. Developer edits `skgo/proxy.go` or `example/cmd/main.go`. Air detects the file change, recompiles the binary, and restarts the Go server within 500ms.

### UC-2: Standalone Production Deployment
- **Actor**: Release engineer / deployment pipeline.
- **Workflow**:
  1. Pipeline executes `mise x -- vp build` inside `example/web`. Adapter-node emits client assets to `example/web/build/client` and prerendered files to `example/web/build/prerendered`.
  2. Pipeline executes `go build -o /app/server ./cmd` inside `example`. Go embeds the compiled static assets into the single binary.
  3. The `/app/server` binary is deployed to a container without Node.js, npm, or external web servers.
  4. The container executes `/app/server --listen :8080`.
  5. A user requests `http://example.com/`. Go serves the prerendered HTML shell with `Content-Type: text/html; charset=utf-8` and `Cache-Control: no-cache`.
  6. The browser requests `/_app/immutable/entry/start.[hash].js`. Go serves the file directly from `embed.FS` with `Cache-Control: public, max-age=31536000, immutable`.
  7. If the client sends `Accept-Encoding: br` and `start.[hash].js.br` exists, Go serves the precompressed Brotli payload with `Content-Encoding: br` and `Vary: Accept-Encoding`.

### UC-3: Client-Side Deep Linking and Routing
- **Actor**: Web user navigating directly to a nested route.
- **Workflow**:
  1. User pastes a deep link `http://example.com/items/42` into the browser address bar.
  2. The path `/items/42` is not a static file on disk. Go's static handler falls back to the SPA boot shell (`prerendered/index.html`), returning HTTP 200 with HTML headers.
  3. SvelteKit's client router boots from the shell, parses `window.location.pathname`, matches the `/items/[id]` route client-side, extracts parameter `id = "42"`, and renders the item view without server-side error.

### UC-4: Client-Side 404 Error Handling
- **Actor**: Web user navigating to a non-existent URL.
- **Workflow**:
  1. User visits `http://example.com/non-existent-page`.
  2. Go's static handler serves the SPA boot shell.
  3. SvelteKit's client-side router initializes, executes route matching, detects no matching route in its client manifest, and mounts `+error.svelte` displaying a 404 Not Found error page.

---

## Architecture

### Target Repository Layout

```
skgo/                                  # Root module: github.com/tylergannon/skgo
├── go.mod                             # module github.com/tylergannon/skgo (Go 1.24)
├── proxy.go                           # Public DevProxy constructor & handler API
├── static.go                          # Public StaticServer constructor & handler API
├── server.go                          # High-level Server wiring & options
├── internal/
│   ├── proxy/
│   │   ├── handler.go                 # httputil.ReverseProxy + WebSocket upgrade tunneling
│   │   └── handler_test.go            # HTTP proxy and WebSocket upgrade unit tests
│   └── static/
│       ├── handler.go                 # sirv-style fs.FS file lookup & caching handler
│       ├── handler_test.go            # Table-driven static serving, ETag, and fallback tests
│       └── mrmime.go                  # Embedded MIME lookup table matching mrmime 2.0.1
example/                               # Example application module
├── go.mod                             # module github.com/tylergannon/skgo/example
├── Procfile                           # Overmind process manager configuration
├── .air.toml                          # Air configuration for live Go reloads
├── cmd/
│   └── main.go                        # CLI entrypoint with flag parsing (--listen, --proxy)
├── web/                               # SvelteKit application root (Vite root)
│   ├── package.json                   # Pinned JS dependencies and devEngines
│   ├── pnpm-workspace.yaml            # pnpm configuration
│   ├── vite.config.ts                 # SvelteKit Vite configuration (adapter-node)
│   ├── tsconfig.json                  # Extends $app/tsconfig
│   ├── mise.toml                      # Toolchain pins: node 24.16.0, vite-plus 0.3.0
│   ├── dist.go                        # Embed declaration: //go:embed all:build/client all:build/prerendered
│   ├── build/                         # Build output (gitignored except placeholders)
│   │   ├── client/
│   │   │   └── .gitkeep               # Checked-in placeholder for clean go build
│   │   └── prerendered/
│   │       └── .gitkeep               # Checked-in placeholder for clean go build
│   └── src/
│       ├── app.html                   # HTML template with %sveltekit.head% and %sveltekit.body%
│       └── routes/
│           ├── +layout.ts             # export const ssr = false; export const prerender = true;
│           ├── +layout.svelte         # Shell layout with navigation bar and hydrated marker
│           ├── +page.svelte           # Home page: Svelte 5 counter component
│           ├── about/
│           │   └── +page.svelte       # Static about page for client-side navigation
│           ├── items/
│           │   └── [id]/
│           │       ├── +page.ts       # export const prerender = false;
│           │       └── +page.svelte   # Dynamic deep-link route showing params
│           └── +error.svelte          # Root error component for client 404 display
└── e2e/                               # Acceptance test suite (playwright-bdd)
    ├── package.json                   # Test runner dependencies (@playwright/test, playwright-bdd)
    ├── playwright.config.ts           # Playwright BDD configuration targeting BASE_URL
    ├── tsconfig.json                  # TS config for step definitions
    ├── features/
    │   ├── rendering.feature          # Feature: Svelte component rendering through Go
    │   ├── navigation.feature         # Feature: Client-side routing without full reload
    │   ├── deep_link.feature          # Feature: Direct deep linking to dynamic routes
    │   └── not_found.feature          # Feature: Client-side 404 error page handling
    └── steps/
        └── app.steps.ts               # Step definitions (Given/When/Then)
```

---

### Static Serving Engine (`internal/static`)

The static serving engine implements the behavior specified in `sources/kit/build-adapt/static-serving.md` and `sources/libs/sirv.md`. It operates on an `io/fs.FS` provided by Go's `embed` package.

#### Request Processing Pipeline

```
Incoming Request (GET / HEAD)
│
├── 1. Clean & Decode Path
│      Strip query string, normalize unicode (NFC), decode percent-encoding (preserve reserved delimiters)
│
├── 2. Immutable Asset Check (path starts with `/_app/immutable/`)
│      ├── Exact File Match in `client/`:
│      │   └── Serve file with `Cache-Control: public, max-age=31536000, immutable`
│      └── File Not Found:
│          └── Return 404 Not Found with `Cache-Control: no-store` (NEVER fallback to SPA shell)
│
├── 3. Version File Check (path equals `/_app/version.json`)
│      └── Serve with `Content-Type: application/json` and `Cache-Control: no-cache`
│
├── 4. Other Static Assets in `client/` (e.g. `favicon.ico`, `logo.svg`, robots.txt)
│      └── If file exists: serve with weak ETag and `Cache-Control: max-age=0, must-revalidate`
│
├── 5. Prerendered HTML & Dependencies in `prerendered/`
│      ├── If path exists as `<path>.html` or `<path>/index.html`:
│      │   └── Serve with `Content-Type: text/html; charset=utf-8` and weak ETag
│      └── If trailing slash inverse exists:
│          └── Return 308 Permanent Redirect with relative `Location` (e.g. `about/` → `../about`)
│
└── 6. Dynamic Route SPA Catch-All
       └── If Request Method is GET or HEAD and Accept header allows `text/html`:
           └── Serve prerendered boot shell (`prerendered/index.html`)
               Headers: `Content-Type: text/html; charset=utf-8`, `Cache-Control: no-cache`, Status: 200
```

#### Precompression Negotiation
For any static file request:
1. Parse incoming `Accept-Encoding` header.
2. If `br` is supported and `<file>.br` exists in the filesystem:
   - Serve compressed content with `Content-Encoding: br` and `Vary: Accept-Encoding`.
3. Else if `gzip` is supported and `<file>.gz` exists:
   - Serve compressed content with `Content-Encoding: gzip` and `Vary: Accept-Encoding`.
4. Else serve uncompressed file with `Vary: Accept-Encoding` if a compressed variant exists on disk.

#### Weak ETag and Conditional Requests
- ETag format: `W/"<size>-<mtimeMs>"`.
- If incoming request contains `If-None-Match: <etag>`, respond immediately with `304 Not Modified` and empty body.

#### MIME Table Parity
`internal/static/mrmime.go` embeds the exact MIME mappings from `mrmime 2.0.1` (see `sources/libs/mrmime.md`):
- `.js`, `.mjs` → `text/javascript`
- `.css` → `text/css`
- `.html`, `.htm` → `text/html; charset=utf-8`
- `.json` → `application/json`
- `.wasm` → `application/wasm`
- `.svg` → `image/svg+xml`
- `.png`, `.jpg`, `.jpeg`, `.webp`, `.avif`, `.gif` → respective image MIME types.

---

### Dev Reverse Proxy Engine (`internal/proxy`)

The development reverse proxy fronts the Vite dev server (`vp dev` running on `http://127.0.0.1:5173`).

#### Reverse Proxy Architecture
- Utilizes `net/http/httputil.ReverseProxy`.
- `Director` function:
  - Preserves incoming path and query parameters.
  - Rewrites `req.URL.Scheme` to target scheme (`http`).
  - Rewrites `req.URL.Host` to target host (`127.0.0.1:5173`).
  - Sets standard proxy headers: `X-Forwarded-Host`, `X-Forwarded-Proto`, and `X-Forwarded-For`.
  - Sets `Host` header to `127.0.0.1:5173` to prevent Vite from rejecting the host header if `server.allowedHosts` is active.

#### WebSocket HMR Tunneling
- Standard `httputil.ReverseProxy` in Go ≥1.12 natively supports HTTP/1.1 connection hijacking and bidirectional frame forwarding when request headers contain:
  - `Connection: Upgrade`
  - `Upgrade: websocket`
- When Vite's client script connects to `ws://127.0.0.1:8080/`, Go upgrades the connection to HTTP 101 Switching Protocols and transparently tunnels the duplex stream between browser and Vite dev server.

---

### Embedding Design & Filesystem Layout

Go's `embed` package requires that files to be embedded reside within the directory tree of the embedding package (no `..` relative paths allowed).

#### Solution: `example/web/dist.go`
The embedding package lives inside `example/web`:
```go
package web

import "embed"

// DistFS embeds the built client assets and prerendered outputs.
// Placeholders (.gitkeep) ensure compilation succeeds before running vp build.
//go:embed all:build/client all:build/prerendered
var DistFS embed.FS
```

#### Safe Checkout Guard
To prevent `go build ./example/cmd` from failing on a fresh repository clone before `mise x -- vp build` is executed:
- Placeholders `example/web/build/client/.gitkeep` and `example/web/build/prerendered/.gitkeep` are tracked in git.
- `.gitignore` ignores `example/web/build/*` while whitelisting the `.gitkeep` files:
  ```gitignore
  example/web/build/*
  !example/web/build/client/.gitkeep
  !example/web/build/prerendered/.gitkeep
  ```
- If the binary is run with only placeholder files present, the server logs a helpful error message: `"skgo: embedded assets are empty; run 'mise x -- vp build' in example/web before building production binary"`.

---

### Process Orchestration (`overmind` + `air`)

In development, Overmind coordinates the frontend dev server and the backend rebuilder via `example/Procfile`:

```procfile
web: cd web && mise x -- vp dev --host 127.0.0.1 --port 5173
server: air -c .air.toml
```

#### Air Configuration (`example/.air.toml`)
```toml
root = ".."
test_data_dir = ""
tmp_dir = "tmp"

[build]
  bin = "./tmp/example-server --listen :8080 --proxy http://127.0.0.1:5173"
  cmd = "cd example && go build -o tmp/example-server ./cmd"
  delay = 500
  exclude_dir = ["example/web", "example/e2e", "example/tmp", ".git"]
  include_ext = ["go"]
  stop_on_error = true

[log]
  time = true

[color]
  main = "yellow"
  watcher = "cyan"
  build = "green"
  runner = "magenta"
```

---

## Implementation Plan

### Phase 1: Project Skeleton, Modules & Toolchain Setup

#### Tasks & Files

- **Task 1.1: Initialize Root and Example Go Modules**
  - Path: `go.mod` (root)
    - Module name: `github.com/tylergannon/skgo`
    - Go version: `1.24.0` (compatible with modern macOS arm64/Linux)
  - Path: `example/go.mod`
    - Module name: `github.com/tylergannon/skgo/example`
    - Directive: `replace github.com/tylergannon/skgo => ../`
  - Command: `cd example && go mod tidy`

- **Task 1.2: Configure Web Toolchain in `example/web`**
  - Path: `example/web/mise.toml`
    - Exact pins:
      ```toml
      [tools]
      node = "24.16.0"
      "npm:vite-plus" = "0.3.0"
      ```
  - Path: `example/web/package.json`
    - Exact pins matching junkyard reference:
      ```json
      {
        "name": "example-web",
        "version": "0.0.1",
        "private": true,
        "type": "module",
        "imports": {
          "#lib": "./src/lib/index.js",
          "#lib/*": "./src/lib/*"
        },
        "scripts": {
          "dev": "vp dev",
          "build": "vp build",
          "check": "vp check"
        },
        "devDependencies": {
          "@sveltejs/adapter-node": "6.0.0-next.10",
          "@sveltejs/kit": "3.0.0-next.25",
          "@sveltejs/vite-plugin-svelte": "7.3.0",
          "@types/node": "26.4.1",
          "svelte": "5.56.10",
          "typescript": "6.0.3",
          "vite": "8.2.2"
        },
        "devEngines": {
          "packageManager": {
            "name": "pnpm",
            "version": "11.25.0",
            "onFail": "download"
          }
        }
      }
      ```
  - Path: `example/web/pnpm-workspace.yaml`
    - Bypasses pnpm minimumReleaseAge check for newly published types:
      ```yaml
      onlyBuiltDependencies: []
      ```
  - Path: `example/web/tsconfig.json`
    - Extends Kit 3's virtual `$app/tsconfig`:
      ```json
      {
        "extends": "$app/tsconfig",
        "compilerOptions": {
          "strict": true
        },
        "include": ["src/**/*"]
      }
      ```
  - Path: `example/web/vite.config.ts`
    - Kit 3 flat configuration:
      ```typescript
      import { sveltekit } from '@sveltejs/kit/vite';
      import adapter from '@sveltejs/adapter-node';
      import { defineConfig } from 'vite';

      export default defineConfig({
        plugins: [
          sveltekit({
            adapter: adapter({
              out: 'build',
              precompress: true
            }),
            paths: {
              origin: process.env.ORIGIN ?? 'http://127.0.0.1:8080',
              relative: false
            }
          })
        ]
      });
      ```

- **Task 1.3: Author Minimal SvelteKit Application Source**
  - Path: `example/web/src/app.html`
    - HTML shell template containing `%sveltekit.head%` and `<div style="display: contents">%sveltekit.body%</div>`.
  - Path: `example/web/src/routes/+layout.ts`
    - Disable SSR and enable prerendering of root:
      ```typescript
      export const ssr = false;
      export const prerender = true;
      ```
  - Path: `example/web/src/routes/+layout.svelte`
    - Layout markup providing navigation links (`/`, `/about`, `/items/42`, `/non-existent`) and a hydration marker `<div data-testid="hydrated" data-status={hydrated ? "yes" : "no"}></div>`.
  - Path: `example/web/src/routes/+page.svelte`
    - Home page displaying a greeting and an interactive Svelte 5 counter:
      ```svelte
      <script lang="ts">
        let count = $state(0);
      </script>

      <h1 data-testid="page-title">skgo Home</h1>
      <p data-testid="greeting">SvelteKit running through Go</p>
      <button data-testid="counter-btn" onclick={() => count++}>Count: {count}</button>
      ```
  - Path: `example/web/src/routes/about/+page.svelte`
    - Static about page with `data-testid="about-title"`.
  - Path: `example/web/src/routes/items/[id]/+page.ts`
    - Disable prerendering for dynamic route:
      ```typescript
      export const prerender = false;
      ```
  - Path: `example/web/src/routes/items/[id]/+page.svelte`
    - Dynamic route rendering route parameter:
      ```svelte
      <script lang="ts">
        import { page } from '$app/state';
      </script>

      <h1 data-testid="item-title">Item Detail</h1>
      <p data-testid="item-id">ID: {page.params.id}</p>
      ```
  - Path: `example/web/src/routes/+error.svelte`
    - Root client error component:
      ```svelte
      <script lang="ts">
        import { page } from '$app/state';
      </script>

      <h1 data-testid="error-status">{page.status}</h1>
      <p data-testid="error-message">{page.error?.message ?? 'Not Found'}</p>
      ```

- **Task 1.4: Track Embedding Placeholders**
  - Paths:
    - `example/web/build/client/.gitkeep`
    - `example/web/build/prerendered/.gitkeep`
    - `example/web/dist.go` (defining `package web` and `var DistFS embed.FS`)
  - Run: `cd example/web && mise x -- pnpm install` to verify clean installation.

---

### Phase 2: Core Go Library (`skgo`)

#### Tasks & Files

- **Task 2.1: Implement MIME Registry (`internal/static/mrmime.go`)**
  - Package: `package static`
  - Function: `func LookupMIME(filename string) string`
  - Maps file extensions to canonical MIME types derived from `mrmime 2.0.1`:
    - `.js`, `.mjs` → `text/javascript`
    - `.css` → `text/css`
    - `.html`, `.htm` → `text/html; charset=utf-8`
    - `.json` → `application/json`
    - `.wasm` → `application/wasm`
    - `.svg` → `image/svg+xml`
    - `.png`, `.jpg`, `.jpeg`, `.webp`, `.avif`, `.gif` → respective image MIME types.

- **Task 2.2: Implement Static Handler (`internal/static/handler.go`)**
  - Package: `package static`
  - Types:
    ```go
    type Options struct {
        ClientSubdir      string // default: "client"
        PrerenderedSubdir string // default: "prerendered"
        FallbackHTML      string // default: "index.html"
    }

    type Handler struct {
        fs         fs.FS
        clientFS   fs.FS
        prerenderFS fs.FS
        opts       Options
    }

    func NewHandler(embeddedFS fs.FS, opts Options) (*Handler, error)
    func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
    ```
  - Implementation details:
    - `fs.Sub(embeddedFS, "build")` isolates the root.
    - Path cleansing using `path.Clean("/" + r.URL.EscapedPath())`.
    - Routing logic:
      - Prefix `/_app/immutable/`: if file exists, set `Cache-Control: public, max-age=31536000, immutable`. If missing, write `404 Not Found` with `Cache-Control: no-store` immediately.
      - Path `/_app/version.json`: set `Cache-Control: no-cache`.
      - Other files in `client/`: set `Cache-Control: max-age=0, must-revalidate`.
      - Check `prerendered/` for `<path>.html` or `<path>/index.html`. Handle trailing-slash 308 redirects.
      - Catch-all fallback: serve `prerendered/index.html` with status 200, `Content-Type: text/html; charset=utf-8`, and `Cache-Control: no-cache`.
    - Compression negotiation for `.br` and `.gz` siblings.
    - ETag calculation: `W/"<size>-<mtime>"`, checking `If-None-Match` to return `304 Not Modified`.

- **Task 2.3: Implement Dev Reverse Proxy (`internal/proxy/handler.go`)**
  - Package: `package proxy`
  - Types:
    ```go
    type Handler struct {
        target *url.URL
        proxy  *httputil.ReverseProxy
    }

    func NewHandler(targetURL *url.URL) *Handler
    func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request)
    ```
  - Implementation details:
    - Rewrites incoming requests to `target.Scheme` and `target.Host`.
    - Transparently supports WebSocket upgrades via standard Go `httputil.ReverseProxy` behavior.
    - Sets `X-Forwarded-Host` and `X-Forwarded-Proto`.
    - Rewrites `Host` header to target host (`127.0.0.1:5173`).

- **Task 2.4: Expose Public Library API**
  - Path: `static.go` (package `skgo`):
    ```go
    package skgo

    import (
        "io/fs"
        "net/http"
        "github.com/tylergannon/skgo/internal/static"
    )

    type StaticOptions = static.Options

    func NewStaticServer(distFS fs.FS, opts StaticOptions) (http.Handler, error) {
        return static.NewHandler(distFS, opts)
    }
    ```
  - Path: `proxy.go` (package `skgo`):
    ```go
    package skgo

    import (
        "net/http"
        "net/url"
        "github.com/tylergannon/skgo/internal/proxy"
    )

    func NewDevProxy(targetURL string) (http.Handler, error) {
        u, err := url.Parse(targetURL)
        if err != nil {
            return nil, err
        }
        return proxy.NewHandler(u), nil
    }
    ```

- **Task 2.5: Write Go Unit Tests**
  - Path: `internal/static/handler_test.go`:
    - Tests `/_app/immutable/test.js` sets `Cache-Control: public, max-age=31536000, immutable`.
    - Tests `/_app/immutable/missing.js` returns 404 with `Cache-Control: no-store` and never serves fallback HTML.
    - Tests `/_app/version.json` returns `Cache-Control: no-cache`.
    - Tests `GET /` serves `prerendered/index.html`.
    - Tests `GET /items/123` serves `prerendered/index.html` (SPA fallback).
    - Tests `Accept-Encoding: br` returns `.br` file with `Content-Encoding: br`.
    - Tests `If-None-Match` returns 304.
  - Path: `internal/proxy/handler_test.go`:
    - Tests HTTP request forwarding to test backend.
    - Tests WebSocket upgrade request forwarding.
  - Run: `go test -v ./...` to verify all unit tests pass.

---

### Phase 3: Example CLI & Dev Tooling Configuration

#### Tasks & Files

- **Task 3.1: Implement Example Command Entrypoint**
  - Path: `example/cmd/main.go`
  - Implementation details:
    - Parses command-line flags:
      - `--listen`: address to listen on (default `:8080`).
      - `--proxy`: optional URL of Vite dev server (e.g. `http://127.0.0.1:5173`).
    - Dispatch logic:
      ```go
      package main

      import (
          "flag"
          "fmt"
          "log"
          "net/http"
          "os"
          "os/signal"
          "syscall"

          "github.com/tylergannon/skgo"
          "github.com/tylergannon/skgo/example/web"
      )

      func main() {
          listenAddr := flag.String("listen", ":8080", "Address to listen on")
          proxyURL := flag.String("proxy", "", "Vite dev server URL to proxy to (dev mode)")
          flag.Parse()

          var handler http.Handler
          var err error

          if *proxyURL != "" {
              log.Printf("Starting in DEV mode, proxying to %s", *proxyURL)
              handler, err = skgo.NewDevProxy(*proxyURL)
              if err != nil {
                  log.Fatalf("Failed to create dev proxy: %v", err)
              }
          } else {
              log.Printf("Starting in PROD mode, serving embedded assets")
              handler, err = skgo.NewStaticServer(web.DistFS, skgo.StaticOptions{})
              if err != nil {
                  log.Fatalf("Failed to create static server: %v", err)
              }
          }

          server := &http.Server{
              Addr:    *listenAddr,
              Handler: handler,
          }

          go func() {
              log.Printf("skgo server listening on http://127.0.0.1%s", *listenAddr)
              if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
                  log.Fatalf("HTTP server failed: %v", err)
              }
          }()

          quit := make(chan os.Signal, 1)
          signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
          <-quit
          log.Println("Shutting down server...")
      }
      ```

- **Task 3.2: Configure Air for Live Rebuilds**
  - Path: `example/.air.toml` (content defined in Architecture section).
  - Verify Air triggers rebuild on modifying `example/cmd/main.go` or `skgo/*.go`.

- **Task 3.3: Configure Overmind for Unified Process Management**
  - Path: `example/Procfile`
    ```procfile
    web: cd web && mise x -- vp dev --host 127.0.0.1 --port 5173
    server: air -c .air.toml
    ```
  - Verify Overmind starts both processes cleanly with `overmind start`.

---

### Phase 4: Playwright-BDD E2E Acceptance Suite

#### Tasks & Files

- **Task 4.1: Configure Playwright-BDD Test Package**
  - Path: `example/e2e/package.json`
    ```json
    {
      "name": "example-e2e",
      "version": "0.0.1",
      "private": true,
      "type": "module",
      "scripts": {
        "bddgen": "bddgen",
        "test": "bddgen && playwright test"
      },
      "devDependencies": {
        "@playwright/test": "1.62.1",
        "playwright-bdd": "9.2.0",
        "typescript": "6.0.3"
      }
    }
    ```
  - Path: `example/e2e/playwright.config.ts`
    ```typescript
    import { defineConfig, devices } from '@playwright/test';
    import { defineBddConfig } from 'playwright-bdd';

    const testDir = defineBddConfig({
      features: 'features/*.feature',
      steps: 'steps/*.steps.ts',
    });

    const baseURL = process.env.BASE_URL ?? 'http://127.0.0.1:8080';

    export default defineConfig({
      testDir,
      reporter: 'list',
      fullyParallel: false,
      workers: 1,
      retries: 0,
      use: {
        baseURL,
        trace: 'retain-on-failure',
      },
      projects: [
        {
          name: 'chromium',
          use: { ...devices['Desktop Chrome'] },
        },
      ],
    });
    ```
  - Path: `example/e2e/tsconfig.json`
    ```json
    {
      "compilerOptions": {
        "target": "ESNext",
        "module": "NodeNext",
        "moduleResolution": "NodeNext",
        "strict": true
      },
      "include": ["steps/**/*", "playwright.config.ts"]
    }
    ```

- **Task 4.2: Author Gherkin Acceptance Features**
  - Path: `example/e2e/features/rendering.feature`:
    ```gherkin
    Feature: Svelte Component Rendering
      Scenario: Home page renders through Go server with interactive components
        Given I visit "/"
        Then the page title should be "skgo Home"
        And the greeting should display "SvelteKit running through Go"
        When I click the counter button
        Then the counter button text should be "Count: 1"
    ```
  - Path: `example/e2e/features/navigation.feature`:
    ```gherkin
    Feature: Client-Side Navigation
      Scenario: Navigating between routes does not cause a document reload
        Given I visit "/"
        And the application is hydrated
        When I record network requests
        And I click the navigation link to "/about"
        Then the about page title should be "About skgo"
        And 0 document reloads should have occurred
    ```
  - Path: `example/e2e/features/deep_link.feature`:
    ```gherkin
    Feature: Dynamic Deep Linking
      Scenario: Direct navigation to a dynamic route loads the SPA shell and client route
        Given I visit "/items/42"
        Then the item detail title should be "Item Detail"
        And the item ID should display "ID: 42"
    ```
  - Path: `example/e2e/features/not_found.feature`:
    ```gherkin
    Feature: Not Found Routing
      Scenario: Navigating to an unknown route shows the SvelteKit error page
        Given I visit "/this-path-does-not-exist"
        Then the error status should be "404"
        And the error message should display "Not Found"
    ```

- **Task 4.3: Implement Step Definitions**
  - Path: `example/e2e/steps/app.steps.ts`:
    ```typescript
    import { expect } from '@playwright/test';
    import { createBdd } from 'playwright-bdd';

    const { Given, When, Then } = createBdd();

    let documentLoadCount = 0;

    Given('I visit {string}', async ({ page }, path: string) => {
      await page.goto(path);
    });

    Then('the page title should be {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="page-title"]')).toHaveText(text);
    });

    Then('the greeting should display {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="greeting"]')).toHaveText(text);
    });

    When('I click the counter button', async ({ page }) => {
      await page.locator('[data-testid="counter-btn"]').click();
    });

    Then('the counter button text should be {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="counter-btn"]')).toHaveText(text);
    });

    Given('the application is hydrated', async ({ page }) => {
      await expect(page.locator('[data-testid="hydrated"]')).toHaveAttribute('data-status', 'yes');
    });

    When('I record network requests', async ({ page }) => {
      documentLoadCount = 0;
      page.on('request', (req) => {
        if (req.resourceType() === 'document') {
          documentLoadCount++;
        }
      });
    });

    When('I click the navigation link to {string}', async ({ page }, path: string) => {
      await page.click(`a[href="${path}"]`);
    });

    Then('the about page title should be {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="about-title"]')).toHaveText(text);
    });

    Then('{int} document reloads should have occurred', async ({}, count: number) => {
      expect(documentLoadCount).toBe(count);
    });

    Then('the item detail title should be {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="item-title"]')).toHaveText(text);
    });

    Then('the item ID should display {string}', async ({ page }, text: string) => {
      await expect(page.locator('[data-testid="item-id"]')).toHaveText(text);
    });

    Then('the error status should be {string}', async ({ page }, status: string) => {
      await expect(page.locator('[data-testid="error-status"]')).toHaveText(status);
    });

    Then('the error message should display {string}', async ({ page }, msg: string) => {
      await expect(page.locator('[data-testid="error-message"]')).toHaveText(msg);
    });
    ```

- **Task 4.4: Execute Verification in Both Modes**
  1. **Dev Mode Verification**:
     - Launch Overmind: `cd example && overmind start`
     - Run tests: `cd example/e2e && BASE_URL=http://127.0.0.1:8080 mise x -- pnpm test`
     - Verify all 4 scenarios pass.
  2. **Production Mode Verification**:
     - Build web app: `cd example/web && mise x -- vp build`
     - Build Go binary: `cd example && go build -o tmp/example-server ./cmd`
     - Start binary: `./tmp/example-server --listen :8080` (with Vite stopped)
     - Run tests: `cd example/e2e && BASE_URL=http://127.0.0.1:8080 mise x -- pnpm test`
     - Verify all 4 scenarios pass.

---

## Files Summary

| File Path | Component | Purpose |
|---|---|---|
| `go.mod` | Root Go Module | Defines `github.com/tylergannon/skgo` module |
| `proxy.go` | `skgo` Public API | Exposes `NewDevProxy(targetURL)` |
| `static.go` | `skgo` Public API | Exposes `NewStaticServer(distFS, opts)` |
| `server.go` | `skgo` Public API | Exposes high-level server types and configuration |
| `internal/proxy/handler.go` | Proxy Core | `httputil.ReverseProxy` with WebSocket upgrade tunneling |
| `internal/proxy/handler_test.go` | Proxy Tests | Unit tests for HTTP reverse proxy and WebSocket upgrade |
| `internal/static/handler.go` | Static Core | Static file server from `fs.FS` with sirv caching & SPA fallback |
| `internal/static/handler_test.go` | Static Tests | Table tests for immutable caching, ETags, compression, fallback |
| `internal/static/mrmime.go` | Static Core | Embedded MIME registry matching mrmime 2.0.1 |
| `example/go.mod` | Example Module | Defines `github.com/tylergannon/skgo/example` with local replace |
| `example/Procfile` | Dev Tooling | Overmind process manager definitions for `web` and `server` |
| `example/.air.toml` | Dev Tooling | Air live reload configuration for rebuilding Go binary |
| `example/cmd/main.go` | Example CLI | Application entrypoint with `--listen` and `--proxy` flags |
| `example/web/mise.toml` | Toolchain | Pins Node 24.16.0 and Vite+ 0.3.0 |
| `example/web/package.json` | Toolchain | Pinned npm dependencies, `#lib` imports map, `devEngines` |
| `example/web/pnpm-workspace.yaml` | Toolchain | pnpm workspace settings |
| `example/web/tsconfig.json` | Toolchain | TypeScript configuration extending `$app/tsconfig` |
| `example/web/vite.config.ts` | Vite Config | Kit 3 flat config with adapter-node, `paths.origin`, and `paths.relative: false` |
| `example/web/dist.go` | Embed Package | `package web` declaring `//go:embed all:build/client all:build/prerendered` |
| `example/web/build/client/.gitkeep` | Embed Safety | Checked-in placeholder allowing clean compile before `vp build` |
| `example/web/build/prerendered/.gitkeep` | Embed Safety | Checked-in placeholder allowing clean compile before `vp build` |
| `example/web/src/app.html` | SvelteKit Core | Base HTML document template |
| `example/web/src/routes/+layout.ts` | Route Config | Sets `export const ssr = false; export const prerender = true;` |
| `example/web/src/routes/+layout.svelte` | App Layout | App navigation bar and hydration tracking element |
| `example/web/src/routes/+page.svelte` | Home Route | Interactive Svelte 5 counter component |
| `example/web/src/routes/about/+page.svelte` | About Route | Static content page for client-side navigation test |
| `example/web/src/routes/items/[id]/+page.ts` | Dynamic Route Config | Sets `export const prerender = false;` |
| `example/web/src/routes/items/[id]/+page.svelte` | Dynamic Route | Renders route parameter `id` client-side |
| `example/web/src/routes/+error.svelte` | Error Route | SvelteKit client-side error page for 404 display |
| `example/e2e/package.json` | Test Suite | Playwright and Playwright-BDD test runner dependencies |
| `example/e2e/playwright.config.ts` | Test Suite | Playwright configuration targeting `BASE_URL` |
| `example/e2e/tsconfig.json` | Test Suite | TypeScript configuration for step definitions |
| `example/e2e/features/*.feature` | Test Specs | 4 Gherkin features (rendering, navigation, deep link, 404) |
| `example/e2e/steps/app.steps.ts` | Test Steps | Step definition implementation for acceptance features |

---

## Definition of Done

1. **Clean Code & Verification**:
   - `go test -v ./...` passes at root `skgo/` and in `example/`.
   - `go vet ./...` passes with zero warnings.
2. **Build Cleanliness**:
   - `cd example/web && mise x -- vp build` builds without errors, outputting `build/client` and `build/prerendered`.
   - `cd example && go build ./cmd` succeeds on a fresh checkout (using the checked-in `.gitkeep` placeholders) as well as after `vp build`.
3. **Development Experience**:
   - Running `overmind start` in `example/` starts both Vite+ and Air cleanly.
   - Visiting `http://127.0.0.1:8080` displays the home page.
   - Modifying `example/web/src/routes/+page.svelte` hot-updates the browser via WebSocket HMR without a full page reload.
   - Modifying `example/cmd/main.go` triggers Air to rebuild and restart the Go server.
4. **Standalone Production Binary**:
   - Executing the built Go binary directly (`./example-server --listen :8080`) with Vite and Node terminated completely serves the SvelteKit app from `embed.FS` to a browser.
   - Static assets (`/_app/immutable/**`) are served with `Cache-Control: public, max-age=31536000, immutable`.
   - Missing immutable assets return `404 Not Found` with `Cache-Control: no-store` and never serve the fallback shell.
   - Direct navigation to `/items/42` returns the SPA fallback shell and renders the dynamic route client-side.
5. **Acceptance Test Suite**:
   - Running `BASE_URL=http://127.0.0.1:8080 mise x -- pnpm test` in `example/e2e` passes all 4 feature scenarios against dev mode (Go + Overmind).
   - Running `BASE_URL=http://127.0.0.1:8080 mise x -- pnpm test` in `example/e2e` passes all 4 feature scenarios against prod mode (bare Go binary).
6. **Zero Sidecars in Production**:
   - No Node.js process is spawned by Go; no JavaScript runtime is required for production execution.

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|---|---|---|
| **Go `embed.FS` compilation failure on fresh checkout** | High (build failure) | Check in placeholder `.gitkeep` files under `example/web/build/client/` and `example/web/build/prerendered/`. If assets are unbuilt at runtime, log an explicit warning instructing the user to run `mise x -- vp build`. |
| **Kit 3 `paths.origin` mismatch causing 403 on requests** | High (broken functionality) | Ensure `vite.config.ts` sets `paths.origin` to `process.env.ORIGIN ?? 'http://127.0.0.1:8080'`. The Go server listens at `:8080` by default so origins match across dev and prod. |
| **Vite HMR WebSocket connection dropped by proxy** | High (broken dev HMR) | Rely on `httputil.ReverseProxy`'s native HTTP/1.1 connection hijacking. Test WebSocket tunneling explicitly in `internal/proxy/handler_test.go`. |
| **Vite 6+ host check (`server.allowedHosts`) rejects proxy** | Medium (500 in dev) | In `internal/proxy/handler.go`, rewrite `req.Host = target.Host` (`127.0.0.1:5173`) so Vite treats incoming requests as local requests. |
| **Prerendered SPA shell using relative asset paths fails on deep links** | High (broken deep links) | In `vite.config.ts`, explicitly configure `paths: { relative: false }` so all asset URLs in `prerendered/index.html` are absolute paths (`/_app/immutable/...`). |
| **TypeScript 7 incompatibilities** | High (build failure) | Pin `typescript: "6.0.3"` strictly in `package.json` devDependencies. Kit 3 relies on `ts.sys`, which was removed in TS 7. |

---

## Dependencies

### External CLI Tools (Installed via mise/system)
- **Node.js**: `24.16.0` (managed via `example/web/mise.toml`)
- **Vite+ (`vp`)**: `0.3.0` (managed via `example/web/mise.toml`)
- **pnpm**: `11.25.0` (auto-downloaded via Node `devEngines` in `package.json`)
- **Go**: `1.24.0` (or system Go ≥ 1.22)
- **Overmind**: Process supervisor for Procfile
- **Air**: `v1.52.0+` (Go live reloader)

### Web Package Dependencies (`example/web/package.json`)
- `@sveltejs/kit`: `3.0.0-next.25`
- `@sveltejs/adapter-node`: `6.0.0-next.10`
- `svelte`: `5.56.10`
- `vite`: `8.2.2`
- `@sveltejs/vite-plugin-svelte`: `7.3.0`
- `typescript`: `6.0.3`
- `@types/node`: `26.4.1`

### E2E Test Dependencies (`example/e2e/package.json`)
- `@playwright/test`: `1.62.1`
- `playwright-bdd`: `9.2.0`
- `typescript`: `6.0.3`

### Go Standard Library Dependencies
- `net/http`
- `net/http/httputil`
- `io/fs`
- `embed`
- `net/url`
- Zero external third-party Go dependencies for the core `skgo` library in Sprint 001.

---

## Open Questions

1. **Pre-build Hook for `go build`**:
   - *Question*: Should `go:generate` in `example/cmd/` invoke `mise x -- vp build` automatically, or should build orchestration remain explicit in external task runners/Makefiles?
   - *Recommendation*: Keep it explicit for Sprint 001. An explicit `mise x -- vp build` followed by `go build` avoids hidden dependencies during ordinary `go test ./...` runs.
2. **Dynamic 404 Status vs SPA 200 Shell**:
   - *Question*: In pure CSR mode without SSR, when a user deep-links to `/non-existent`, should the Go server return HTTP 404 with the shell HTML, or HTTP 200 with the shell HTML?
   - *Analysis*: In SvelteKit, if the server returns HTTP 200 with the boot shell, the client-side router matches the route against its client manifest, fails to match, and renders `+error.svelte` with status 404. If Go returns HTTP 404 with the shell body, the client router also renders `+error.svelte` with 404. However, knowing which routes exist without parsing the route manifest requires Go to either parse the route tree or treat all non-asset requests as potential client routes.
   - *Recommendation*: For Sprint 001, Go serves the SPA shell with HTTP 200 for all non-asset paths that accept HTML. SvelteKit's client router handles 404 route rendering client-side. Route manifest parsing in Go will be introduced in Phase A step 5 / Sprint 002.
3. **Precompression Build Step**:
   - *Question*: Does `@sveltejs/adapter-node` with `precompress: true` compress both `client` and `prerendered` files reliably?
   - *Recommendation*: Yes. `builder.compress` in Kit's adapt phase writes `.br` and `.gz` siblings for `.html`, `.js`, `.css`, and `.json`. `internal/static` negotiates these siblings if present on disk.
