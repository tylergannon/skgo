package skgo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"
)

func nativeFixture(t *testing.T, load *ServerLoad, actions ...*PageAction) (http.Handler, func(), *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	development := NewNativeDevelopment("secret")
	loads, err := NewLoads(LoadConfig{Origin: "http://app.test"}, load)
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	if err != nil {
		t.Fatal(err)
	}
	actionRegistry, err := NewActions(actions...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = development.NewSSR(fstest.MapFS{}, Manifest{}, loads, remotes, SSROptions{Actions: actionRegistry}); err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		calls.Add(1)
		event.Locals = &buildLocals{Value: "original-owner-713"}
		response, err := resolve(ctx, event)
		if err == nil {
			response.Header.Set("X-Go-After", "original-hook-901")
		}
		return response, err
	})
	app := FetchConfig{Origin: "http://app.test", Handler: pages}.Intercept(buildBoundary(hook)(loads.Intercept(remotes.Intercept(pages))))
	handler, close, err := development.Handler(app)
	if err != nil {
		t.Fatal(err)
	}
	return handler, close, calls
}
func beginNative(t *testing.T, h http.Handler) string {
	t.Helper()
	var begin prerenderRequestAnswer
	buildOperation(t, h, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/page", Method: "GET", Params: map[string]string{"name": "literal-param"}}, &begin)
	if !begin.Resolve || begin.Handle == "" {
		t.Fatalf("begin: %+v", begin)
	}
	return begin.Handle
}
func TestNativeDevelopmentOriginalHookContextAndStreamedResponse(t *testing.T) {
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		return map[string]any{"owner": RequestLocals[buildLocals](ctx).Value}, nil
	}})
	handler, close, calls := nativeFixture(t, load)
	defer close()
	handle := beginNative(t, handler)
	response := httptest.NewRecorder()
	raw := fmt.Sprintf(`{"handle":%q,"module":"page","url":"http://app.test/page"}`, handle)
	request := httptest.NewRequest("POST", "/_skgo/dev/load", strings.NewReader(raw))
	request.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"original-owner-713"`) {
		t.Fatal(response.Body.String())
	}
	server := httptest.NewServer(handler)
	defer server.Close()
	input, write := io.Pipe()
	request, _ = http.NewRequest("POST", server.URL+"/_skgo/dev/response", input)
	request.Header.Set("Authorization", "Bearer secret")
	request.Header.Set("X-Skgo-Handle", handle)
	metadata, _ := json.Marshal(prerenderResponse{Status: 201, Headers: http.Header{"X-Kit": {"literal-kit-321"}}})
	request.Header.Set("X-Skgo-Response", base64.StdEncoding.EncodeToString(metadata))
	got := make(chan *http.Response, 1)
	go func() {
		r, err := server.Client().Do(request)
		if err != nil {
			t.Error(err)
			return
		}
		got <- r
	}()
	write.Write([]byte("first-112\n"))
	var result *http.Response
	select {
	case result = <-got:
	case <-time.After(time.Second):
		t.Fatal("response waited for complete Kit body")
	}
	if result.StatusCode != 201 || result.Header.Get("X-Go-After") != "original-hook-901" || result.Header.Get("X-Kit") != "literal-kit-321" {
		t.Fatalf("response: %d %v", result.StatusCode, result.Header)
	}
	reader := bufio.NewReader(result.Body)
	line, err := reader.ReadString('\n')
	if err != nil || line != "first-112\n" {
		t.Fatalf("initial body %q %v", line, err)
	}
	write.Write([]byte("last-229"))
	write.Close()
	body, err := io.ReadAll(reader)
	result.Body.Close()
	if err != nil || string(body) != "last-229" {
		t.Fatalf("tail %q %v", body, err)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
	if calls.Load() != 1 {
		t.Fatalf("hooks ran %d times", calls.Load())
	}
}
func TestNativeDevelopmentDeferredCancellationAndOwnerReplacement(t *testing.T) {
	entered, cancelled := make(chan struct{}), make(chan struct{})
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		return map[string]any{"immediate": "literal-602", "later": Async(ctx, func(ctx context.Context) (string, error) {
			close(entered)
			<-ctx.Done()
			close(cancelled)
			return "", ctx.Err()
		})}, nil
	}})
	handler, close, _ := nativeFixture(t, load)
	defer close()
	handle := beginNative(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()
	raw := fmt.Sprintf(`{"handle":%q,"module":"page","url":"http://app.test/page"}`, handle)
	req, _ := http.NewRequest("POST", server.URL+"/_skgo/dev/load", strings.NewReader(raw))
	req.Header.Set("Authorization", "Bearer secret")
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	<-entered
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(first, "literal-602") || !strings.Contains(first, "Promise") {
		t.Fatalf("initial deferred frame %q %v", first, err)
	}
	// Replacement owns another map. It must reject this mutation identity rather
	// than run an old action with fresh locals.
	var mutations atomic.Int32
	replacement, closeReplacement, _ := nativeFixture(t, NewServerLoad(LoadSpec{Module: "page", Run: func(context.Context) (any, error) { return map[string]any{}, nil }}), NewPageAction(ActionSpec{Module: "src/routes/+page.server.ts", Name: "default", Run: func(context.Context) (any, error) { mutations.Add(1); return nil, nil }}))
	defer closeReplacement()
	old := httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/_skgo/dev/action", bytes.NewBufferString(fmt.Sprintf(`{"handle":%q,"module":"src/routes/+page.server.ts","name":"default"}`, handle)))
	req.Header.Set("Authorization", "Bearer secret")
	replacement.ServeHTTP(old, req)
	if old.Code != 409 || mutations.Load() != 0 {
		t.Fatalf("old handle status=%d mutations=%d", old.Code, mutations.Load())
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("request did not cancel deferred work")
	}
	second := beginNative(t, handler)
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": second}, nil)
}

func TestNativeDevelopmentActionResultsAndConfiguredErrorHook(t *testing.T) {
	module := "src/routes/+page.server.ts"
	for _, test := range []struct {
		name string
		run  func(context.Context) (any, error)
		want []string
	}{
		{"success", func(ctx context.Context) (any, error) {
			if RequestLocals[buildLocals](ctx).Value != "original-owner-713" {
				return nil, fmt.Errorf("locals lost")
			}
			return map[string]any{"receipt": "literal-778"}, nil
		}, []string{`"type":"success"`, `literal-778`}},
		{"failure", func(context.Context) (any, error) {
			return nil, Fail(422, map[string]any{"field": "literal-invalid-991"})
		}, []string{`"type":"failure"`, `"status":422`, `literal-invalid-991`}},
		{"redirect", func(context.Context) (any, error) { return nil, &Redirect{Status: 303, Location: "/literal-next-669"} }, []string{`"type":"redirect"`, `"status":303`, `"location":"/literal-next-669"`}},
	} {
		t.Run(test.name, func(t *testing.T) {
			handler, close, _ := nativeFixture(t, NewServerLoad(LoadSpec{Module: "page", Run: func(context.Context) (any, error) { return nil, nil }}), NewPageAction(ActionSpec{Module: module, Name: "submit", Run: test.run}))
			defer close()
			handle := beginNative(t, handler)
			var answer map[string]any
			buildOperation(t, handler, "/_skgo/dev/action", map[string]string{"handle": handle, "module": module, "name": "submit"}, &answer)
			raw, _ := json.Marshal(answer)
			for _, want := range test.want {
				if !strings.Contains(string(raw), want) {
					t.Fatalf("answer %s missing %s", raw, want)
				}
			}
			buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
		})
	}
	// A configured Go error hook, captured from the actual app renderer, retains
	// its App.Error fields instead of replacing them with a JS transport failure.
	// Build a second constructor explicitly to supply the application's config.
	development := NewNativeDevelopment("secret")
	loads, _ := NewLoads(LoadConfig{Origin: "http://app.test", HandleError: func(context.Context, CaughtError) map[string]any {
		return map[string]any{"message": "literal-public-834", "supportId": "literal-case-335"}
	}}, NewServerLoad(LoadSpec{Module: "page", Run: func(context.Context) (any, error) { return nil, fmt.Errorf("private-database-detail") }}))
	remotes, _ := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	development.NewSSR(fstest.MapFS{}, Manifest{}, loads, remotes, SSROptions{})
	pages, _ := development.NewStaticHandler(fstest.MapFS{})
	configured, closeConfigured, err := development.Handler(pages)
	if err != nil {
		t.Fatal(err)
	}
	defer closeConfigured()
	handle := beginNative(t, configured)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/_skgo/dev/load", strings.NewReader(fmt.Sprintf(`{"handle":%q,"module":"page","url":"http://app.test/page"}`, handle)))
	request.Header.Set("Authorization", "Bearer secret")
	configured.ServeHTTP(recorder, request)
	var configuredAnswer prerenderLoadOutput
	if err := json.Unmarshal(recorder.Body.Bytes(), &configuredAnswer); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(configuredAnswer.Body), `"supportId":"literal-case-335"`) || !strings.Contains(string(configuredAnswer.Body), `"message":"literal-public-834"`) || strings.Contains(string(configuredAnswer.Body), "private-database-detail") || configuredAnswer.NativeError == nil || configuredAnswer.NativeError.Message != "private-database-detail" {
		t.Fatal(recorder.Body.String())
	}
	buildOperation(t, configured, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeDevelopmentEndpointManifestUsesOriginalRegistry(t *testing.T) {
	manifest := Manifest{Remotes: []string{}, Loads: []string{}, Routes: []ManifestRoute{{ID: "/orders", Pattern: `^/orders/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}}}}
	development := NewNativeDevelopment("secret")
	loads, err := NewLoads(manifest.LoadConfig("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(manifest.RemoteConfig("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = development.NewSSR(fstest.MapFS{}, manifest, loads, remotes, SSROptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	endpoints, err := NewEndpoints(manifest.EndpointConfig("http://app.test"), NewEndpoint("/orders", "GET", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Original-Endpoint", "literal-718")
		io.WriteString(w, "original-orders-502")
	}))
	if err != nil {
		t.Fatal(err)
	}
	// Match the real generated-host fixture: its ordinary constructor declares
	// endpoint routes, but does not install an outer FetchConfig.
	app := endpoints.Intercept(pages)
	handler, close, err := development.Handler(app)
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	var begin prerenderRequestAnswer
	buildOperation(t, handler, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/orders", LogicalURL: "http://app.test/orders", Method: "GET", RouteID: "/orders"}, &begin)
	if !begin.Resolve {
		t.Fatalf("begin: %+v", begin)
	}
	request := httptest.NewRequest("POST", "/_skgo/dev/endpoint", strings.NewReader(fmt.Sprintf(`{"handle":%q}`, begin.Handle)))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || response.Body.String() != "original-orders-502" || response.Header().Get("X-Original-Endpoint") != "literal-718" {
		t.Fatalf("endpoint %d %v %s", response.Code, response.Header(), response.Body.String())
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": begin.Handle}, nil)
}

func TestNativeDevelopmentDeferredValueArrivesAfterInitialData(t *testing.T) {
	release := make(chan struct{})
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		return map[string]any{"now": "literal-now-121", "later": Async(ctx, func(ctx context.Context) (string, error) {
			select {
			case <-release:
				return "literal-later-934", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		})}, nil
	}})
	handler, cleanup, _ := nativeFixture(t, load)
	defer cleanup()
	handle := beginNative(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()
	request, _ := http.NewRequest("POST", server.URL+"/_skgo/dev/load", strings.NewReader(fmt.Sprintf(`{"handle":%q,"module":"page","url":"http://app.test/page"}`, handle)))
	request.Header.Set("Authorization", "Bearer secret")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(first, "literal-now-121") || strings.Contains(first, "literal-later-934") {
		t.Fatalf("initial %q %v", first, err)
	}
	close(release)
	next, err := reader.ReadString('\n')
	if err != nil || !strings.Contains(next, `"id":1`) || !strings.Contains(next, "literal-later-934") {
		t.Fatalf("deferred %q %v", next, err)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeDevelopmentDerivesTypedAndRawParamsFromOriginalURL(t *testing.T) {
	const module = "src/routes/typed/[n=Number]/+page.server.ts"
	manifest := Manifest{Remotes: []string{}, Loads: []string{module}, Nodes: []string{module}, Routes: []ManifestRoute{{ID: "/typed/[n=Number]", Pattern: `^/typed/([^/]+?)/?$`, Params: []ManifestParam{{Name: "n", Matcher: "Number"}}, Page: &ManifestPage{Leaf: 0}}}}
	matchers := map[string]ParamMatcher{"Number": func(raw string) (any, bool) {
		if raw == "00042" {
			return fixtureNumber(42), true
		}
		return nil, false
	}}
	load := NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
		event := EventFrom(ctx)
		return map[string]any{"typed": LoadParamValue[fixtureNumber](event, "n").Label(), "raw": event.Param("n")}, nil
	}})
	cfg := manifest.LoadConfig("http://app.test")
	cfg.Matchers = matchers
	loads, err := NewLoads(cfg, load)
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(manifest.RemoteConfig("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	development := NewNativeDevelopment("secret")
	development.NewSSR(fstest.MapFS{}, manifest, loads, remotes, SSROptions{})
	pages, _ := development.NewStaticHandler(fstest.MapFS{})
	handleCfg := manifest.HandleConfig()
	handleCfg.Origin = "http://app.test"
	handleCfg.Matchers = matchers
	handleCfg.Loads = loads
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		if event.Param("n") != "00042" {
			return nil, fmt.Errorf("hook lost raw matched parameter")
		}
		return resolve(ctx, event)
	})
	handler, cleanup, err := development.Handler(hook.Intercept(handleCfg, func(*Event) (struct{}, error) { return struct{}{}, nil }, loads.Intercept(pages)))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var begin prerenderRequestAnswer
	buildOperation(t, handler, "/_skgo/dev/begin", map[string]any{"url": "http://app.test/typed/00042", "logicalUrl": "http://app.test/typed/00042", "method": "GET", "routeId": "/typed/[n=Number]", "params": map[string]any{"n": 42}}, &begin)
	if !begin.Resolve {
		t.Fatalf("begin: %+v", begin)
	}
	response := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/_skgo/dev/load", strings.NewReader(fmt.Sprintf(`{"handle":%q,"module":%q,"url":"http://app.test/typed/00042"}`, begin.Handle, module)))
	request.Header.Set("Authorization", "Bearer secret")
	handler.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"number:42"`) || !strings.Contains(response.Body.String(), `"00042"`) {
		t.Fatal(response.Body.String())
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": begin.Handle}, nil)
}

func TestNativeDevelopmentClientAddressPreservesPublicPeerAndConfiguredProvider(t *testing.T) {
	for _, test := range []struct {
		name     string
		provider func(*http.Request) (string, error)
		want     string
	}{
		{name: "default", want: "127.0.0.1"},
		{name: "application", provider: func(*http.Request) (string, error) { return "literal-configured-address-827", nil }, want: "literal-configured-address-827"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := NewRemote(RemoteSpec{Kind: KindQuery, Module: "src/peer.remote.ts", Name: "peer", Call: func(ctx context.Context, _ Call) (any, error) { return EventFrom(ctx).ClientAddress() }})
			loads, _ := NewLoads(LoadConfig{Origin: "http://app.test"}, NewServerLoad(LoadSpec{Module: "peer", Run: func(ctx context.Context) (any, error) {
				address, err := EventFrom(ctx).ClientAddress()
				return map[string]any{"address": address}, err
			}}))
			remotes, _ := NewRemotes(RemoteConfig{Origin: "http://app.test"}, fn)
			development := NewNativeDevelopment("secret")
			development.NewSSR(fstest.MapFS{}, Manifest{}, loads, remotes, SSROptions{})
			pages, _ := development.NewStaticHandler(fstest.MapFS{})
			hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
				return resolve(ctx, event)
			})
			app := hook.Intercept(HandleConfig{Origin: "http://app.test", ClientAddress: test.provider}, func(*Event) (struct{}, error) { return struct{}{}, nil }, remotes.Intercept(pages))
			handler, cleanup, err := development.Handler(app)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			request := httptest.NewRequest("GET", remotes.Prefix()+fn.ID(), nil)
			request.RemoteAddr = "@"
			request.Header.Set("X-Skgo-Client-Address", "127.0.0.1")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != 200 || !strings.Contains(response.Body.String(), test.want) {
				t.Fatalf("remote address status=%d %s", response.Code, response.Body.String())
			}
			var begin prerenderRequestAnswer
			buildOperation(t, handler, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/page", Method: "GET", ClientAddress: "127.0.0.1"}, &begin)
			recorder := httptest.NewRecorder()
			callback := httptest.NewRequest("POST", "/_skgo/dev/load", strings.NewReader(fmt.Sprintf(`{"handle":%q,"module":"peer","url":"http://app.test/page"}`, begin.Handle)))
			callback.Header.Set("Authorization", "Bearer secret")
			handler.ServeHTTP(recorder, callback)
			if recorder.Code != 200 || !strings.Contains(recorder.Body.String(), test.want) {
				t.Fatalf("Kit callback address status=%d %s", recorder.Code, recorder.Body.String())
			}
			buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": begin.Handle}, nil)

		})
	}
}

func TestNativeDevelopmentHookRefusalKeepsHandledPublicError(t *testing.T) {
	development := NewNativeDevelopment("secret")
	loads, _ := NewLoads(LoadConfig{Origin: "http://app.test"})
	remotes, _ := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	development.NewSSR(fstest.MapFS{}, Manifest{}, loads, remotes, SSROptions{})
	pages, _ := development.NewStaticHandler(fstest.MapFS{})
	var calls atomic.Int32
	hook := hookMiddleware(func(context.Context, hookEvent, hookResolve) (*http.Response, error) {
		return nil, Errorf(403, "literal-refusal-551")
	})
	cfg := HandleConfig{Origin: "http://app.test", HandleError: func(context.Context, CaughtError) map[string]any {
		calls.Add(1)
		return map[string]any{"supportId": "literal-case-781"}
	}}
	app := hook.Intercept(cfg, func(*Event) (struct{}, error) { return struct{}{}, nil }, pages)
	handler, cleanup, err := development.Handler(app)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	var answer prerenderRequestAnswer
	buildOperation(t, handler, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/action", Method: "POST", Headers: http.Header{"Accept": {"application/json"}}}, &answer)
	if answer.Error == nil || answer.Error.Status != 403 || !strings.Contains(string(answer.Body), `"supportId":"literal-case-781"`) || !strings.Contains(string(answer.Body), `"message":"literal-refusal-551"`) || calls.Load() != 1 {
		t.Fatalf("answer=%+v public=%s hookCalls=%d", answer, string(answer.Body), calls.Load())
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": answer.Handle}, nil)
}

func TestNativeDevelopmentLoadDiagnosticsWithoutTracing(t *testing.T) {
	for _, test := range []struct {
		name, literal string
		run           func(context.Context) (any, error)
	}{
		{"returned", "DEV_DIAG_RETURNED_243", func(context.Context) (any, error) { return nil, fmt.Errorf("DEV_DIAG_RETURNED_243") }},
		{"panic", "DEV_DIAG_PANIC_243", func(context.Context) (any, error) { panic("DEV_DIAG_PANIC_243") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			var healthy atomic.Bool
			load := NewServerLoad(LoadSpec{Module: "page", Source: "src/routes/diagnostic/page.server.go:17", Run: func(ctx context.Context) (any, error) {
				if healthy.Load() {
					return map[string]any{"receipt": "DEV_DIAG_HEALTHY_243"}, nil
				}
				return test.run(ctx)
			}})
			handler, close, _ := nativeFixture(t, load)
			defer close()
			invoke := func() prerenderLoadOutput {
				handle := beginNative(t, handler)
				var answer prerenderLoadOutput
				buildOperation(t, handler, "/_skgo/dev/load", map[string]string{"handle": handle, "module": "page", "url": "http://app.test/page"}, &answer)
				buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
				return answer
			}
			answer := invoke()
			if answer.Error == nil || answer.Error.Status != 500 || answer.NativeError == nil || answer.NativeError.Message != test.literal || answer.NativeError.Source != "src/routes/diagnostic/page.server.go:17" {
				t.Fatalf("missing private diagnostic: %+v", answer)
			}
			if strings.Contains(string(answer.Body), test.literal) || strings.Contains(string(answer.Body), "page.server.go") {
				t.Fatalf("private diagnostic in public body: %s", answer.Body)
			}
			if test.name == "panic" && !strings.Contains(answer.NativeError.Stack, "dev_bridge_test.go:") {
				t.Fatalf("panic lost actual Go stack: %+v", answer.NativeError)
			}
			healthy.Store(true)
			next := invoke()
			if next.Error != nil || !strings.Contains(string(next.Data), "DEV_DIAG_HEALTHY_243") {
				t.Fatalf("recovery failed: %+v", next)
			}
		})
	}
}

func TestDevelopmentPanicFramesUseAuthoredRoute(t *testing.T) {
	stack := "goroutine 1 [running]:\nexample.com/app/generated/_links/onzggl3sn52xizlt/panicLoad()\n\t/private/app/generated/_links/onzggl3sn52xizlt/page.server.go:9 +0x12\n"
	got := authoredDevelopmentStack(stack)
	if !strings.Contains(got, "\tsrc/routes/page.server.go:9 +0x12") || strings.Contains(got, "/generated/_links/onzggl3sn52xizlt/page.server.go") {
		t.Fatal(got)
	}
}
