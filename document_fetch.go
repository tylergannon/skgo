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
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
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

// fetchAnswer runs one render-time fetch through the same requestFetcher
// Event.Fetch uses: the app's own routes are answered in-process by s.fetch,
// everything else goes out over HTTP, and Kit's credential rules apply.
func (s *SSR) fetchAnswer(ctx context.Context, req ssr.FetchRequest) ssr.FetchAnswer {

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	switch {
	case req.BodyBase64 != "":
		raw, err := base64.StdEncoding.DecodeString(req.BodyBase64)
		if err != nil {
			return ssr.FetchAnswer{Error: "Failed to fetch"}
		}
		body = bytes.NewReader(raw)
	case req.Body != "":
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, req.URL, body)
	if err != nil {
		return ssr.FetchAnswer{Error: "Failed to fetch"}
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	parent, ok := ctx.Value(fetchParentKey{}).(fetchParent)
	if !ok {
		return ssr.FetchAnswer{Error: "skgo: a render-time fetch needs the request being rendered"}
	}
	filter := parent.filter
	if filter == nil {
		filter = s.filterSerializedResponseHeaders
	}
	fetcher := &requestFetcher{handler: s.fetch, hook: s.handleFetch, base: s.base, prerendered: s.prerendered}
	source := fetchSource{url: parent.url, request: parent.request, jar: parent.jar}
	if source.jar == nil {
		if event := EventFrom(ctx); event != nil {
			source.jar = event.jar
		}
	}
	response, err := fetcher.fetch(ctx, source, httpReq, fetchOptions{
		credentials: FetchCredentials(req.Credentials),
		mode:        FetchMode(req.Mode),
	})
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
	answer := &ssr.FetchResponse{
		Status:     response.StatusCode,
		StatusText: http.StatusText(response.StatusCode),
		Headers:    fetchHeaders(response.Header, filter),
	}
	if utf8.Valid(raw) {
		answer.Body = string(raw)
	} else {
		answer.BodyBase64 = base64.StdEncoding.EncodeToString(raw)
	}
	return ssr.FetchAnswer{Response: answer}

}

// fetchHeaders lists a response's headers the way the standard's Headers
// iterates them — lowercase names in order, a repeated name joined with ", ",
// Set-Cookie once per value — and marks the ones the app's
// filter lets a universal load's hydration data carry.
// Kit applies that filter to what `get` returns as well as to what iteration
// yields, so a Set-Cookie that has several values is also judged joined.
func fetchHeaders(header http.Header, filter func(name, value string) bool) []ssr.FetchHeader {
	allowed := func(name, value string) bool {
		return filter != nil && filter(name, value)
	}
	names := make([]string, 0, len(header))
	for name := range header {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
	var out []ssr.FetchHeader
	for _, name := range names {
		lower := strings.ToLower(name)
		values := header[name]
		if lower != "set-cookie" {
			joined := strings.Join(values, ", ")
			out = append(out, ssr.FetchHeader{Name: lower, Value: joined, Serialized: allowed(lower, joined)})
			continue
		}
		for _, value := range values {
			out = append(out, ssr.FetchHeader{Name: lower, Value: value, Serialized: allowed(lower, value)})
		}
		if len(values) > 1 {
			joined := strings.Join(values, ", ")
			out = append(out, ssr.FetchHeader{Name: lower, Value: joined, Serialized: allowed(lower, joined)})
		}
	}
	return out
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
