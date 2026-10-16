package skgo

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/skgo/internal/formdata"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

func tracingRecorder(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) })
	return recorder
}
func tracedHandler(m Manifest, next http.Handler) http.Handler {
	cfg := m.HandleConfig()
	cfg.Origin = "http://app.test"
	return Middleware(func(ctx context.Context, _ *Event, resolve Resolve) (*http.Response, error) { return resolve(ctx) }).Intercept(cfg, next)
}
func spanAttribute(span sdktrace.ReadOnlySpan, name string) any {
	for _, attr := range span.Attributes() {
		if string(attr.Key) == name {
			return attr.Value.AsInterface()
		}
	}
	return nil
}
func namedSpan(t *testing.T, recorder *tracetest.SpanRecorder, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	var found sdktrace.ReadOnlySpan
	for _, span := range recorder.Ended() {
		if span.Name() == name {
			if found != nil {
				t.Fatalf("duplicate span %s", name)
			}
			found = span
		}
	}
	if found == nil {
		var names []string
		for _, s := range recorder.Ended() {
			names = append(names, s.Name())
		}
		t.Fatalf("missing span %s; got %v", name, names)
	}
	return found
}
func tracingEvent(t *testing.T, ctx context.Context) {
	t.Helper()
	event := EventFrom(ctx)
	if event == nil || !event.Tracing.Enabled {
		t.Fatal("missing enabled tracing event")
	}
	current := trace.SpanFromContext(ctx)
	if current.SpanContext().SpanID() != event.Tracing.Current.SpanContext().SpanID() {
		t.Fatal("context/current span mismatch")
	}
	event.Tracing.Root.SetAttributes(attribute.String("receipt.root", "ROOT_7819"))
	event.Tracing.Current.SetAttributes(attribute.String("receipt.current", "CURRENT_4361"))
	_, child := otel.Tracer("application").Start(ctx, "application.work")
	child.SetAttributes(attribute.String("receipt.child", "CHILD_2307"))
	child.End()
}
func TestTracingFrameworkOperations(t *testing.T) {
	t.Run("server load", func(t *testing.T) {
		recorder := tracingRecorder(t)
		m := section()
		m.TracingServer = true
		ls := mustLoads(t, nil, NewServerLoad(LoadSpec{Module: "src/routes/a/+page.server.ts", Run: func(ctx context.Context) (any, error) {
			tracingEvent(t, ctx)
			return map[string]any{"receipt": "LOAD_1937"}, nil
		}}))
		response := get(t, tracedHandler(m, ls.Intercept(http.NotFoundHandler())), "http://app.test/a/__data.json")
		if response.Code != 200 || !strings.Contains(response.Body.String(), "LOAD_1937") {
			t.Fatal(response.Code, response.Body.String())
		}
		root := namedSpan(t, recorder, "sveltekit.handle.root")
		resolve := namedSpan(t, recorder, "sveltekit.resolve")
		load := namedSpan(t, recorder, "sveltekit.load")
		child := namedSpan(t, recorder, "application.work")
		if len(recorder.Ended()) != 4 || resolve.Parent().SpanID() != root.SpanContext().SpanID() || load.Parent().SpanID() != resolve.SpanContext().SpanID() || child.Parent().SpanID() != load.SpanContext().SpanID() {
			t.Fatal("incorrect load trace hierarchy", recorder.Ended())
		}
		if spanAttribute(root, "receipt.root") != "ROOT_7819" || spanAttribute(load, "receipt.current") != "CURRENT_4361" || spanAttribute(load, "sveltekit.load.node_id") != "src/routes/a/+page.server.ts" || spanAttribute(load, "sveltekit.load.environment") != "server" || spanAttribute(load, "sveltekit.load.node_type") != "page" || spanAttribute(root, "sveltekit.is_data_request") != true {
			t.Fatal("missing literal Kit attributes")
		}
		for _, span := range recorder.Ended() {
			if span.SpanKind() != trace.SpanKindInternal {
				t.Fatalf("%s kind %v", span.Name(), span.SpanKind())
			}
		}
	})
	t.Run("endpoint methods", func(t *testing.T) {
		for _, method := range []string{"GET", "HEAD", "QUERY", "PATCH"} {
			t.Run(method, func(t *testing.T) {
				recorder := tracingRecorder(t)
				cfg := endpointFixture(nil, []string{"GET", "QUERY", "*"}, nil)
				endpoints, err := NewEndpoints(cfg, NewEndpoint("/api/thing", "GET", func(w http.ResponseWriter, r *http.Request) {
					tracingEvent(t, r.Context())
					io.WriteString(w, "GET_2943")
				}), NewEndpoint("/api/thing", "QUERY", func(w http.ResponseWriter, r *http.Request) {
					tracingEvent(t, r.Context())
					io.WriteString(w, "QUERY_2943")
				}), NewEndpoint("/api/thing", "*", func(w http.ResponseWriter, r *http.Request) {
					tracingEvent(t, r.Context())
					io.WriteString(w, "FALLBACK_2943")
				}))
				if err != nil {
					t.Fatal(err)
				}
				m := Manifest{TracingServer: true, Routes: cfg.Routes}
				response := httptest.NewRecorder()
				r := httptest.NewRequest(method, "http://app.test/api/thing", nil)
				r.Header.Set("Content-Type", "application/json")
				tracedHandler(m, endpoints.Intercept(http.NotFoundHandler())).ServeHTTP(response, r)
				if response.Code != 200 {
					t.Fatal(response.Code, response.Body.String())
				}
				endpoint := namedSpan(t, recorder, "sveltekit.endpoint")
				child := namedSpan(t, recorder, "application.work")
				if spanAttribute(endpoint, "http.method") != method || spanAttribute(endpoint, "http.route") != "/api/thing" || child.Parent().SpanID() != endpoint.SpanContext().SpanID() {
					t.Fatal("endpoint attributes/ancestry")
				}
			})
		}
	})
	t.Run("remote variants and direct calls", func(t *testing.T) {
		for _, kind := range []Kind{KindQuery, KindCommand, KindBatch, KindLive, KindPrerender} {
			t.Run(remoteTraceKind(kind), func(t *testing.T) {
				recorder := tracingRecorder(t)
				nested := NewRemote(RemoteSpec{Module: "src/lib/nested.remote.ts", Name: "nested", Kind: KindQuery, Call: func(ctx context.Context, _ Call) (any, error) { tracingEvent(t, ctx); return "NESTED_2841", nil }})
				fn := NewRemote(RemoteSpec{Module: "src/lib/trace.remote.ts", Name: "value", Kind: kind, Call: func(ctx context.Context, _ Call) (any, error) { return nested.call(ctx, Call{}) }, Batch: func(ctx context.Context, _ []Call) ([]any, error) {
					value, err := nested.call(ctx, Call{})
					return []any{value}, err
				}, Live: func(ctx context.Context, _ Call, yield func(any) error) error {
					value, err := nested.call(ctx, Call{})
					if err != nil {
						return err
					}
					return yield(value)
				}})
				rs, err := NewRemotes(RemoteConfig{Development: true}, fn)
				if err != nil {
					t.Fatal(err)
				}
				target := "http://app.test" + rs.Prefix() + fn.ID()
				method := "GET"
				var body io.Reader
				switch kind {
				case KindCommand:
					method = "POST"
					body = strings.NewReader("null")
				case KindBatch:
					method = "POST"
					body = strings.NewReader(`{"payloads":[""]}`)
				case KindForm:
					t.Fatal("form fixtures use their binary envelope test")
				}
				r := httptest.NewRequest(method, target, body)
				if method == "POST" {
					r.Header.Set("Content-Type", "application/json")
				}
				response := httptest.NewRecorder()
				tracedHandler(Manifest{TracingServer: true}, rs).ServeHTTP(response, r)
				if response.Code != 200 || !strings.Contains(response.Body.String(), "NESTED_2841") {
					t.Fatal(response.Code, response.Body.String())
				}
				remote := namedSpan(t, recorder, "sveltekit.remote.call")
				child := namedSpan(t, recorder, "application.work")
				if len(recorder.Ended()) != 4 || spanAttribute(remote, "sveltekit.remote.call.type") != remoteTraceKind(kind) || spanAttribute(remote, "sveltekit.remote.call.name") != "value" || child.Parent().SpanID() != remote.SpanContext().SpanID() {
					t.Fatal("remote trace scopes/ancestry")
				}
			})
		}
	})
}

