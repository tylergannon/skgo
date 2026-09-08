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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/tylergannon/skgo/internal/ssr"
)

// fetchDispatch answers one render-time `event.fetch`. It is the Fetch host
// ssr.Engine.Render is given, so payload is a JSON-encoded ssr.FetchRequest
// and the reply is a JSON-encoded ssr.FetchAnswer.
func (s *SSR) fetchDispatch(payload []byte) ([]byte, error) {
	var req ssr.FetchRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return nil, fmt.Errorf("skgo: decoding a render-time fetch: %w", err)
	}
	answer := s.fetchAnswer(req)
	return json.Marshal(answer)
}

// fetchAnswer runs one render-time fetch against the app's own server-route
// registry, exactly as if the browser had asked for it — except that nothing
// here opens a connection. httptest.NewRecorder is a ResponseWriter that
// writes to memory; s.fetch.ServeHTTP is the same http.Handler a real request
// to this path would reach.
func (s *SSR) fetchAnswer(req ssr.FetchRequest) ssr.FetchAnswer {
	if s.fetch == nil {
		return ssr.FetchAnswer{Error: "skgo: no server route is registered to answer event.fetch during a render"}
	}

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if req.Body != "" {
		body = strings.NewReader(req.Body)
	}

	httpReq, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return ssr.FetchAnswer{Error: "Failed to fetch"}
	}
	for name, value := range req.Headers {
		httpReq.Header.Set(name, value)
	}

	rec := httptest.NewRecorder()
	s.fetch.ServeHTTP(rec, httpReq)

	headers := map[string]string{}
	for name := range rec.Header() {
		headers[name] = rec.Header().Get(name)
	}
	return ssr.FetchAnswer{Response: &ssr.FetchResponse{
		Status:  rec.Code,
		Headers: headers,
		Body:    rec.Body.String(),
	}}
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
