// Render-time I/O: the Go side of `event.fetch` and `$app/paths` `match()`.
//
// Both are calls a render makes back out to Go rather than anything the engine
// does itself. `event.fetch` never opens a socket — a same-origin fetch is an
// in-process call into the same server-route registry that answers
// `+server.ts` requests from the outside, run against an httptest recorder
// rather than a real connection — and `match()` asks the same route table
// every page, load and endpoint request matches against, because building a
// second router for the engine would be the thing this project's own rules
// forbid: reimplementing kit rather than mirroring it.
package skgo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tylergannon/skgo/internal/ssr"
)

// fetchDispatch answers one render-time `event.fetch`. It is the Fetch host
// ssr.Engine.Render is given, so payload is a JSON-encoded ssr.FetchRequest
// and the reply is a JSON-encoded ssr.FetchAnswer.
func (s *SSR) fetchDispatch(ctx context.Context, payload []byte) ([]byte, error) {
	var req ssr.FetchRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("skgo: decoding a render-time fetch: %w", err)
	}
	answer := s.fetchAnswer(ctx, req)
	return json.Marshal(answer)
}

// fetchAnswer runs one render-time fetch against the app's own server-route
// registry, exactly as if the browser had asked for it — except that nothing
// here opens a connection. httptest.NewRecorder is a ResponseWriter that
// writes to memory; s.fetch.ServeHTTP is the same http.Handler a real request
// to this path would reach.
func (s *SSR) fetchAnswer(ctx context.Context, req ssr.FetchRequest) ssr.FetchAnswer {

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, body)
	if err != nil {
		return ssr.FetchAnswer{Error: "Failed to fetch"}
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	parent, hasParent := ctx.Value(fetchParentKey{}).(fetchParent)
	dispatch := func(request *http.Request) (*http.Response, error) {
		if s.fetch == nil {
			return nil, fmt.Errorf("skgo: no server route is registered to answer event.fetch during a render")
		}
		// A subrequest has its own locals; only the fetch hook sees the
		// originating request's locals. Mount Handle over Fetch to populate
		// the endpoint's locals from the inherited request credentials.
		request = request.Clone(context.WithValue(ctx, localsKey{}, &locals{values: map[reflect.Type]any{}}))
		if hasParent {
			target := request.URL
			if target == nil || !sameFetchOrigin(target, parent.url) {
				return nil, fmt.Errorf("skgo: event.fetch only reaches this app's own routes during server-side rendering")
			}
			// Kit inherits credentials only after handleFetch rewrites the URL.
			if req.Credentials != "omit" {
				cookies := map[string]string{}
				for _, cookie := range parent.request.Cookies() {
					cookies[cookie.Name] = cookie.Value
				}
				for _, cookie := range request.Cookies() {
					cookies[cookie.Name] = cookie.Value
				}
				request.Header.Del("Cookie")
				names := make([]string, 0, len(cookies))
				for name := range cookies {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					request.AddCookie(&http.Cookie{Name: name, Value: cookies[name]})
				}
				if !hasFetchHeader(request.Header, "Authorization") {
					if value := parent.request.Header.Get("Authorization"); value != "" {
						request.Header.Set("Authorization", value)
					}
				}
			}
			if !hasFetchHeader(request.Header, "Origin") {
				request.Header.Set("Origin", parent.url.Scheme+"://"+parent.url.Host)
			}
			if request.Method == http.MethodGet || request.Method == http.MethodHead {
				request.Header.Del("Origin")
			}
			if !hasFetchHeader(request.Header, "Accept") {
				request.Header.Set("Accept", "*/*")
			}
			if !hasFetchHeader(request.Header, "Accept-Language") {
				if value := parent.request.Header.Get("Accept-Language"); value != "" {
					request.Header.Set("Accept-Language", value)
				}
			}
		}
		rec := httptest.NewRecorder()
		s.fetch.ServeHTTP(rec, request)
		return rec.Result(), nil
	}
	var response *http.Response
	if s.handleFetch != nil {
		response, err = s.handleFetch(ctx, httpReq, dispatch)
	} else {
		response, err = dispatch(httpReq)
	}
	if err != nil {
		return ssr.FetchAnswer{Error: err.Error()}
	}
	if response == nil {
		return ssr.FetchAnswer{Error: "skgo: handleFetch returned no response"}
	}
	var raw []byte
	if response.Body != nil {
		defer response.Body.Close()
		raw, err = io.ReadAll(response.Body)
		if err != nil {
			return ssr.FetchAnswer{Error: err.Error()}
		}
	}
	headers := map[string]string{}
	serialized := map[string]string{}
	for name := range response.Header {
		value := response.Header.Get(name)
		headers[name] = value
		lower := strings.ToLower(name)
		if s.filterSerializedResponseHeaders != nil && s.filterSerializedResponseHeaders(lower, value) {
			serialized[lower] = value
		}
	}
	answer := &ssr.FetchResponse{Status: response.StatusCode, StatusText: http.StatusText(response.StatusCode), Headers: headers, SerializedHeaders: serialized}
	if utf8.Valid(raw) {
		answer.Body = string(raw)
	} else {
		answer.BodyBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	return ssr.FetchAnswer{Response: answer}

}

// matchDispatch answers one render-time `$app/paths` `match()` lookup. It is
// the Match host ssr.Engine.Render is given: pathname arrives already decoded
// and base-stripped, the same shape s.loads.match and Endpoints.match both
// key on, so this is the same route table a page, load or endpoint request
// matches against rather than a second implementation of kit's router.
func (s *SSR) matchDispatch(pathname string) (routeID string, params map[string]string, ok bool) {
	route, params, matched := s.loads.match(pathname)
	if !matched {
		return "", nil, false
	}
	return route.id, params, true
}

func sameFetchOrigin(a, b *url.URL) bool {
	if a == nil || b == nil || a.Scheme != b.Scheme || !strings.EqualFold(a.Hostname(), b.Hostname()) {
		return false
	}
	port := func(u *url.URL) string {
		if u.Port() != "" {
			return u.Port()
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return (a.Scheme == "http" || a.Scheme == "https") && port(a) == port(b)
}

func hasFetchHeader(h http.Header, name string) bool {
	_, ok := h[http.CanonicalHeaderKey(name)]
	return ok
}
