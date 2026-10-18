package app

import (
	"context"
	"github.com/tylergannon/skgo"
	"net/http"
	"strings"
)

// docs: application-config
// Config is the application policy selected by go:generate and read by server.go.
// Generated registrations and the request boundary remain generated.
var Config = skgo.ApplicationConfig{
	HandleFetch:                     handleFetch,
	HandleError:                     handleError,
	FilterSerializedResponseHeaders: SerializedHeader,
}

// docs: end application-config

// docs: handle-error
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

// docs: end handle-error

// docs: handle-fetch
// The example hook rewrites an API alias query before dispatch and marks the
// response. Fetching this API in a universal load still reuses its response
// during hydration; the hook itself only runs in Go.
func handleFetch(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error) {
	request = request.Clone(ctx)
	if request.URL.Path == "/api/todos" {
		query := request.URL.Query()
		query.Del("via")
		request.URL.RawQuery = query.Encode()
	}
	if strings.HasPrefix(request.URL.Path, "/api/replay/") && request.URL.Query().Has("alias") {
		query := request.URL.Query()
		query.Del("alias")
		query.Set("step", "rewritten")
		request.URL.RawQuery = query.Encode()
	}
	response, err := next(request)
	if err == nil && strings.HasPrefix(request.URL.Path, "/api/") {
		response.Header.Set("X-Fetch-Hook", "Go")
	}
	return response, err
}

// docs: end handle-fetch
