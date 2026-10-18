---
title: "Loading data"
summary: "Server loads in Go, their inputs, what reaches the page, and when they rerun."
---

Before a `+page.svelte` and its layouts render, Kit runs their `load` functions. In SKGo a server `load` is a Go function in `page.server.go` or `layout.server.go`, and everything on this page is Kit's [Loading data](https://svelte.dev/docs/kit/load) with the server half written in Go.

## Page data

A `page.server.go` declares one load with `skgo.Load`. The function takes the generated `PageRequestEvent` and returns a struct whose JSON tags name the properties the page sees on `data`:

```go
/// file: src/routes/invoices/page.server.go
package invoices

import (
	"github.com/tylergannon/skgo"
)

type Invoice struct {
	Number           string `json:"number"`
	DueDate          string `json:"dueDate"`
	OutstandingCents int    `json:"outstandingCents"`
}

type PageData struct {
	AsOf     string    `json:"asOf"`
	Invoices []Invoice `json:"invoices"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{
		AsOf: "2026-09-28",
		Invoices: []Invoice{
			{Number: "INV-300", DueDate: "2026-10-15", OutstandingCents: 7400},
			{Number: "INV-100", DueDate: "2026-09-10", OutstandingCents: 12500},
			{Number: "INV-200", DueDate: "2026-09-05", OutstandingCents: 0},
		},
	}, nil
}

var _ = skgo.Load(pageLoad)
```

```svelte
<!--- file: src/routes/invoices/+page.svelte --->
<script lang="ts">
	import type { PageProps } from './$types';

	let { data }: PageProps = $props();
</script>

