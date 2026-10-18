---
title: "skgo: server"
summary: "SSR, the static handler, the build manifest, transport and the prerender service."
---

The pieces `skgo new` composes into `server.go` and `internal/serverhooks`.
`NewSSR` runs Kit's server-side rendering in a pool of goja runtimes over the
adapter's build output; `NewStaticHandler` serves the client assets and
prerendered pages and hands everything else to SSR; `ReadManifest` reads the
description of routes, pages and assets the adapter wrote. `Transport`
declares the domain types that cross between Go and the browser with their
own encoding. `RunPrerenderService` is the Go side of the build-time
prerender.

## SSR

```go
type SSR struct {
	// contains filtered or unexported fields
}
```

SSR renders page documents in this process.

It owns nothing the rest of skgo does not already own: the loads it renders with are the same ones that answer `__data.json`, and the remote functions it answers during a render are the same ones that answer `/_app/remote/...`. A page rendered here and the same page rendered by kit's client from those two endpoints are the same page by construction.

### ErrorTemplate

```go
func (s *SSR) ErrorTemplate() string
```

ErrorTemplate returns the Kit error.html used for fatal errors before page dispatch.

## NewSSR

```go
func NewSSR(build fs.FS, m Manifest, loads *Loads, remotes *Remotes, opts SSROptions) (*SSR, error)
```

NewSSR builds a renderer over an adapter build. It fails if the build carries no SSR bundle, if the bundle does not parse, or if it does not come up.

## SSROptions

```go
type SSROptions struct {
	// Environment is the Go-validated startup snapshot shared by Go and Kit.
	Environment *EnvironmentSnapshot
	// Actions are the Go handlers generated for classic page form actions.
	Actions *Actions
	// Runtimes bounds how many pages may render at once. Zero means one per
	// CPU. Each runtime holds its own copy of the app's module state, so this
	// is a memory-for-throughput dial and nothing else.
	Runtimes int
	// OnError is told about a failure the visitor will not see, with the route
	// it was for: a render that threw, a load whose value could not be encoded,
	// an error that reached the root layout. The visitor gets kit's own answer
	// to it — the error page, or `error.html` — which says nothing about the
	// cause, so this is the only record. Leaving it nil logs.
	OnError func(routeID string, err error)
	// Fetch answers a render-time `event.fetch` of the app's own routes,
	// dispatched in-process rather than over a socket. Pass the handler the
	// app serves with, from Handle inwards, so a fetch reaches every page,
	// `__data.json`, remote function, server route, asset and prerendered file
	// as a request would. It is only known once the page handler exists, so
	// hand it over by reference. A page it renders runs on a runtime of its
	// own: the engine keeps a separate bounded pool for each level of nesting.
	// Leaving it nil refuses every render-time fetch.
	//
	// The Go dispatcher decides after HandleFetch runs: this handler only ever
	// sees requests for this app, and any other URL goes out over HTTP with
	// Kit's rules for which credentials leave with it.
	Fetch http.Handler
	// HandleFetch intercepts render-time fetches before credential inheritance.
	// It may return a response or call next with a rewritten request.
	HandleFetch HandleFetch
	// FilterSerializedResponseHeaders selects response headers that universal
	// loads may read and that hydration receives. The default includes none.
	FilterSerializedResponseHeaders func(name, value string) bool
}
```

SSROptions configures the renderer.

## NewStaticHandler

```go
func NewStaticHandler(build fs.FS, options ...StaticOption) (http.Handler, error)
```

NewStaticHandler serves an embedded skgo adapter build. build must be rooted at the adapter's output directory, so that `index.html`, `skgo.manifest.json` and `client/` are at its top level.

Requests are answered in this order: an exact file under `client/`; a 404 with an empty body for any other miss below the app directory; a page the build prerendered; a page rendered by the renderer, if one was given and the route's branch does not turn SSR off; kit's boot document with status 200 for a path matching a route from the manifest; and the boot document with status 404 for anything else, so kit's client router can render `+error.svelte`.

## StaticOption

```go
type StaticOption func(*staticHandler)
```

A StaticOption configures a static handler.

### WithSSR

```go
func WithSSR(s *SSR) StaticOption
```