func TestTracingRemoteFormsAndRefresh(t *testing.T) {
	t.Run("enhanced form and requested refresh", func(t *testing.T) {
		recorder := tracingRecorder(t)
		read := func(ctx context.Context) (string, error) {
			_, span := otel.Tracer("application").Start(ctx, "application.refresh")
			span.End()
			return "REFRESHED_5210", nil
		}
		query := NewQueryNoArg(testModule, "getMessages", read)
		form := NewForm(testModule, "sendMessage", func(ctx context.Context, in draft) (receipt, error) {
			tracingEvent(t, ctx)
			if in.From != "Ada" || in.Body != "hello there" {
				return receipt{}, errors.New("lost literal form input")
			}
			return receipt{ID: "FORM_4912"}, RefreshRequestedNoArg(ctx, read)
		})
		rs := testRemotes(t, RemoteConfig{}, query, form)
		raw, err := base64.StdEncoding.DecodeString(formGoldenSubmission)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "http://app.test"+rs.Prefix()+form.ID(), strings.NewReader(string(raw)))
		request.Header.Set("Content-Type", formdata.ContentType)
		response := httptest.NewRecorder()
		tracedHandler(Manifest{TracingServer: true}, rs).ServeHTTP(response, request)
		if response.Code != 200 || !strings.Contains(response.Body.String(), "FORM_4912") || !strings.Contains(response.Body.String(), "REFRESHED_5210") {
			t.Fatal(response.Code, response.Body.String())
		}
		operation := namedSpan(t, recorder, "sveltekit.remote.call")
		if spanAttribute(operation, "sveltekit.remote.call.type") != "form" || namedSpan(t, recorder, "application.refresh").Parent().SpanID() != operation.SpanContext().SpanID() || len(recorder.Ended()) != 5 {
			t.Fatal("form/refresh tracing scope", recorder.Ended())
		}
	})
	t.Run("unenhanced form", func(t *testing.T) {
		recorder := tracingRecorder(t)
		form := NewForm(testModule, "sendMessage", func(ctx context.Context, _ draft) (receipt, error) {
			tracingEvent(t, ctx)
			return receipt{ID: "FORM_NATIVE_4912"}, nil
		})
		rs := testRemotes(t, RemoteConfig{}, form)
		s := &SSR{loads: &Loads{}, remotes: rs}
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			value, redirect, err := s.runFormAction(r, actionID(r.URL))
			if redirect != nil || err != nil {
				t.Error(redirect, err)
				http.Error(w, "failed", 500)
				return
			}
			if value == nil {
				t.Error("missing form result")
				return
			}
			io.WriteString(w, "FORM_NATIVE_4912")
		})
		body := "from/" + form.ID() + "=Ada&body/" + form.ID() + "=hello"
		request := httptest.NewRequest("POST", "http://app.test/a?/remote="+form.ID(), strings.NewReader(body))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		tracedHandler(Manifest{TracingServer: true}, handler).ServeHTTP(response, request)
		if response.Code != 200 || response.Body.String() != "FORM_NATIVE_4912" {
			t.Fatal(response.Code, response.Body.String())
		}
		operation := namedSpan(t, recorder, "sveltekit.remote.form.post")
		if spanAttribute(operation, "sveltekit.remote.form.post.id") != form.ID() || namedSpan(t, recorder, "application.work").Parent().SpanID() != operation.SpanContext().SpanID() || len(recorder.Ended()) != 4 {
			t.Fatal("native form scope", recorder.Ended())
		}
	})
}