<p>As of {data.asOf}</p>
<ul>
	{#each data.invoices as invoice (invoice.number)}
		<li>{invoice.number} due {invoice.dueDate}</li>
	{/each}
</ul>
```

`data` is typed from the Go struct, through the generated `+page.server.ts`, so the page gets the same type safety Kit's `$types` gives any server load.

## Layout data

A `layout.server.go` declares its load the same way, and its data is available to the layout and to every page below it, merged with the page's own data as in Kit:

```go
/// file: src/routes/account/layout.server.go
// LayoutData is what every page under /account gets, whether it asks or not.
type LayoutData struct {
	// AccountUser is who the visitor is signed in as.
	AccountUser string `json:"accountUser"`
	// AccountSerial counts the times this load has run. The page shows it, so
	// a visitor can see for themselves whether moving between two pages under
	// this layout ran it again.
	AccountSerial int `json:"accountSerial"`
}
```

```go
/// file: src/routes/account/layout.server.go
// layoutLoad is the section's guard and its shared data.
//
// The session comes from the request's locals, which the app's one `handle`
// hook filled in; nothing here reads a cookie or knows how a session is
// spelled. Turning a signed-out visitor away here turns them away from every
// page below, so adding another one needs no new guard.
func layoutLoad(event LayoutRequestEvent) (LayoutData, error) {
	ctx := event.Context()
	session := app.LocalsFrom(ctx).Session
	if session.User == "" {
		return LayoutData{}, &skgo.Redirect{Status: 307, Location: "/"}
	}

	// The visitor can ask for this section's data again without navigating;
	// `invalidate('app:account')` in the browser is what makes kit's client ask
	// for this node and no other.
	event.Depends("app:account")

	return LayoutData{
		AccountUser:   session.User,
		AccountSerial: nextSerial(ctx),
	}, nil
}

var _ = skgo.Load(layoutLoad)
```

Layout data is reused across navigations between the layout's children; Kit asks Go to rerun the layout load only when something it depends on changed. See [Rerunning load functions](#Rerunning-load-functions).

## Universal vs server

Go loads are server loads. They always run on the server, and Kit's client asks for their result as `__data.json` on navigation, as it does for a `+page.server.ts`. A [universal load](https://svelte.dev/docs/kit/load#Universal-vs-server) runs in the browser too, so it stays in `+page.ts` or `+layout.ts`, written in TypeScript, and when a route has both, the Go load's result is the universal load's `data` argument:

```ts
/// file: src/routes/invoices/+page.ts
import type { PageLoad } from "./$types";

export const load: PageLoad = ({ data, url }) => {
  const overdueOnly = url.searchParams.get("overdue") === "1";
  const invoices = data.invoices
    .map((invoice) => ({
      ...invoice,
      overdue: invoice.outstandingCents > 0 && invoice.dueDate < data.asOf,
    }))
    .filter((invoice) => !overdueOnly || invoice.overdue)
    .sort((a, b) => a.dueDate.localeCompare(b.dueDate));
  return {
    invoices,
    totalCents: invoices.reduce((total, invoice) => total + invoice.outstandingCents, 0),
    overdueOnly,
  };
};
```

Use a Go load for anything that touches your Go services, a database or a secret, which is almost everything; use a universal load for what has to run in the browser or return something that cannot be serialised, as Kit's own guidance says.

### Input

A Go load's argument is the generated event for its directory: `PageRequestEvent` in a `page.server.go`, `LayoutRequestEvent` in a `layout.server.go`. Both are `skgo.RequestEvent` with three parts:

- `event.Params`, the route parameters as typed accessors; see [params](#Using-URL-data-params)
- `event.Locals`, your application's `Locals` struct, filled by the `handle` hook
- the embedded `*skgo.Event`, with `Cookie`, `SetCookie`, `SetHeader`, `URL`, `SearchParam`, `RouteID`, `Fetch`, `Depends`, `Untrack` and `Request`

`event.Context()` is the request's context, and it is what the rest of your Go code should take: `skgo.EventFrom(ctx)` recovers the event from it anywhere down the call stack. See [Go and web standards](go-standards#context.Context).

### Output

A load returns a struct. Every exported field with a JSON tag becomes a property of `data`, and the generator writes its TypeScript type from the Go type: strings, numbers, booleans, slices, maps, nested structs, `time.Time` as a string, and a `skgo.Deferred[T]` as a `Promise<T>`. A struct that cannot be projected fails `skgo generate` with the reason. [Types and transport](transport) has the rules, including how a Go type with methods crosses as a class.

## Using URL data

### url

`event.URL()` is the page's URL as a `*url.URL`, with Kit's internal query parameters removed. Reading it makes the load depend on the whole URL. In a load, `url.Fragment` is empty, as `url.hash` is unavailable in Kit.

### route

`event.RouteID()` is the route's id, such as `/blog/[slug]`.

### params

Each parameter is a method on `event.Params`, named after the segment and typed by its matcher: `event.Params.Slug()` is a `string`, and under `[number=Order]` with the matcher from [Routing](routing#Route-parameters-Matching), `event.Params.Number()` is the Go type the matcher returned.

```go
/// file: src/routes/account/reruns/[slug]/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	slug := event.Params.Slug()
	filter, _ := event.SearchParam("x")
	event.Depends("app:reruns")
	who := visitor.Of(ctx)
	runs.Lock()
	runs.byVisitor[who]++
	serial := runs.byVisitor[who]
	runs.Unlock()
	return PageData{Slug: slug, Filter: filter, Serial: serial}, nil
}
```

`event.SearchParam(name)` reads one query parameter and records a dependency on that parameter alone, as Kit's `url.searchParams.get(name)` does.

## Making fetch requests

`event.Fetch(ctx, request)` is Kit's special `fetch`: a relative URL resolves against the page's URL, a request for one of the application's own routes is answered in-process with the visitor's cookies, and anything else goes out over HTTP.

```go
/// file: src/routes/request-fetch/page.server.go
// pageLoad calls /api/request-fetch the way a load calls any service: with
// Event.Fetch, a relative URL, and no socket. The visitor's session cookie
// travels with the subrequest, so the endpoint knows who is asking.
func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	request, err := http.NewRequest(http.MethodGet, "/api/request-fetch", nil)
	if err != nil {
		return PageData{}, err
	}
	response, err := event.Fetch(ctx, request)
	if err != nil {
		return PageData{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return PageData{}, skgo.Errorf(502, "the request-fetch service answered %d", response.StatusCode)
	}
	var data PageData
	if err := json.NewDecoder(response.Body).Decode(&data); err != nil {
		return PageData{}, fmt.Errorf("reading the request-fetch service: %w", err)
	}
	return data, nil
}
```

A fetch inside a Go load records no dependency, as a fetch in a server load does not in Kit. [Go and web standards](go-standards#Fetch) has the details.

## Cookies

A server load may read cookies with `event.Cookie(name)` and write them with `event.SetCookie(name, value, skgo.CookieOptions{...})`. The example's root layout hands every browser a visitor id this way, so that every load and remote function on the page can read it back:

```go
/// file: businesslogic/visitor/visitor.go
// Start makes sure a browser has a visitor id, writing a new one on the
// response if the request carried none. The root layout's load calls it.
//
// A browser is a request with a Sec-Fetch-Site header, which browsers send on
// every request to a secure origin (localhost counts as one) and which a page's
// script cannot set. Nothing finer is needed: a caller that sends neither that
// header nor the cookie is exactly the caller Default exists for, and handing
// it an id it will never send back would move this one request off the list
// it wrote to a moment ago.
func Start(ctx context.Context) error {
	event := skgo.EventFrom(ctx)
	visitor, ok := event.Cookie(Cookie)
	if !ok || visitor == "" {
		if event.Request().Header.Get("Sec-Fetch-Site") == "" {
			return nil
		}
		raw := make([]byte, 16)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		visitor = base64.RawURLEncoding.EncodeToString(raw)
		if err := event.SetCookie(Cookie, visitor, skgo.CookieOptions{Path: "/"}); err != nil {
			return err
		}
	}
	app.LocalsFrom(ctx).Visitor = visitor
	return nil
}
```

A cookie set in a load is visible to later reads of `event.Cookie` in the same request, as in Kit. Writing a cookie from a `query` is refused, and the `skgo advice SKGO001` analyzer check flags it before it runs.

## Headers

`event.SetHeader(name, value)` is Kit's `setHeaders`, with Kit's rules: it cannot set `set-cookie`, and a header set twice in one request is an error except `server-timing`, which accumulates. Setting `cache-control` from a load is the usual use.

## Using parent data

`skgo.Parent[T](ctx)` is `await parent()`, decoded into the type of the nearest layout load above:

```go
/// file: src/routes/account/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	parent, err := skgo.Parent[LayoutData](ctx)
	if err != nil {
		return PageData{}, err
	}
	return PageData{ParentUser: parent.AccountUser}, nil
}
```

As in Kit, calling it makes the page load wait for the layout load and rerun when the layout load reruns; see [Implications for authentication](#Implications-for-authentication) before guarding every page this way.

## Errors

A load that returns `skgo.Errorf(status, format, ...)` is Kit's `error(status, message)`: the response has that status, and the nearest `+error.svelte` renders with `page.status` and `page.error.message` set from it.

```go
/// file: src/routes/account/statement/page.server.go
// Package statement is the page whose load fails, so that a visitor can see
// what a failed load does: an error page with the status and message the Go
// function chose, inside the layout that declared the error page.
package statement