WithSSR makes the handler render page documents with s rather than answer them with kit's SPA shell. The shell is still what answers a page whose branch turns SSR off.

## ServedAsFile

```go
func ServedAsFile(pages http.Handler) func(*http.Request) bool
```

ServedAsFile returns the predicate HandleConfig.Static wants for pages: a request the handler answers from a file of the build (a client asset, a prerendered page). Kit's adapters answer those before `handle` runs, so the hook must not see them. It returns nil for a handler that is neither.

## Manifest

```go
type Manifest struct {
	// TracingServer is the resolved SvelteKit tracing.server build setting.
	TracingServer bool `json:"tracingServer,omitempty"`

	// Development identifies a Vite development snapshot. Kit executes
	// prerender remotes on demand in development instead of reading artifacts.
	Development bool `json:"development,omitempty"`

	// Skgo is the version of skgo whose adapter wrote this build, as the
	// adapter stamped it: a released version, or "devel" for an adapter
	// written by a skgo built from a checkout.
	Skgo string `json:"skgo"`
	// SkgoAdapter fingerprints the adapter itself, which is the half of the
	// identity that is always meaningful — two checkouts both call themselves
	// devel. ReadManifest refuses a build whose adapter is not the one this
	// module carries, because the adapter and the Go that reads its output are
	// one contract written in two languages.
	SkgoAdapter string `json:"skgoAdapter"`
	// AppDir is kit's `appDir` — the directory holding immutable assets,
	// normally `_app`.
	AppDir string `json:"appDir"`
	// Base is kit's `paths.base`, without a trailing slash.
	Base string `json:"base"`
	// Version is kit's build version name.
	Version string `json:"version"`
	// TrustedOrigins is Kit's csrf.trustedOrigins, applied to form-shaped
	// page actions and endpoints in production.
	TrustedOrigins []string `json:"trustedOrigins,omitempty"`
	// Nodes gives, per kit node index, the vite-root-relative path of that
	// node's `+page.server.ts` or `+layout.server.ts`, or "" when it has
	// none. It is the key a Go load is registered under.
	Nodes []string `json:"nodes"`
	// Loads names the modules with a load export; an action-only page still
	// appears in Nodes, but has no load handler to register.
	Loads []string `json:"loads"`
	// Actions names page server modules that export Go-backed actions, including
	// action-only pages with no load export.
	Actions []string `json:"actions,omitempty"`
	// Routes lists every route kit knows about, with the regular expression
	// kit's own router uses to match it.
	Routes []ManifestRoute `json:"routes"`
	// PrerenderedEndpoints retains the compiled methods of endpoint routes Kit
	// removed from Routes after prerendering. It checks binary/build agreement;
	// these routes are served only at the paths recorded in Prerendered.
	PrerenderedEndpoints map[string][]string `json:"prerenderedEndpoints,omitempty"`
	// Remotes lists the `<hash>/<name>` ids of every remote function the
	// built client calls. The adapter copies it from the list `skgo generate`
	// wrote and refuses to build if kit compiled a different set, so a
	// mismatch here means the Go binary and the frontend were generated from
	// different sources.
	Remotes []string `json:"remotes"`
	// Prerendered lists the pathnames the build wrote a file for, exactly as
	// kit records them in `builder.prerendered.paths`: percent-decoded, and
	// carrying the configured base. It is the authority for two things kit's
	// own adapters use it for — whether a request is answered from the
	// prerendered tree at all, and whether the other trailing-slash form of
	// that request should be redirected to (`adapter-node/src/handler.js`,
	// `serve_prerendered`).
	//
	// A route kit prerenders is *removed* from Routes: `generateManifest`
	// filters out every route whose prerender option is `true`
	// (`core/adapt/builder.js`). So a prerendered path that is not served from
	// here is not served at all.
	Prerendered []string `json:"prerendered,omitempty"`
	// Precompressed reports that the build wrote `.br` and `.gz` siblings for
	// the files kit's own `builder.compress` compresses.
	Precompressed bool `json:"precompressed,omitempty"`
	// PrerenderedFiles maps Kit's logical paths below prerendered/ to safe
	// build-relative paths when the native logical filename cannot be embedded
	// directly (currently, query-bearing filenames).
	PrerenderedFiles map[string]string `json:"prerenderedFiles,omitempty"`
	// SSR describes the server-rendering half of the build: the bundle the Go
	// process renders pages with, the document template, and everything kit's
	// own `render_response` reads out of its manifest to assemble a document.
	// It is absent from a build made by an adapter that emitted no bundle, and
	// such a build is served the way it always was, as the SPA shell.
	SSR *ManifestSSR `json:"ssr,omitempty"`
}
```