func TestTracingEndsAtResponseConstruction(t *testing.T) {
	recorder := tracingRecorder(t)
	cfg := endpointFixture(nil, []string{"GET"}, nil)
	blocked, release := make(chan struct{}), make(chan struct{})
	endpoints, err := NewEndpoints(cfg, NewEndpoint("/api/thing", "GET", func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "STREAM_5192")
		close(blocked)
		<-release
		io.WriteString(w, "END_6134")
	}))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(tracedHandler(Manifest{TracingServer: true, Routes: cfg.Routes}, endpoints.Intercept(http.NotFoundHandler())))
	defer server.Close()
	requestDone := make(chan error, 1)
	go func() {
		res, err := http.Get(server.URL + "/api/thing")
		if err == nil {
			body, readErr := io.ReadAll(res.Body)
			res.Body.Close()
			err = readErr
			if string(body) != "STREAM_5192END_6134" {
				err = fmt.Errorf("stream %s", body)
			}
		}
		requestDone <- err
	}()
	<-blocked
	// All framework scopes have produced their Response while the producer is
	// still blocked. No deadline or body EOF can be mistaken for span completion.
	deadline := time.After(3 * time.Second)
	for {
		if len(recorder.Ended()) == 3 {
			break
		}
		select {
		case <-deadline:
			close(release)
			t.Fatal("spans stayed open for streaming body")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	root := namedSpan(t, recorder, "sveltekit.handle.root")
	resolve := namedSpan(t, recorder, "sveltekit.resolve")
	endpoint := namedSpan(t, recorder, "sveltekit.endpoint")
	if resolve.Parent().SpanID() != root.SpanContext().SpanID() || endpoint.Parent().SpanID() != resolve.SpanContext().SpanID() {
		t.Fatal("stream ancestry")
	}
	close(release)
	if err := <-requestDone; err != nil {
		t.Fatal(err)
	}
}

func TestTracingErrorsAndDisabled(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		kind   string
		status codes.Code
	}{
		{"known client", Errorf(418, "TEAPOT_781"), "known_error", codes.Unset},
		{"known server", Errorf(503, "UNAVAILABLE_781"), "known_error", codes.Error},
		{"redirect", &Redirect{Status: 303, Location: "/elsewhere"}, "redirect", codes.Unset},
		{"unexpected", errors.New("UNEXPECTED_781"), "unknown_error", codes.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracingRecorder(t)
			m := section()
			m.TracingServer = true
			ls := mustLoads(t, nil, NewServerLoad(LoadSpec{Module: "src/routes/a/+page.server.ts", Run: func(context.Context) (any, error) { return nil, tc.err }}))
			response := get(t, tracedHandler(m, ls.Intercept(http.NotFoundHandler())), "http://app.test/a/__data.json")
			if response.Code != 200 {
				t.Fatal(response.Code)
			}
			span := namedSpan(t, recorder, "sveltekit.load")
			if spanAttribute(span, "sveltekit.load.result.type") != tc.kind || span.Status().Code != tc.status {
				t.Fatal(span.Attributes(), span.Status())
			}
			if (len(span.Events()) > 0) != (tc.status == codes.Error) {
				t.Fatal("exception classification", span.Events())
			}
		})
	}
	t.Run("panic before response", func(t *testing.T) {
		recorder := tracingRecorder(t)
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("PANIC_6448") })
		cfg := Manifest{TracingServer: true}.HandleConfig()
		cfg.OnPanic = func(string, any, []byte) {}
		hook := func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error) { return resolve(ctx) }
		handler := Sequence(hook).Intercept(cfg, next)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", "http://app.test/panic", nil))
		if w.Code != 500 {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, name := range []string{"sveltekit.handle.root", "sveltekit.handle.sequenced.0", "sveltekit.resolve"} {
			span := namedSpan(t, recorder, name)
			if spanAttribute(span, name+".result.type") != "unknown_error" || span.Status().Code != codes.Error {
				t.Fatal(name, span.Attributes(), span.Status())
			}
		}
		if len(recorder.Ended()) != 3 {
			t.Fatal("panic leaked span", recorder.Ended())
		}
	})

	t.Run("disabled", func(t *testing.T) {
		recorder := tracingRecorder(t)
		m := section()
		ls := mustLoads(t, nil, NewServerLoad(LoadSpec{Module: "src/routes/a/+page.server.ts", Run: func(ctx context.Context) (any, error) {
			tracing := EventFrom(ctx).Tracing
			if tracing.Enabled || tracing.Root == nil || tracing.Current == nil || tracing.Current.IsRecording() {
				t.Fatal("unsafe disabled tracing")
			}
			tracing.Root.SetAttributes(attribute.String("safe", "yes"))
			tracing.Current.AddEvent("safe")
			return map[string]any{"receipt": "DISABLED_9342"}, nil
		}}))
		response := get(t, tracedHandler(m, ls.Intercept(http.NotFoundHandler())), "http://app.test/a/__data.json")
		if !strings.Contains(response.Body.String(), "DISABLED_9342") || len(recorder.Ended()) != 0 {
			t.Fatal(response.Body.String(), recorder.Ended())
		}
	})
}

