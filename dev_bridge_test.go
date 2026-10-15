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
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
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
	if _, err = NewSSR(development, Manifest{}, loads, remotes, SSROptions{Actions: actionRegistry}); err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(development)
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
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
	loads, _ := NewLoads(LoadConfig{Origin: "http://app.test", HandleError: func(context.Context, CaughtError) map[string]any {
		return map[string]any{"message": "literal-public-834", "supportId": "literal-case-335"}
	}}, NewServerLoad(LoadSpec{Module: "page", Run: func(context.Context) (any, error) { return nil, fmt.Errorf("private-database-detail") }}))
	remotes, _ := NewRemotes(RemoteConfig{Origin: "http://app.test"})
	NewSSR(development, Manifest{}, loads, remotes, SSROptions{})
	pages, _ := NewStaticHandler(development)
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
	if !strings.Contains(recorder.Body.String(), `"supportId":"literal-case-335"`) || !strings.Contains(recorder.Body.String(), `"message":"literal-public-834"`) || strings.Contains(recorder.Body.String(), "private-database-detail") {
		t.Fatal(recorder.Body.String())
	}
	buildOperation(t, configured, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeDevelopmentEndpointManifestUsesOriginalRegistry(t *testing.T) {
	manifest := Manifest{Remotes: []string{}, Loads: []string{}, Routes: []ManifestRoute{{ID: "/orders", Pattern: `^/orders/?$`, Endpoint: &ManifestEndpoint{Methods: []string{"GET"}}}}}
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
	loads, err := NewLoads(manifest.LoadConfig("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(manifest.RemoteConfig("http://app.test"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewSSR(development, manifest, loads, remotes, SSROptions{})
	if err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(development)
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
