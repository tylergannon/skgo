---
title: "Deploying"
summary: "Run the binary behind its public origin, a proxy or a container, and shut it down cleanly."
---

A production SKGo app is one Go executable. It embeds the client bundle, the prerendered files and the server-rendering bundle, and it renders pages in pooled goja runtimes, so the host needs no Node and no files beside the binary. Nothing in SKGo uses cgo, so `CGO_ENABLED=0 go build` produces a static binary that runs on a minimal image.

This is the place of Kit's [Node adapter](https://svelte.dev/docs/kit/adapter-node). Its `node build` server and its environment variables have no equivalent here; this page covers what does.

## Listening

The generated `cmd/main.go` listens on the host and port of the compiled origin. Its `-listen` flag overrides that without touching the origin, which is what a container wants:

```sh
./myapp -listen 0.0.0.0:8080
```

## The origin

SKGo never derives the public origin from a request. The origin `NewHandler` receives decides:

- which form submissions count as same-origin. A page action or a form-shaped `server.go` request from another origin is a 403 unless Kit's `csrf.trustedOrigins` lists it; a non-`GET` remote call from another origin is a 403, with no exceptions, as in Kit.
- the scheme and host of the `URL` that loads, endpoints and hooks see.
- whether cookies default to `Secure`. They do, unless the origin is `http://localhost`.

Kit compiled the same value into the frontend through [`paths.origin`](building-your-app#The-origin-is-compiled-in) and used it while prerendering, so when the origin changes, rebuild rather than overriding `-origin` on an old build.

## Behind a proxy

Terminate TLS at your proxy or platform and forward plain HTTP to the `-listen` address. Because the URL comes from the compiled origin, the proxy does not need to forward `Host` or protocol headers.

`event.ClientAddress()` is the peer address of the connection, which behind a proxy is the proxy. SKGo reads no forwarding header on its own. Set `HandleConfig.ClientAddress` in `server.go` to read the one your proxy sets, and only when every request arrives through that proxy:

```go
handleCfg.ClientAddress = func(r *http.Request) (string, error) {
	if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
		return ip, nil
	}
	return "", errors.New("no Fly-Client-IP header")
}
```

The first value or error is shared by the request's event and every `event.Fetch` it makes.

## Request limits

SKGo caps the body of a remote `form` function at 32 MiB and nothing else. Page actions, commands, queries and `server.go` routes read the request themselves, so limit them where you read them, with `http.MaxBytesReader` or `ParseMultipartForm`'s memory argument, or wrap the whole app in `http.MaxBytesHandler` in `main`. Timeouts are fields of `http.Server`; see [Shutdown](#Shutdown).

## Static files

Client files and prerendered pages are answered before the `handle` hook, as Kit's own adapters answer them, in the order [`NewStaticHandler`](skgo-server#NewStaticHandler) lists. Files under `_app/immutable/` are sent with `Cache-Control: public, max-age=31536000, immutable`. With the adapter's `precompress` option on, which is the default, the server sends the `.br` or `.gz` copy the request's `Accept-Encoding` accepts, with `Vary: Accept-Encoding` and the uncompressed file's content type. The other trailing-slash form of a prerendered page is a 308 to the right one, as in Kit.

## Containers

For a project laid out by `skgo new`, build the frontend in an image that has both Node and Go, because the adapter compiles the [prerender service](prerendering#How-prerendering-runs) during `vp build`. Then compile the binary and copy it alone into a distroless image:

```dockerfile
/// file: Dockerfile
FROM golang:1.27.2-bookworm AS toolchain

FROM node:22.17-bookworm AS web
COPY --from=toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}"
ENV VP_HOME=/opt/vite-plus
ENV PATH="/opt/vite-plus/bin:${PATH}"
RUN curl -fsSL https://vite.plus -o /tmp/install-vp.sh
RUN VP_VERSION=1.3.0 VP_NODE_MANAGER=no VP_PM_MANAGER=yes CI=true bash /tmp/install-vp.sh
ARG ORIGIN
WORKDIR /src
COPY . .
RUN cd web && vp install --frozen-lockfile
RUN go generate ./... && go tool skgo kit-patch --web web --check
RUN cd web && ORIGIN="$ORIGIN" vp build

FROM golang:1.27.2-bookworm AS build
ARG ORIGIN
COPY --from=web /src /src
WORKDIR /src
RUN CGO_ENABLED=0 go build -ldflags "-X main.origin=$ORIGIN" -o /myapp ./cmd

FROM gcr.io/distroless/static-debian12
COPY --from=build /myapp /myapp
EXPOSE 8080
ENTRYPOINT ["/myapp", "-listen", "0.0.0.0:8080"]
```

[`kit-patch --check`](/docs/cli/kit-patch) fails the image when the installed Kit lacks SKGo's prerender queue correction, before a build with declared prerender inputs would. The origin is a build argument, not a runtime variable, so on Fly.io it goes under `[build.args]`:

```toml
/// file: fly.toml
[build.args]
  ORIGIN = "https://myapp.fly.dev"

[http_service]
  internal_port = 8080
  force_https = true
```

[Environment variables](environment-variables) that bind at startup go under `[env]`; a required one that is unset stops the binary as it starts, naming the variable.

## Shutdown

The generated `main` installs no signal handler, so `SIGTERM` ends the process with requests in flight. To drain them, serve through an `http.Server` and wait for `Shutdown` before `main` returns:

```go
srv := &http.Server{Addr: *listen, Handler: handler}
ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
defer stop()

go func() {
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}()

<-ctx.Done()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
if err := srv.Shutdown(shutdownCtx); err != nil {
	log.Print(err)
}
```

`Shutdown` returns once every request has finished or the deadline passes. Keep the deadline inside your platform's kill timeout.

## Version skew

The client bundle is inside the binary, so a deployment replaces the server and its assets together, and the new binary serves only its own. A tab left open on the previous deployment gets a 404 for a chunk that no longer exists. Kit's answer is its [`version`](https://svelte.dev/docs/kit/configuration#version) option: the manifest records `version.name`, SKGo sends it as `x-sveltekit-version` on data and remote responses as Kit's server does, and Kit's client falls back to a full-page navigation when it sees a new one. The default name is the build time, so every build is distinct without configuration.