Manifest is the build description written by the skgo SvelteKit adapter to `skgo.manifest.json`. Only the fields the server needs are modelled.

### ReadManifest

```go
func ReadManifest(build fs.FS) (Manifest, error)
```

ReadManifest parses `skgo.manifest.json` from an adapter build.

### EndpointConfig

```go
func (m Manifest) EndpointConfig(origin string) EndpointConfig
```

EndpointConfig derives an endpoint-registry configuration from a build manifest.

### HandleConfig

```go
func (m Manifest) HandleConfig() HandleConfig
```

HandleConfig derives a Handle's configuration from a build manifest.

### LoadConfig

```go
func (m Manifest) LoadConfig(origin string) LoadConfig
```

LoadConfig derives a load-registry configuration from a build manifest.

### RemoteConfig

```go
func (m Manifest) RemoteConfig(origin string) RemoteConfig
```

RemoteConfig derives a registry configuration from a build manifest.

## ManifestCSP

```go
type ManifestCSP struct {
	// Mode is "auto", "hash" or "nonce". kit's default is "auto"
	// (`list(['auto', 'hash', 'nonce'])`, whose fallback is the first option).
	Mode string `json:"mode"`
	// Directives is the policy the response's `content-security-policy`
	// header (and any nonce/hash skgo computes for its own boot script) is
	// built from.
	Directives map[string]CSPDirectiveValue `json:"directives"`
	// ReportOnly is the same shape, for `content-security-policy-report-only`.
	// kit requires `report-to` or `report-uri` here whenever any other
	// directive is set (`CspReportOnlyProvider`'s constructor,
	// `runtime/server/page/csp.js`) — NewSSR checks this once, at startup,
	// rather than on every render the way kit's own per-request `new Csp(...)`
	// does, because the config cannot change between one render and the next.
	ReportOnly map[string]CSPDirectiveValue `json:"reportOnly"`
}
```

ManifestCSP is the build's `csp` option, exactly as kit validates it (`core/config/options.js`: `csp: object({ mode: list(['auto', 'hash', 'nonce']), directives, reportOnly: directives })`). The adapter writes `builder.config.csp` into the manifest verbatim, so a directive kit's schema left `undefined` (never configured) is absent here too — `JSON.stringify` drops it — while one set to `[]` still comes across, and still forces the directive into the header with no sources, because an empty array is truthy in JS.

## ManifestClient

```go
type ManifestClient struct {
	Start                string         `json:"start"`
	App                  string         `json:"app"`
	Imports              []string       `json:"imports"`
	Stylesheets          []string       `json:"stylesheets"`
	Fonts                []ManifestFont `json:"fonts"`
	UsesEnvDynamicPublic bool           `json:"usesEnvDynamicPublic"`
}
```

ManifestClient is kit's own `manifest._.client`: the entry points a document imports and the assets it must link.

## ManifestEndpoint

```go
type ManifestEndpoint struct {
	// Methods are the HTTP methods the compiled `+server.ts` exports a handler
	// for, plus "*" when it exports a `fallback`. It is kit's own
	// `builder.routes[].api.methods`, computed from the built module's exports
	// (`core/postbuild/analyse.js`, `analyse_endpoint`), so it describes the
	// frontend that shipped rather than the source it came from.
	Methods []string `json:"methods"`
}
```

ManifestEndpoint is a route's server route, as kit's build describes it.

## ManifestFont

```go
type ManifestFont struct {
	File     string `json:"file"`
	Filename string `json:"filename"`
}
```

ManifestFont is one font the build asks a document to preload.

## ManifestPage

