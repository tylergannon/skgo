package skgo

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"

	"github.com/tylergannon/devalue/v5"
)

type devContinuationKey struct{}
type devEndpointsKey struct{}
type devRendererKey struct{}
type devStreamingLoadKey struct{}

// NativeDevelopment carries development-only rendering through the application's
// ordinary constructor. Its configured Go registries and request middleware are
// retained; Kit, rather than an embedded engine, supplies the page response.
type NativeDevelopment struct {
	fs.FS
	renderer     *SSR
	manifest     Manifest
	secret       string
	kitTransport *http.Transport
}

func NewNativeDevelopment(build fs.FS, secret string, kitSocket ...string) *NativeDevelopment {
	d := &NativeDevelopment{FS: build, secret: secret}
	if len(kitSocket) > 0 {
		d.kitTransport = nativeKitTransport(kitSocket[0])
	}
	return d
}

// Handler mounts private callbacks beside the application's browser remote
// handler. Closing it cancels its own requests; handles have no meaning in any
// replacement process and are never retried there.
func (d *NativeDevelopment) Handler(app http.Handler) (http.Handler, func(), error) {
	s := d.renderer
	if s == nil || s.loads == nil || s.remotes == nil {
		return nil, nil, fmt.Errorf("skgo: native development requires the application's SSR registries")
	}
	ctx, cancel := context.WithCancel(context.Background())
	loads := make([]*ServerLoad, 0, len(s.loads.byModule))
	for _, load := range s.loads.byModule {
		loads = append(loads, load)
	}
	remotes := make([]*Remote, 0, len(s.remotes.fns))
	for _, remote := range s.remotes.fns {
		remotes = append(remotes, remote)
	}
	sessions := &prerenderRequests{ctx: ctx, development: true, entries: map[string]*prerenderRequest{}, options: PrerenderServiceOptions{
		BindRequest: func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ctx := context.WithValue(r.Context(), devKitTransportKey{}, d)
				app.ServeHTTP(w, r.WithContext(context.WithValue(ctx, devContinuationKey{}, next)))
			})
		},
		Matchers: s.loads.cfg.Matchers, FilterSerializedResponseHeaders: s.filterSerializedResponseHeaders,
	}}
	exchanges := &devFetchRequests{}
	callbacks := prerenderHandler(d.secret, s.loads.cfg.Transport, loads, remotes, nil, sessions)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/_skgo/dev/") {
			if address := r.Header.Get("X-Skgo-Client-Address"); address != "" {
				r = r.Clone(r.Context())
				r.RemoteAddr = address
			}
			app.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), devKitTransportKey{}, d)))
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+d.secret)) != 1 {
			http.Error(w, "Unauthorized", 401)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", 405)
			return
		}
		path := "/" + strings.TrimPrefix(r.URL.Path, "/_skgo/dev/")
		if path == "/fetch-begin" || path == "/fetch-response" || path == "/fetch-end" {
			d.fetch(w, r, sessions, exchanges, path)
			return
		}
		if path == "/response" {
			d.response(w, r, sessions)
			return
		}
		r = r.Clone(context.WithValue(r.Context(), devRendererKey{}, s))
		r.URL.Path = path
		if path == "/begin" {
			var fields map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&fields); err != nil {
				http.Error(w, "invalid begin: "+err.Error(), 400)
				return
			}
			// Kit3 matcher outcomes may be typed values. Go independently
			// reconstructs raw and converted parameters from its pinned route.
			delete(fields, "params")
			rawInput, _ := json.Marshal(fields)
			var input prerenderRequestInput
			if err := json.Unmarshal(rawInput, &input); err != nil {
				http.Error(w, "invalid begin: "+err.Error(), 400)
				return
			}

			for _, route := range d.manifest.Routes {
				if route.ID == input.RouteID {
					logical := input.LogicalURL
					if logical == "" {
						logical = input.URL
					}
					pageURL, err := url.Parse(logical)
					if err != nil {
						http.Error(w, "invalid logical URL", 400)
						return
					}
					routePath, ok := requestRoutingPath(pageURL)
					if !ok {
						http.Error(w, "invalid route path", 400)
						return
					}
					routePath = strings.TrimPrefix(routePath, d.manifest.Base)
					if routePath == "" {
						routePath = "/"
					}
					input.RoutePattern = route.Pattern
					input.RouteParams = route.Params
					input.RoutePath = routePath
					break
				}
			}
			if len(input.Manifest.Routes) == 0 {
				input.Manifest = d.manifest
			}
			raw, _ := json.Marshal(input)
			r.Body = io.NopCloser(bytes.NewReader(raw))
		}
		if path == "/load" || path == "/action" || path == "/endpoint" || path == "/remote" {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), 400)
				return
			}
			var identity struct {
				Handle string `json:"handle"`
			}
			if json.Unmarshal(raw, &identity) != nil {
				http.Error(w, "invalid callback", 400)
				return
			}
			entry, err := sessions.acquire(identity.Handle)
			if err != nil {
				http.Error(w, err.Error(), 409)
				return
			}
			defer entry.active.Done()
			ctx, cancel := context.WithCancel(entry.request.Context())
			stop := context.AfterFunc(r.Context(), func() {
				cancel()
				if path != "/remote" {
					entry.cancel()
				}
			})
			defer func() { stop(); cancel() }()
			ctx = context.WithValue(ctx, prerenderRequestKey{}, entry.request)
			ctx = context.WithValue(ctx, devRendererKey{}, s)
			if path == "/endpoint" {
				request := entry.request.WithContext(ctx)
				entry.endpoints.Intercept(http.NotFoundHandler()).ServeHTTP(w, request)
				return
			}
			if path == "/remote" {
				err = d.remote(ctx, raw, w)
			} else if path == "/action" {
				err = d.action(ctx, raw, w)
			} else {
				w.Header().Set("Content-Type", "application/x-ndjson")
				err = runPrerenderLoad(context.WithValue(ctx, devStreamingLoadKey{}, true), bytes.NewReader(raw), w, s.loads.cfg.Transport, loads)
			}
			if err != nil {
				if path == "/load" {
					json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				} else {
					http.Error(w, err.Error(), 500)
				}
			}
			return
		}
		callbacks.ServeHTTP(w, r)
	})
	return handler, func() {
		cancel()
		exchanges.close()
		sessions.close()
		if d.kitTransport != nil {
			d.kitTransport.CloseIdleConnections()
		}
	}, nil
}

