package skgo

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
)

// PrerenderServiceOptions installs the application's ordinary request boundary
// in the build service. Endpoints answer Go-backed event.fetch calls.
type PrerenderServiceOptions struct {
	BindRequest func(http.Handler) http.Handler
	Endpoints   []*Endpoint
	Matchers    map[string]ParamMatcher
}

type prerenderRequestKey struct{}
type prerenderMatchKey struct{}
type prerenderManifestKey struct{}
type prerenderRefusalKey struct{}
type prerenderBodyFailureKey struct{}
type prerenderRequestInput struct {
	URL          string          `json:"url"`
	LogicalURL   string          `json:"logicalUrl"`
	Method       string          `json:"method"`
	Headers      http.Header     `json:"headers"`
	Body         []byte          `json:"body"`
	RouteID      string          `json:"routeId"`
	RoutePattern string          `json:"routePattern"`
	RouteParams  []ManifestParam `json:"routeParams"`
	RoutePath    string          `json:"routePath"`
	IsSubRequest bool            `json:"isSubRequest"`
	Manifest     Manifest        `json:"manifest"`
}
type prerenderResponse struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Body    []byte      `json:"body"`
}
type prerenderRequestAnswer struct {
	Handle   string             `json:"handle,omitempty"`
	Resolve  bool               `json:"resolve,omitempty"`
	Response *prerenderResponse `json:"response,omitempty"`
	Redirect *prerenderRedirect `json:"redirect,omitempty"`
	Error    *HTTPError         `json:"error,omitempty"`
	Headers  http.Header        `json:"headers,omitempty"`
	Cookies  []prerenderCookie  `json:"cookies,omitempty"`
}
type prerenderRequest struct {
	input     prerenderRequestInput
	endpoints *Endpoints
	cancel    context.CancelFunc
	ready     chan *http.Request
	response  chan prerenderResponse
	done      chan struct{}
	request   *http.Request
	answer    prerenderRequestAnswer
	mu        sync.Mutex
	active    sync.WaitGroup
	failure   error
	closing   bool
}
type prerenderRequests struct {
	mu      sync.Mutex
	entries map[string]*prerenderRequest
	ctx     context.Context
	options PrerenderServiceOptions
}