import (
	"github.com/tylergannon/skgo"
)

// PageData is never produced.
type PageData struct {
	// Balance would be the amount owed.
	Balance int `json:"balance"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, skgo.Errorf(402, "Your account is in arrears")
}

var _ = skgo.Load(pageLoad)
```

Any other error is an unexpected error, and Kit's rule applies: the visitor is never shown its text. The response is a 500 whose message is whatever your `handleError` hook returns, or `Internal Error` without one.

```go
/// file: src/routes/error/unexpected/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, errors.New("the connection string is postgres://ada:hunter2@db")
}
```

The hook is a Go function set on the server's configuration, and what it returns is merged over the error the page receives:

```go
/// file: server.go
// HandleError is the app's `handleError` hook — kit's own contract, mirrored:
// it runs for page and remote-function failures, expected or not
// (`exports/hooks/public.d.ts`: "runs for every error thrown ... except
// redirects"), and whatever it returns is merged over what the error already
// carries.
//
// Every failure gets a support id, which is the one thing a real error page
// almost always adds and kit's own default has no room for. An error nobody
// meant to happen also loses its own message here: this app makes no promise
// about what an arbitrary Go error's text might contain, so the visitor is
// told something a person wrote instead of it. caught.Err still carries the
// real one, for a hook that wants to log it before replacing it.
func handleError(ctx context.Context, caught skgo.CaughtError) map[string]any {
	if caught.Kind == "validation" {
		if event := skgo.EventFrom(ctx); event != nil && event.Request() != nil && strings.HasSuffix(event.Request().URL.Path, "/validateCatalog") {
			return map[string]any{"message": "Choose a catalog name.", "reference": "catalog-validation-166"}
		}
	}
	extra := map[string]any{"supportId": "case-1121"}
	if caught.Kind == "unknown" {
		extra["message"] = "Something went wrong on our end."
	}
	return extra
}
```

The `skgo advice SKGO004` analyzer check flags an SKGo operation whose error is discarded.

## Redirects

Return a `*skgo.Redirect` to redirect, with the status and location Kit's `redirect(status, location)` takes. The layout load under [Layout data](#Layout-data) sends a signed-out visitor to `/` with a 307 this way. As in Kit, the redirect stops the load; nothing after it runs.

## Streaming with promises

A load may return a value it does not have yet. A field of type `skgo.Deferred[T]` reaches the page as a `Promise<T>`, the page renders without waiting for it, and the value arrives on the same response when the goroutine producing it finishes:

```go
/// file: src/routes/account/orders/page.server.go
// PageData is the orders page. The total is known at once; the orders
// themselves are slow, so they are deferred and arrive on the same response.
type PageData struct {
	// OrderTotal is on screen immediately.
	OrderTotal int `json:"orderTotal"`
	// Orders arrives later, without a second request.
	Orders skgo.Deferred[[]Order] `json:"orders"`
}

func pageLoad(event PageRequestEvent) (PageData, error) {
	ctx := event.Context()
	return PageData{
		OrderTotal: 2,
		Orders: skgo.Async(ctx, func(ctx context.Context) ([]Order, error) {
			select {
			case <-time.After(1500 * time.Millisecond):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			return []Order{
				{Item: "a slow parcel", Cents: 1999},
				{Item: "a slower parcel", Cents: 2999},
			}, nil
		}),
	}, nil
}
```

```svelte
<!--- file: src/routes/account/orders/+page.svelte --->
<p>{data.orderTotal} orders</p>

{#await data.orders}
	<p>fetching the orders…</p>
{:then orders}
	<ul>
		{#each orders as order}
			<li>{order.item}</li>
		{/each}
	</ul>
{/await}
```

`skgo.Async(ctx, fn)` starts `fn` on its own goroutine with a context that is cancelled when the visitor leaves; `skgo.Resolved(value)` makes a `Deferred` that is already settled. A `Deferred` may sit anywhere inside the returned struct, as a promise may in Kit. Kit's limits hold: headers and status are fixed once the response starts streaming, so a deferred value cannot set a header or redirect, and a deferred error reaches the page as a rejected promise.

## Parallel loading

Kit runs the loads of a page and its layouts concurrently, and in SKGo each one runs on its own goroutine. Code they share has to be safe to call from several goroutines at once; see [State management](state-management).

## Rerunning load functions

Kit reruns a load during navigation only when something it depended on changed, and SKGo records those dependencies the same way:

- reading a parameter through its `event.Params` accessor depends on that parameter
- `event.URL()` depends on the whole URL; `event.SearchParam(name)` depends on that one query parameter
- `skgo.Parent` depends on the parent load
- `event.Depends(id)` depends on an identifier the page can invalidate

In the load under [params](#Using-URL-data-params), navigating from `/account/reruns/a` to `/account/reruns/b` reruns it because `Slug()` was read, and so does a change to `?x=`; a change to any other query parameter does not. As in Kit, dependencies are recorded only while the load runs: reading a parameter inside a goroutine started by `skgo.Async` records nothing.

### Untracking dependencies

`event.Untrack(fn)` runs `fn` without recording what it reads:

```go
/// file: src/routes/typed-dependencies/[a=Order]/[b=Order]/[ignored]/page.server.go
var ignored string
event.Untrack(func() { ignored = event.Params.Ignored() })
```

### Manual invalidation

`event.Depends("app:account")` in the layout load under [Layout data](#Layout-data) lets the page rerun it on demand:

```svelte
<!--- file: src/routes/account/+layout.svelte --->
<script lang="ts">
	import { invalidate } from '$app/navigation';

	let { data, children } = $props();
</script>

<button onclick={() => invalidate('app:account')}>Refresh the account</button>

{@render children()}
```

`invalidate(id)` reruns every load that declared `id`; `refreshAll()` reruns them all. The identifier is any string of the form `[a-z]+:...`, as in Kit.

### When do load functions rerun?

A Go load reruns when a parameter it read changed, when the URL or a query parameter it read changed, when a parent load it waited on reran, when an identifier it declared was invalidated, or when everything was refreshed with `refreshAll()`. It does not rerun because of a fetch it made. Rerunning updates `data` in place; the component is not recreated.

## Implications for authentication

Kit's cautions apply unchanged: a layout load does not run on every navigation between its children, and a page load runs concurrently with its layout unless it calls `skgo.Parent`. The example protects `/account` with the layout load shown under [Layout data](#Layout-data): the session comes from `event.Locals`, which the `handle` hook filled in once for the request, and a signed-out visitor is redirected before any page below runs. A page that needs its own check reads `event.Locals` itself; it does not need to wait on the parent.

## Using getRequestEvent

`skgo.EventFrom(ctx)` is Kit's [`getRequestEvent`](https://svelte.dev/docs/kit/load#Using-getRequestEvent): a function several calls below the load can reach the request without being handed the event. See [Go and web standards](go-standards#context.Context).