func TestTracingIncomingAndConcurrentContext(t *testing.T) {
	recorder := tracingRecorder(t)
	m := section()
	m.TracingServer = true
	ls := mustLoads(t, nil, NewServerLoad(LoadSpec{Module: "src/routes/a/+page.server.ts", Run: func(ctx context.Context) (any, error) {
		tracing := EventFrom(ctx).Tracing
		tracing.Root.SetAttributes(attribute.String("receipt.request", EventFrom(ctx).Request().Header.Get("X-Receipt")))
		if tracing.Current.SpanContext().TraceID() != tracing.Root.SpanContext().TraceID() {
			return nil, errors.New("cross-request tracing")
		}
		return map[string]any{"receipt": "CONCURRENT_1492"}, nil
	}}))
	handler := tracedHandler(m, ls.Intercept(http.NotFoundHandler()))
	var wg sync.WaitGroup
	for i := 1; i <= 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest("GET", "http://app.test/a/__data.json", nil)
			r.Header.Set("Traceparent", fmt.Sprintf("00-%032x-%016x-01", i, 100+i))
			r.Header.Set("Tracestate", "vendor=literal")
			r.Header.Set("X-Receipt", fmt.Sprint(i))
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			if !strings.Contains(w.Body.String(), "CONCURRENT_1492") {
				t.Error(w.Code, w.Body.String())
			}
		}(i)
	}
	wg.Wait()
	roots := 0
	for _, span := range recorder.Ended() {
		if span.Name() != "sveltekit.handle.root" {
			continue
		}
		roots++
		receipt := spanAttribute(span, "receipt.request")
		if span.SpanContext().TraceID().String() != fmt.Sprintf("%032x", func() int { i, _ := strconv.Atoi(receipt.(string)); return i }()) {
			t.Error("request identity mismatch", receipt, span.SpanContext())
		}
		if !span.Parent().IsRemote() || span.SpanContext().TraceState().String() != "vendor=literal" {
			t.Error("lost W3C parent/state", span)
		}
	}
	if roots != 12 || len(recorder.Ended()) != 36 {
		t.Fatal("trace count", roots, len(recorder.Ended()))
	}
}