func (d *NativeDevelopment) response(w http.ResponseWriter, r *http.Request, sessions *prerenderRequests) {
	_ = http.NewResponseController(w).EnableFullDuplex()
	entry, err := sessions.acquire(r.Header.Get("X-Skgo-Handle"))
	if err != nil {
		http.Error(w, err.Error(), 409)
		return
	}
	defer entry.active.Done()
	raw, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Skgo-Response"))
	if err != nil {
		http.Error(w, "invalid response metadata", 400)
		return
	}
	var response prerenderResponse
	if json.Unmarshal(raw, &response) != nil || response.Status < 200 || response.Status > 599 {
		http.Error(w, "invalid response metadata", 400)
		return
	}
	response.Reader = r.Body
	stop := context.AfterFunc(r.Context(), entry.cancel)
	defer stop()
	select {
	case entry.response <- response:
	case <-r.Context().Done():
		return
	case <-entry.request.Context().Done():
		http.Error(w, "request ended", 409)
		return
	}
	select {
	case <-entry.stream.committed:
	case <-r.Context().Done():
		return
	}
	result := entry.stream.response(entry.request)
	defer result.Body.Close()
	for name, values := range result.Header {
		w.Header()[name] = values
	}
	w.WriteHeader(result.StatusCode)
	flush := http.NewResponseController(w)
	_ = flush.Flush()
	buffer := make([]byte, 32*1024)
	for {
		n, err := result.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := w.Write(buffer[:n]); writeErr != nil {
				return
			}
			_ = flush.Flush()
		}
		if err != nil {
			if err != io.EOF {
				panic(http.ErrAbortHandler)
			}
			return
		}
	}
}

func (d *NativeDevelopment) action(ctx context.Context, raw []byte, out io.Writer) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("skgo: Go action panicked")
		}
	}()
	var input struct{ Module, Name string }
	if err = json.Unmarshal(raw, &input); err != nil {
		return err
	}
	action := d.renderer.actions.lookup(input.Module, input.Name)
	if action == nil {
		return fmt.Errorf("skgo: no Go action %s#%s", input.Module, input.Name)
	}
	request := ctx.Value(prerenderRequestKey{}).(*http.Request).WithContext(ctx)
	s := d.renderer
	event := s.remotes.newEvent(request, true)
	event.remote = false
	event.query = false
	if state := hookStateOf(ctx); state != nil {
		event.params = state.params
	}
	event.actionResponse = &loadRequest{responseState: newResponseState(request)}
	value, callErr := action.run(withEvent(ctx, event))
	answer := map[string]any{"type": "success", "status": 200, "headers": requestResponseHeaders(request), "cookies": prerenderCookies(requestCookieJar(request, false))}
	var failure *actionFailure
	switch {
	case errors.As(callErr, &failure):
		answer["type"] = "failure"
		answer["status"] = failure.status
		value = failure.data
	case asRedirect(callErr) != nil:
		redirect := asRedirect(callErr)
		answer["type"] = "redirect"
		answer["status"] = redirect.status()
		answer["location"] = redirect.Location
		return json.NewEncoder(out).Encode(answer)
	case callErr != nil:
		answer["type"] = "error"
		handled := s.loads.dataError(ctx, "", asHTTPError(callErr), callErr)
		body, encodeErr := jsonBytes(errorNodeExtra(handled))
		if encodeErr != nil {
			return encodeErr
		}
		answer["status"] = handled.Status
		answer["error"] = &HTTPError{Status: handled.Status, Message: handled.Message}
		answer["body"] = json.RawMessage(body)
		return json.NewEncoder(out).Encode(answer)
	}
	tree, err := s.loads.cfg.Transport.encodeLoadValue(value)
	if err != nil {
		return err
	}
	data, err := devalue.StringifyWith(tree, s.loads.cfg.Transport.reducers())
	if err != nil {
		return err
	}
	answer["data"] = data
	return json.NewEncoder(out).Encode(answer)
}
