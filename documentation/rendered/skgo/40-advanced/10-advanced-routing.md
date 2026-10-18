---
title: "Advanced routing"
summary: "Rest and optional parameters, Go parameter matchers, typed parameter values and layout groups."
---

Route syntax is Kit's. Rest parameters, optional parameters, matchers, sorting, encoding and layout groups all mean what [Kit's advanced routing](https://svelte.dev/docs/kit/advanced-routing) says they mean, and the browser's router is Kit's own. The difference is on the server: Go matches the request, using the route patterns and order Kit compiled, and hands your Go code typed parameter values. This page covers what that adds. The basics of parameters and matchers are on [Routing](routing#Route-parameters).

## Rest parameters

A `[...rest]` segment matches any number of path segments, as in [Kit](https://svelte.dev/docs/kit/advanced-routing#Rest-parameters). In a Go load, endpoint or remote function, the parameter is one `string` holding the matched segments joined by `/`, and the empty string when nothing matched:

```go
/// file: src/routes/shadow/[...rest]/server.go
func get(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("dynamic-rest:" + skgo.EventFrom(r.Context()).Param("rest")))
}

var _ = skgo.GET(get)
```

A load on a rest route gets the same value from its generated accessor, `event.Params.Rest()`.

### 404 pages

Kit's pattern for a [custom 404 inside a subtree](https://svelte.dev/docs/kit/advanced-routing#Rest-parameters-404-pages) works with a Go load. Add a `[...path]` route under the subtree and return a 404 from its load:

```go
/// file: src/routes/missing/[...path]/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	return PageData{}, skgo.Errorf(http.StatusNotFound, "Not Found")
}

var _ = skgo.Load(pageLoad)
```

The nearest `+error.svelte` above the route renders the error, as [Errors](errors#App-errors) describes.

## Optional parameters

A `[[lang]]` segment may be absent, as in [Kit](https://svelte.dev/docs/kit/advanced-routing#Optional-parameters). Its Go accessor returns a pointer, `nil` when the segment is missing:

```go
/// file: src/routes/lang/[[lang]]/page.server.go
func pageLoad(event PageRequestEvent) (PageData, error) {
	lang := "en"
	if value := event.Params.Lang(); value != nil {
		lang = *value
	}
	return PageData{Lang: lang}, nil
}
```

An optional parameter with a matcher, such as `[[n=Order]]`, returns a pointer to the matcher's type. When the matcher rejects a segment, the segment is not consumed and the next parameter or route gets a chance at it, as in Kit.

## Matching

Kit declares matchers in `src/params.ts` with [`defineParams`](https://svelte.dev/docs/kit/@sveltejs-kit-params), and the browser runs them. SKGo matches on the server in Go, so every matcher has two implementations: the Kit one in `src/params.ts` and a Go one in `src/params.go`. You write both; neither is generated from the other. [Routing](routing#Route-parameters-Matching) shows the pair for the `Order` matcher. `skgo generate` stops with `matcher "Order" requires a Go matcher in src/params.go` when the Go half is missing, and refuses a route that names a matcher declared on only one side.

### Go matchers

A Go matcher is an exported function in `src/params.go` with the same name as the Kit matcher. It takes the decoded segment and returns the parameter's value and whether the segment matched. Its signature must be `func(string) (T, bool)`; `skgo generate` stops with `matcher Order must be func(string) (T, bool)` otherwise.

`T` can be any exported type, but neither `src/params.go` nor the type's package may import your generated bindings, your route packages or a package with a `.remote.go` file, because the bindings import the matcher. A type declared in `src/params.go` itself qualifies. Otherwise `skgo generate` stops with `move matcher domain types to a leaf package`.

The two halves must agree. When Go accepts a segment the browser rejects, a client-side navigation goes to a different route than a full page load does. Test the boundaries of the Go matcher with an ordinary Go test beside it, and keep the TypeScript matcher's rules the same:

```go
/// file: src/params_test.go
package hooks

import "testing"

func TestOrderMatcherBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input    string
		accepted bool
		label    string
	}{
		{"0", true, "Order #0"}, {"+0", true, "Order #0"}, {"00042", true, "Order #42"},
		{"1000000", true, "Order #1000000"}, {"1000001", false, ""},
		{"-1", false, ""}, {"9223372036854775808", false, ""}, {"banana", false, ""},
	} {
		t.Run(tc.input, func(t *testing.T) {
			value, accepted := Order(tc.input)
			if accepted != tc.accepted {
				t.Fatalf("accepted=%t, want %t", accepted, tc.accepted)
			}
			if accepted && value.Label() != tc.label {
				t.Fatalf("label=%q, want %q", value.Label(), tc.label)
			}
		})
	}
}
```

When the Go matcher rejects a segment, Go tries the next route in Kit's order, the same fallback Kit's router applies. A route with a matcher and no Go load still runs the Go matcher, so a `+page.svelte` with only a universal load or a `server.go` under `[number=Order]` gets the same fallback.

### Typed parameter values

The value a Go matcher returns is the parameter's Go type. On `typed-load/[number=Order]`, the page's accessor returns an `OrderNumber`, with its methods:

```go
/// file: src/routes/typed-load/[number=Order]/page.server.go
package typedload

import "github.com/tylergannon/skgo"

type PageData struct {
	Label string `json:"label"`
}

func load(event PageRequestEvent) (PageData, error) {
	return PageData{Label: event.Params.Number().Label()}, nil
}

var _ = skgo.Load(load)
```

Kit's own matcher return value still becomes the parameter in the browser, so `page.params.number` is what `params.ts` returned, not the Go type.

Where the accessor's type comes from depends on where you read it:

- A page reads its own parameters as plain values: `string`, or the matcher's type.
- A layout reads its own parameters as values, and parameters that only a child page has as pointers, `nil` when the current page lacks them. When two child pages give the same name different types, the accessor returns an interface with one variant per type, such as `NumberParam_OrderNumber`.
- Hooks, commands and forms run for every route, so they see the app-wide `params.Params`, where every accessor returns such an interface: `Slug()` returns a `Key_Slug`, and a type assertion to `params.SlugParam_String` gives the string.

A layout above `[section]/[item]` reads `Section()` as a value and `Item()` as a pointer:

```go
/// file: src/routes/event-layout/[section]/layout.server.go
package eventlayout

import (
	"crypto/rand"
	"sync"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
)

type LayoutData struct {
	Section string `json:"eventSection"`
	Item    string `json:"eventItem"`
	Route   string `json:"eventRoute"`
	User    string `json:"eventUser"`
	Serial  int    `json:"eventSerial"`
}

var runs = struct {
	sync.Mutex
	byVisitor map[string]int
}{byVisitor: map[string]int{}}

func layoutLoad(event LayoutRequestEvent) (LayoutData, error) {
	data := LayoutData{Section: event.Params.Section(), User: event.Locals.Session.User}
	if app.LocalsFrom(event.Context()) != event.Locals {
		return data, skgo.Errorf(500, "layout event and context locals differ")
	}
	read := func() {
		// Item belongs only to descendant pages, so it is optional here.
		if item := event.Params.Item(); item != nil {
			data.Item = *item
		}
		data.Route = event.RouteID()
	}
	mode, _ := event.SearchParam("route")
	if mode == "tracked" {
		read()
	} else {
		event.Untrack(read)
	}
	who, _ := event.Cookie("skgo_event_layout_visitor")
	if who == "" {
		who = rand.Text()
		if err := event.SetCookie("skgo_event_layout_visitor", who, skgo.CookieOptions{Path: "/"}); err != nil {
			return data, err
		}
	}
	runs.Lock()
	runs.byVisitor[who]++
	data.Serial = runs.byVisitor[who]
	runs.Unlock()
	return data, nil
}

var _ = skgo.Load(layoutLoad)
```

Every accessor records that the load read the parameter, so Kit reruns the load when it changes. Read a parameter inside `event.Untrack`, as the layout above does, to avoid that; see [Loading data](load#Rerunning-load-functions-Untracking-dependencies).

## Sorting

Kit's [sorting rules](https://svelte.dev/docs/kit/advanced-routing#Sorting) apply. Go reads Kit's compiled route order from the build manifest and tries routes in that order, so the server and the browser pick the same route.

## Encoding

Kit's [encoded characters](https://svelte.dev/docs/kit/advanced-routing#Encoding) in route directory names, such as `[x+3a]`, are compiled by Kit into the route pattern, and Go matches against that pattern. Parameter values reach Go decoded the way Kit decodes them: `/items/a%2Fb` gives `a/b` to `[id]`, and a malformed escape such as `%FF` is a `400 Bad Request` before any load or matcher runs.

Directory names like `[id]` and `(marketing)` are not legal Go import paths; see [Routing](routing#Go-packages) for how code outside the route tree imports them.

## Advanced layouts

[Layout groups](https://svelte.dev/docs/kit/advanced-routing#Advanced-layouts-(group)) and `+page@` and `+layout@` [resets](https://svelte.dev/docs/kit/advanced-routing#Advanced-layouts-Breaking-out-of-layouts) are decided by Kit's Svelte files, and Go code is not involved. A Go load in a group is an ordinary route package whose directory name has parentheses: `src/routes/(marketing)/pricing/page.server.go` is `package pricing`, its URL is `/pricing`, and its generated event type is the same `PageRequestEvent` any page gets.
