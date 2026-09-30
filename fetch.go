package skgo

import (
	"context"
	"net/http"
	"net/url"
)

// Fetch dispatches a server-side fetch. A HandleFetch hook may change the
// request or return its own response without calling the dispatcher.
type Fetch func(request *http.Request) (*http.Response, error)

// HandleFetch mirrors Kit's server hook. It receives the normalized request
// before the dispatcher inherits the page's credentials. EventFrom(ctx) exposes
// the originating request and its locals. The supplied fetch dispatches directly
// rather than re-entering the hook, and retains the render's cancellation.
// Configure it through SSROptions; it runs during rendering, never in the browser.
type HandleFetch func(ctx context.Context, request *http.Request, fetch Fetch) (*http.Response, error)

type fetchParentKey struct{}
type fetchParent struct {
	request *http.Request
	url     *url.URL
}

func requestHeaders(r *http.Request) map[string]string {
	headers := make(map[string]string, len(r.Header))
	for name := range r.Header {
		headers[name] = r.Header.Get(name)
	}
	return headers
}