func (s *prerenderRequests) close() {
	s.mu.Lock()
	entries := s.entries
	s.entries = map[string]*prerenderRequest{}
	s.mu.Unlock()
	for _, entry := range entries {
		entry.cancel()
	}
	for _, entry := range entries {
		<-entry.done
		entry.active.Wait()
	}
}
func (s *prerenderRequests) acquire(handle string) (*prerenderRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.entries[handle]
	if entry == nil {
		return nil, fmt.Errorf("skgo: unknown prerender request %q", handle)
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if entry.closing || entry.request == nil {
		return nil, fmt.Errorf("skgo: prerender request is complete")
	}
	entry.active.Add(1)
	return entry, nil
}
func (s *prerenderRequests) end(handle string) error {
	s.mu.Lock()
	entry := s.entries[handle]
	delete(s.entries, handle)
	s.mu.Unlock()
	if entry == nil {
		return fmt.Errorf("skgo: unknown prerender request %q", handle)
	}
	entry.mu.Lock()
	entry.closing = true
	entry.mu.Unlock()
	entry.cancel()
	<-entry.done
	entry.active.Wait()
	return nil
}
func (s *prerenderRequests) begin(ctx context.Context, input prerenderRequestInput) (prerenderRequestAnswer, error) {
	if s.options.BindRequest == nil {
		return prerenderRequestAnswer{}, fmt.Errorf("skgo: prerender service requires BindRequest")
	}
	logical, cancel := context.WithCancel(s.ctx)
	entry := &prerenderRequest{input: input, cancel: cancel, ready: make(chan *http.Request, 1), response: make(chan prerenderResponse, 1), done: make(chan struct{})}
	logical = context.WithValue(logical, prerenderBodyFailureKey{}, func(err error) { entry.failure = err })
	logical = context.WithValue(logical, prerenderMatchKey{}, input)
	logical = context.WithValue(logical, prerenderManifestKey{}, input.Manifest)
	logical = context.WithValue(logical, prerenderRefusalKey{}, func(err error, state *hookState) {
		if asRedirect(err) != nil {
			entry.answer.Cookies = prerenderCookies(state.jar)
		}
		if redirect := asRedirect(err); redirect != nil {
			entry.answer.Redirect = &prerenderRedirect{Status: redirect.status(), Location: redirect.Location}
		} else {
			entry.answer.Error = asHTTPError(err)
		}
	})
	request, err := http.NewRequestWithContext(logical, input.Method, input.URL, bytes.NewReader(input.Body))
	if err != nil {
		cancel()
		return prerenderRequestAnswer{}, err
	}
	if input.IsSubRequest {
		request = request.WithContext(withFetchDepth(request.Context(), 1))
	}
	request.Header = http.Header{}
	for name, values := range input.Headers {
		request.Header[http.CanonicalHeaderKey(name)] = append([]string(nil), values...)
	}
	logicalURL := input.LogicalURL
	if logicalURL == "" {
		logicalURL = input.URL
	}
	pageURL, err := url.Parse(logicalURL)
	if err != nil || pageURL.Host == "" {
		cancel()
		return prerenderRequestAnswer{}, fmt.Errorf("skgo: invalid logical request URL")
	}
	origin := pageURL.Scheme + "://" + pageURL.Host
	endpointConfig := input.Manifest.EndpointConfig(origin)
	endpointConfig.Matchers = s.options.Matchers
	endpoints, err := NewEndpoints(endpointConfig, s.options.Endpoints...)
	if err != nil {
		cancel()
		return prerenderRequestAnswer{}, err
	}
	entry.endpoints = endpoints
	var fetchHandler http.Handler
	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), prerenderManifestKey{}, input.Manifest)
		fetchHandler.ServeHTTP(w, r.WithContext(ctx))
	})
	fetchHandler = s.options.BindRequest(endpoints.Intercept(http.NotFoundHandler()))
	fetcher := &requestFetcher{origin: pageURL, base: normalizeFetchBase(input.Manifest.Base), handler: dispatch}
	request = request.WithContext(context.WithValue(request.Context(), requestFetcherKey{}, fetcher))

	token := make([]byte, 24)
	if _, err := rand.Read(token); err != nil {
		cancel()
		return prerenderRequestAnswer{}, err
	}
	handle := hex.EncodeToString(token)
	s.mu.Lock()
	s.entries[handle] = entry
	s.mu.Unlock()
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entry.ready <- r
		select {
		case response := <-entry.response:
			for k, v := range response.Headers {
				w.Header()[http.CanonicalHeaderKey(k)] = append([]string(nil), v...)
			}
			w.WriteHeader(response.Status)
			_, _ = w.Write(response.Body)
		case <-r.Context().Done():
			panic(http.ErrAbortHandler)
		}
	})
	go func() {
		defer close(entry.done)
		defer func() {
			if value := recover(); value != nil {
				entry.failure = fmt.Errorf("skgo: prerender response aborted: %v", value)
			}
		}()
		writer := httptest.NewRecorder()
		s.options.BindRequest(next).ServeHTTP(writer, request)
		if entry.answer.Error == nil && entry.answer.Redirect == nil {
			entry.answer.Response = &prerenderResponse{Status: writer.Code, Headers: writer.Header().Clone(), Body: writer.Body.Bytes()}
		}
	}()
	select {
	case r := <-entry.ready:
		entry.request = r
		return prerenderRequestAnswer{Handle: handle, Resolve: true, Headers: requestResponseHeaders(r).Clone(), Cookies: prerenderCookies(requestCookieJar(r, false))}, nil
	case <-entry.done:
		if entry.failure != nil {
			s.end(handle)
			return prerenderRequestAnswer{}, entry.failure
		}
		entry.answer.Handle = handle
		if state := hookStateOf(request.Context()); state != nil {
			entry.answer.Cookies = prerenderCookies(state.jar)
		}
		return entry.answer, nil
	case <-ctx.Done():
		s.end(handle)
		return prerenderRequestAnswer{}, ctx.Err()
	}
}
func prerenderCookies(jar *cookieJar) []prerenderCookie {
	var out []prerenderCookie
	jar.mu.Lock()
	defer jar.mu.Unlock()
	for _, key := range jar.order {
		stored := jar.written[key]
		c := stored.httpCookie()
		cookie := prerenderCookie{Name: c.Name, Value: c.Value, Path: c.Path, Domain: c.Domain, HTTPOnly: c.HttpOnly, Secure: c.Secure, SameSite: int(c.SameSite), Partitioned: c.Partitioned, Priority: stored.priority, Raw: stored.raw}
		if stored.hasMaxAge {
			age := stored.maxAge
			cookie.MaxAge = &age
		}
		if !stored.expires.IsZero() {
			expires := stored.expires
			cookie.Expires = &expires
		}
		out = append(out, cookie)
	}
	return out
}
func (s *prerenderRequests) operation(ctx context.Context, path string, raw []byte, out io.Writer) error {
	if path == "/begin" {
		var input prerenderRequestInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return err
		}
		answer, err := s.begin(ctx, input)
		if err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(answer)
	}
	var input struct {
		Handle   string            `json:"handle"`
		Response prerenderResponse `json:"response"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	if path == "/end" {
		if err := s.end(input.Handle); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(struct{}{})
	}
	entry, err := s.acquire(input.Handle)
	if err != nil {
		return err
	}
	defer entry.active.Done()
	if path != "/response" {
		return fmt.Errorf("skgo: unsupported prerender operation")
	}
	select {
	case entry.response <- input.Response:
	case <-ctx.Done():
		entry.cancel()
		return ctx.Err()
	}
	select {
	case <-entry.done:
		if entry.failure != nil {
			return entry.failure
		}
		return json.NewEncoder(out).Encode(entry.answer)
	case <-ctx.Done():
		entry.cancel()
		return ctx.Err()
	}
}

// A buffered transport must report body failures, rather than returning the
// bytes received before an error as a successful complete response.
type prerenderBody struct {
	io.ReadCloser
	failed func(error)
}

func (body *prerenderBody) Read(p []byte) (int, error) {
	n, err := body.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		body.failed(err)
	}
	return n, err
}
func (body *prerenderBody) Close() error {
	err := body.ReadCloser.Close()
	if err != nil {
		body.failed(err)
	}
	return err
}

func runPrerenderEndpoint(ctx context.Context, entry *prerenderRequest, out io.Writer) error {
	request := entry.request.WithContext(ctx)
	previousHeaders := requestResponseHeaders(request).Clone()
	writer := httptest.NewRecorder()
	entry.endpoints.Intercept(http.NotFoundHandler()).ServeHTTP(writer, request)
	headers := requestResponseHeaders(request).Clone()
	for name := range previousHeaders {
		delete(headers, name)
	}
	return json.NewEncoder(out).Encode(prerenderRequestAnswer{Response: &prerenderResponse{Status: writer.Code, Headers: writer.Header().Clone(), Body: writer.Body.Bytes()}, Headers: headers, Cookies: prerenderCookies(requestCookieJar(request, false))})
}

func runPrerenderCookies(ctx context.Context, raw []byte, out io.Writer) error {
	var input struct {
		URL     string   `json:"url"`
		Cookies []string `json:"cookies"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return err
	}
	u, err := url.Parse(input.URL)
	if err != nil || u.Host == "" {
		return fmt.Errorf("skgo: invalid internal fetch URL")
	}
	request, ok := ctx.Value(prerenderRequestKey{}).(*http.Request)
	if !ok {
		return fmt.Errorf("skgo: cookies need a logical request")
	}
	jar := requestCookieJar(request, false)
	fallback := "/"
	if i := strings.LastIndex(u.EscapedPath(), "/"); i > 0 {
		fallback = u.EscapedPath()[:i]
	}
	for _, header := range input.Cookies {
		cookie := parseSetCookie(header)
		if cookie.name == "" {
			continue
		}
		if cookie.path == "" {
			cookie.path = fallback
		}
		jar.setInternal(cookie)
	}
	return json.NewEncoder(out).Encode(struct{}{})
}