func TestTracingActionsAndSequencedHooks(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"success", nil, ""}, {"failure", Fail(422, map[string]any{"field": "INVALID_711"}), "failure"}, {"redirect", &Redirect{Status: 303, Location: "/saved"}, "redirect"}, {"error", errors.New("ACTION_ERROR_711"), "unknown_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracingRecorder(t)
			m := section()
			m.TracingServer = true
			ls := mustLoads(t, nil)
			actions, err := NewActions(NewPageAction(ActionSpec{Module: "src/routes/a/+page.server.ts", Name: "save", Run: func(ctx context.Context) (any, error) {
				tracingEvent(t, ctx)
				return map[string]any{"receipt": "ACTION_711"}, tc.err
			}}))
			if err != nil {
				t.Fatal(err)
			}
			s := streamer(nil)
			s.loads = ls
			s.actions = actions
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !s.serve(w, r, r.URL.Path) {
					t.Error("action route not dispatched")
				}
			})
			hook := func(ctx context.Context, event *Event, resolve Resolve) (*http.Response, error) {
				if !event.Tracing.Enabled {
					t.Error("hook tracing disabled")
				}
				return resolve(ctx)
			}
			cfg := m.HandleConfig()
			cfg.Origin = "http://app.test"
			app := Sequence(hook, hook).Intercept(cfg, handler)
			r := httptest.NewRequest("POST", "http://app.test/a?/save", strings.NewReader("value=literal"))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Set("Accept", "application/json")
			r.Header.Set("X-Sveltekit-Action", "true")
			w := httptest.NewRecorder()
			app.ServeHTTP(w, r)
			action := namedSpan(t, recorder, "sveltekit.form_action")
			if spanAttribute(action, "sveltekit.form_action.name") != "save" || spanAttribute(action, "sveltekit.form_action.result.type") != func() any {
				if tc.want == "" {
					return nil
				}
				return tc.want
			}() {
				t.Fatal(action.Attributes(), w.Code, w.Body.String())
			}
			outer := namedSpan(t, recorder, "sveltekit.handle.sequenced.0")
			inner := namedSpan(t, recorder, "sveltekit.handle.sequenced.1")
			resolve := namedSpan(t, recorder, "sveltekit.resolve")
			if inner.Parent().SpanID() != outer.SpanContext().SpanID() || resolve.Parent().SpanID() != inner.SpanContext().SpanID() || action.Parent().SpanID() != resolve.SpanContext().SpanID() {
				t.Fatal("sequence parentage")
			}
		})
	}
}