```go
type ManifestPage struct {
	// Layouts holds the node index of each layout wrapping the page. -1 marks
	// a slot no layout fills, which JSON cannot express as a hole.
	Layouts []int `json:"layouts"`
	// Errors holds the node index of the `+error.svelte` declared at each
	// layout's depth, positionally aligned with Layouts, and -1 where a depth
	// declares none. It is what decides which error page a failure renders and
	// how many layouts survive with it: kit walks it outward from the node
	// that failed (`runtime/error-chain.js`, `nearest_error_pages`) and the
	// first depth that declares one wins, with every layout above that depth
	// still rendered around it.
	Errors []int `json:"errors,omitempty"`
	// Leaf is the node index of the page itself.
	Leaf int `json:"leaf"`
}
```

ManifestPage is a page route's node branch. `append(Layouts, Leaf)` is the branch itself: one slot per node, outermost first, and the order kit's client positions its `x-sveltekit-invalidated` string over.

### Branch

```go
func (p *ManifestPage) Branch() []int
```

Branch is `[...layouts, leaf]`: the nodes of this route, outermost first.

### ErrorPages

```go
func (p *ManifestPage) ErrorPages() []int
```

ErrorPages is Errors, and nil for a route that has no page at all.

## ManifestParam

```go
type ManifestParam struct {
	Name     string `json:"name"`
	Optional bool   `json:"optional"`
	Rest     bool   `json:"rest"`
	Chained  bool   `json:"chained"`
	Matcher  string `json:"matcher,omitempty"`
}
```

ManifestParam is one route parameter, as kit's own router describes it.

## ManifestRoute

```go
type ManifestRoute struct {
	// TrailingSlash is Kit's resolved route policy: never, always or ignore.
	// Empty means never, Kit's default. Page policies include layout inheritance.
	TrailingSlash string `json:"trailingSlash,omitempty"`
	ID            string `json:"id"`
	Pattern       string `json:"pattern"`
	// Params names the route's parameters in the order the pattern captures
	// them.
	Params []ManifestParam `json:"params,omitempty"`
	// Page describes the node branch of a route that has a page. It is nil for
	// a route that is an endpoint and nothing else.
	Page *ManifestPage `json:"page,omitempty"`
	// Endpoint describes the route's `+server.ts`, and is nil for a route that
	// has none. Kit's own route table carries the same distinction — a lazy
	// loader for the compiled module, or null (`core/generate_manifest`) — and
	// a route with neither a page nor an endpoint never reaches the table at
	// all.
	Endpoint *ManifestEndpoint `json:"endpoint,omitempty"`
}
```

ManifestRoute is one entry of Manifest.Routes.

## ManifestSSR

```go
type ManifestSSR struct {
	// Bundle is the path of the SSR bundle inside the build.
	Bundle string `json:"bundle"`
	// Template is the path of `app.html` inside the build, verbatim, with
	// kit's `%sveltekit.*%` placeholders still in it.
	Template string `json:"template"`
	// ErrorTemplate is the path of kit's `error.html` inside the build: the
	// document a request gets when even the error page cannot be rendered —
	// an error in the root layout, or a render the engine could not finish.
	// Kit calls it `static_error_page` (`runtime/server/errors.js`) and falls
	// to it from exactly those two places.
	ErrorTemplate string `json:"errorTemplate"`
	// Target is the ECMAScript version the bundle was compiled to. It is
	// recorded because it is a correctness claim, not a preference: below
	// es2022 the bundle still runs and costs several times more.
	Target string `json:"target"`
	// GlobalName is the `__sveltekit_<hash>` object the boot script assigns
	// and the client reads.
	GlobalName string `json:"globalName"`
	// Assets is kit's `paths.assets`.
	Assets string `json:"assets"`
	// Relative is kit's `paths.relative`: whether the document addresses its
	// own assets by a path relative to the page.
	Relative bool `json:"relative"`
	// Client is the client bundle the document boots.
	Client ManifestClient `json:"client"`
	// Nodes describes each node of the table `Nodes` indexes, positionally.
	Nodes []ManifestSSRNode `json:"nodes"`
	// CSP is the app's `csp` option (`core/config/options.js`), or nil for a
	// build made before the adapter carried it — an absent field, not an
	// absent policy: a build whose app never set `csp` still gets one, with
	// every directive empty, because kit validates the option whether or not
	// a developer wrote it.
	CSP *ManifestCSP `json:"csp,omitempty"`
}
```

