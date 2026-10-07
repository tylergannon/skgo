package serverhooks

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/businesslogic"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
)

const SessionCookie = "skgo_session"

var Handle = params.Sequence(params.Handle(initialize).Middleware(), SerializedHeaders, VisitMiddleware)

func visitSlug(p params.Params) string {
	if value, ok := p.Slug().(params.SlugParam_String); ok {
		return value.Value
	}
	return ""
}
func filterSerializedResponseHeaders(name, value string) bool {
	switch name {
	case "x-fetch-hook", "x-replay-allowed":
		return true
	case "set-cookie":
		return strings.HasPrefix(value, "lamp=")
	}
	return false
}

// Handle is the app's `handle` hook — the one place it decides who the caller
// is. It runs once per request, before any load or remote function, and puts
// the answer where all of them can read it with app.LocalsFrom.
//
// A guard is then a load that reads the session; see
// web/src/routes/account/layout.server.go, which turns a signed-out visitor
// away from every page under /account without any of those pages knowing.
func initialize(ctx context.Context, event params.RequestEvent) (params.RequestEvent, error) {
	request := event.Request()
	if request.Method == http.MethodPost && request.URL.Path == "/actions" {
		switch request.URL.Query().Get("hook") {
		case "sign-in":
			return event, &skgo.Redirect{Status: http.StatusSeeOther, Location: "/actions/signed-in?required=1"}
		case "forbidden":
			return event, skgo.Errorf(http.StatusForbidden, "Hook denied this edit")
		}
	}
	if run := request.URL.Query().Get("run"); run != "" && request.URL.Path != "/api/replay-count" {
		businesslogic.Replays.Record(run, request.Method+" "+request.URL.Path)
	}
	id, _ := event.Cookie(SessionCookie)
	event.Locals.Session = businesslogic.Default.Session(id)
	return event, nil
}

// VisitMiddleware is the part of the app's `handle` hook that wraps the
// response. For the /middleware pages it establishes a visit before anything
// answers — refreshing the visit cookie when it is missing or stale, so the
// load that runs next reads the token from the same cookie jar the visitor
// receives it from — and marks the response it gets back with what it
// observed on the matched event.
func VisitMiddleware(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
	route := event.RouteID()
	if route == "/stream" {
		response, err := resolve(ctx, event, skgo.ResolveOptions{TransformPageChunk: markTransformed})
		if err != nil {
			return nil, err
		}
		response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
		return response, nil
	}
	if route != "/middleware" && !strings.HasPrefix(route, "/middleware/") {
		return resolve(ctx, event)
	}
	token, _ := event.Cookie(businesslogic.VisitCookie)
	if !strings.HasPrefix(token, "visit-") {
		token = businesslogic.FreshVisitToken
		if err := event.SetCookie(businesslogic.VisitCookie, token, skgo.CookieOptions{}); err != nil {
			return nil, err
		}
	}
	event.Locals.Visit = businesslogic.Visit{
		Token: token, Route: route, Slug: visitSlug(event.Params), Data: event.IsDataRequest(),
	}
	response, err := resolve(ctx, event, skgo.ResolveOptions{TransformPageChunk: markTransformed})
	if err != nil {
		return nil, err
	}
	response.Header.Set("X-Skgo-Middleware", route+" data="+strconv.FormatBool(event.IsDataRequest()))
	return response, nil
}

// markTransformed is the document transform of the /middleware pages and of
// /stream, whose deferred values must still reach kit's client behind a
// transformed shell: the one place a middleware chooses to rewrite the
// assembled document, here by marking the root element so a scenario can see
// kit's client hydrate a transformed page.
func markTransformed(_ context.Context, html string, _ bool) (string, error) {
	return strings.Replace(html, `<html lang="en">`, `<html lang="en" data-middleware-transformed="yes">`, 1), nil
}

// SerializedHeaders is the app's choice of which headers a universal load's
// hydration data carries. It is made here, per request, rather than on the
// renderer, so the whole universal-fetch suite runs through the request-local
// path.
func SerializedHeaders(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
	return resolve(ctx, event, skgo.ResolveOptions{FilterSerializedResponseHeaders: filterSerializedResponseHeaders})
}