func TestTracingInternalGoFetch(t *testing.T) {
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprint(override), func(t *testing.T) {
			recorder := tracingRecorder(t)
			m := section()
			m.TracingServer = true
			for i := range m.Routes {
				if m.Routes[i].ID == "/api" {
					m.Routes[i].Endpoint = &ManifestEndpoint{Methods: []string{"GET"}}
				}
			}
			type privateKey struct{}
			load := NewServerLoad(LoadSpec{Module: "src/routes/a/+page.server.ts", Run: func(ctx context.Context) (any, error) {
				request, _ := http.NewRequest("GET", "http://127.0.0.1:8080/api", nil)
				if override {
					request.Header.Set("Traceparent", "00-33333333333333333333333333333333-4444444444444444-01")
				}
				response, err := EventFrom(ctx).Fetch(ctx, request)
				if err != nil {
					return nil, err
				}
				defer response.Body.Close()
				raw, err := io.ReadAll(response.Body)
				return map[string]any{"receipt": string(raw)}, err
			}})
			loads := mustLoads(t, nil, load)
			endpoints, err := NewEndpoints(m.EndpointConfig("http://127.0.0.1:8080"), NewEndpoint("/api", "GET", func(w http.ResponseWriter, r *http.Request) {
				if r.Context().Value(privateKey{}) != nil {
					t.Error("internal fetch leaked originating request values")
				}
				io.WriteString(w, "GO_FETCH_7138")
			}))
			if err != nil {
				t.Fatal(err)
			}
			cfg := m.HandleConfig()
			cfg.Origin = "http://127.0.0.1:8080"
			var application http.Handler
			inner := Middleware(func(ctx context.Context, _ *Event, resolve Resolve) (*http.Response, error) { return resolve(ctx) }).Intercept(cfg, loads.Intercept(endpoints.Intercept(http.NotFoundHandler())))
			application = FetchConfig{Origin: "http://127.0.0.1:8080", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { application.ServeHTTP(w, r) })}.Intercept(inner)
			request := httptest.NewRequest("GET", "http://127.0.0.1:8080/a/__data.json", nil)
			request.Header.Set("Traceparent", "00-11111111111111111111111111111111-2222222222222222-01")
			request = request.WithContext(context.WithValue(request.Context(), privateKey{}, "must-not-cross"))
			response := httptest.NewRecorder()
			application.ServeHTTP(response, request)
			if response.Code != 200 || !strings.Contains(response.Body.String(), "GO_FETCH_7138") {
				t.Fatal(response.Code, response.Body.String())
			}
			loadSpan := namedSpan(t, recorder, "sveltekit.load")
			var nested sdktrace.ReadOnlySpan
			for _, span := range recorder.Ended() {
				if span.Name() == "sveltekit.handle.root" && spanAttribute(span, "http.route") == "/api" {
					nested = span
				}
			}
			if nested == nil {
				t.Fatal("nested request was not traced")
			}
			if override {
				if nested.SpanContext().TraceID().String() != "33333333333333333333333333333333" || nested.Parent().SpanID().String() != "4444444444444444" {
					t.Fatal("explicit W3C header failed to override", nested)
				}
			} else if nested.SpanContext().TraceID() != loadSpan.SpanContext().TraceID() || nested.Parent().SpanID() != loadSpan.SpanContext().SpanID() {
				t.Fatal("Go fetch lost active parent", nested)
			}
		})
	}
}