ManifestSSR is the build's server-rendering description.

## ManifestSSRNode

```go
type ManifestSSRNode struct {
	// Index is the number the node's own module declares, which is not its
	// position in this array. Kit renumbers the nodes it writes into a
	// manifest — a prerendered page's node is dropped and every later one
	// shifts down — and keeps the original numbering in the client bundle. The
	// boot script's `node_ids` are the original numbers, so a document built
	// from the manifest's positions hydrates the wrong components, silently,
	// because they are valid indices for other pages.
	Index int `json:"index"`
	// Component reports that kit compiled a component for this node. Kit omits
	// one for a node that will never be server-rendered.
	Component bool `json:"component"`
	// SSR and CSR are the page options this node sets, or nil for a node that
	// sets neither. Kit reduces them over a route's branch, last one wins.
	SSR *bool `json:"ssr"`
	CSR *bool `json:"csr"`
	// Imports, Stylesheets and Fonts are the client assets a page carrying
	// this node must link.
	Imports     []string       `json:"imports"`
	Stylesheets []string       `json:"stylesheets"`
	Fonts       []ManifestFont `json:"fonts"`
}
```

ManifestSSRNode is one node's contribution to a document.

## CSPDirectiveValue

```go
type CSPDirectiveValue struct {
	Sources []string
	Flag    bool
	IsFlag  bool
}
```

CSPDirectiveValue is one directive's configured value. Every directive in kit's schema is a `string_array()` except `upgrade-insecure-requests` and `block-all-mixed-content`, which are bare `boolean()`s (`core/config/options.js`). A zero CSPDirectiveValue — absent from the map entirely — is kit's `undefined`: a directive never configured, which never appears in the header. `IsFlag` distinguishes the two boolean directives from every other one, which carries `Sources` instead, even when that is `[]`.

### UnmarshalJSON

```go
func (v *CSPDirectiveValue) UnmarshalJSON(data []byte) error
```

UnmarshalJSON accepts either shape the manifest may carry: a JSON boolean for the two flag directives, or an array of strings for every other one.

## Transport

```go
type Transport map[string]Transporter
```

Transport is kit's `transport` hook: custom types keyed by name.

The keys are the wire, not a Go detail. Kit's client builds its decoders from the same object skgo is mirroring here —

```go
export const transport = {
	Money: { encode: (v) => v instanceof Money && { cents: v.cents }, decode: (data) => new Money(data.cents) }
};
```

— so a key skgo serializes under and a key `src/hooks.ts` declares must be spelled identically. A value goes out as `["Money", <slot>]`, and a client with no `Money` decoder fails to parse the response rather than quietly receiving a plain object.

Kit puts the transport on a \*universal\* hook because both sides need it, and skgo keeps that: the browser half stays in `src/hooks.ts` where kit looks for it, and this is the server half.

#### Why a Transport changes how results are encoded

Kit hands devalue the real object graph, so its encoders see a `Money` instance. skgo cannot: devalue refuses an arbitrary Go struct, so a result reaches it through a `json.Marshal`/`json.Unmarshal` round trip that has already turned every struct into a `map[string]any`. A reducer running after that sees an anonymous map and has nothing left to recognise — which is precisely why a domain type arrives in the browser as a plain object today.

So a non-empty Transport switches the outbound encoding to a walk that descends into the Go value along any path that can reach a transported type and leaves the transported values themselves in place, where devalue's reducers can still see them. Everything the walk cannot reach still goes through encoding/json exactly as before.

## Transporter

```go
type Transporter struct {
	// Type is the Go type this transports — kit's `instanceof`.
	Type reflect.Type
	// Encode returns what the browser's `decode` receives: a devalue value
	// model tree, the same shapes devalue.Stringify accepts. v is of Type.
	Encode func(v any) (encoded any, err error)
	// Decode turns what the browser's `encode` produced back into a Go value
	// of Type.
	Decode func(encoded any) (any, error)
}
```

Transporter is one member of kit's `transport` hook — the server half of the `encode`/`decode` pair the app declares in `src/hooks.ts`.

Kit's own shape is

```go
export interface Transporter<T, U> {
	encode: (value: T) => false | U;
	decode: (data: U) => T;
}
```

and the two halves sit on opposite sides of the wire. The browser runs kit's client, so `decode` is the app's TypeScript: it is what turns the encoding back into an instance of a class, which is the whole point — a domain type that arrives as a plain object has lost its methods. Go runs the server, so this is the half that decides whether a Go value is the custom type and what the browser receives for it.

Kit's `encode` answers two questions at once — is this my type, and what is its encoding — because JavaScript spells the first as `value instanceof Money`. Go spells it as a type, so Type asks it and Encode answers only the second.

Type is not decoration. skgo has to know, before it serializes anything, which Go types must survive the trip to devalue intact; see Transport.

## Transported

```go
func Transported[T any](key string) Marker
```

Transported declares that T crosses the wire as a custom type under key, and is skgo's mirror of one entry of kit's `transport` hook. The encoded object shape is the polytype projection of T's JSON fields.

It goes in `src/hooks.go`, beside the `src/hooks.ts` that holds the browser half, because kit's transport is a universal hook and both halves are one declaration:

```go
// src/hooks.go
var _ = skgo.Transported[businesslogic.Money]("Money")

// src/hooks.ts
export const transport = {
	Money: {
		encode: (v) => v instanceof Money && { cents: v.cents },
		decode: (data) => new Money(data.cents)
	}
};
```

`skgo generate` reads the marker, projects T's shape with polytype, and emits the Go encoder and strict decoder that answer the browser's pair. Like every other skgo marker it is a declaration and not a call: it does nothing at run time.

## RunPrerenderService

```go
func RunPrerenderService(transport Transport, loads []*ServerLoad, remotes []*Remote, options PrerenderServiceOptions) error
```

RunPrerenderService serves the generated application's build operations until its build owner closes the dedicated inherited control channel. It does not depend on the application's final frontend manifest.

## PrerenderOptions

```go
type PrerenderOptions struct {
	Inputs   any
	Dynamic  bool
	Validate any
}
```

PrerenderOptions configures build-time inputs for a prerender remote. Dynamic and Validate are reserved for follow-up implementations; the generator currently reports either authored key as unsupported.

## PrerenderServiceOptions

```go
type PrerenderServiceOptions struct {
	// Reroute is the app's universal Go hook, before build-time route selection.
	Reroute     Reroute
	BindRequest func(http.Handler) http.Handler
	Endpoints   []*Endpoint
	Matchers    map[string]ParamMatcher
	// FilterSerializedResponseHeaders supplies the same default passed to
	// SSROptions when a request's hooks do not select their own filter.
	FilterSerializedResponseHeaders func(name, value string) bool
}
```

PrerenderServiceOptions installs the application's ordinary request boundary in the build service. Endpoints answer Go-backed event.fetch calls.

## NativeDevelopment

```go
type NativeDevelopment struct {
	// contains filtered or unexported fields
}
```

NativeDevelopment configures the application's explicit development constructor. Its configured Go registries and request middleware are retained; Kit, rather than an embedded engine, supplies the page response.

### Handler

```go
func (d *NativeDevelopment) Handler(app http.Handler) (http.Handler, func(), error)
```

Handler mounts private callbacks beside the application's browser remote handler. Closing it cancels its own requests; handles have no meaning in any replacement process and are never retried there.

### NewSSR

```go
func (d *NativeDevelopment) NewSSR(build fs.FS, m Manifest, loads *Loads, remotes *Remotes, opts SSROptions) (*SSR, error)
```

NewSSR registers the application's real Go configuration for Kit-owned development rendering. Call this explicitly from NewDevelopmentHandler; build remains an ordinary filesystem and may be wrapped or composed. A nil development receiver creates the ordinary production renderer.

### NewStaticHandler

```go
func (d *NativeDevelopment) NewStaticHandler(build fs.FS, options ...StaticOption) (http.Handler, error)
```

NewStaticHandler gives Kit the page response while preserving the Go application's surrounding request middleware. A nil development receiver constructs the normal production static handler with the same options.

## NewNativeDevelopment

```go
func NewNativeDevelopment(secret string, kitSocket ...string) *NativeDevelopment
```
