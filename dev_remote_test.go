package skgo

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/tylergannon/devalue/v5"
)

func nativeRemoteFixture(t *testing.T, functions ...*Remote) (http.Handler, func()) {
	t.Helper()
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
	loads, err := NewLoads(LoadConfig{Origin: "http://app.test", HandleError: func(context.Context, CaughtError) map[string]any {
		return map[string]any{"message": "literal-safe-441", "supportId": "literal-case-552"}
	}})
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://app.test"}, functions...)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSSR(development, Manifest{}, loads, remotes, SSROptions{}); err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(development)
	if err != nil {
		t.Fatal(err)
	}
	handler, close, err := development.Handler(loads.Intercept(remotes.Intercept(pages)))
	if err != nil {
		t.Fatal(err)
	}
	return handler, close
}
func TestNativeBatchInvokesOneGoBatchForKitResolver(t *testing.T) {
	var seen [][]string
	fn := batchQuotes(&seen)
	handler, close := nativeRemoteFixture(t, fn)
	defer close()
	handle := beginNative(t, handler)
	var answer struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	buildOperation(t, handler, "/_skgo/dev/remote", map[string]any{"handle": handle, "module": testModule, "name": "getQuotes", "payloads": []string{payload(t, "ALPHA"), payload(t, "BETA")}}, &answer)
	if len(seen) != 1 || len(seen[0]) != 2 || seen[0][0] != "ALPHA" || seen[0][1] != "BETA" {
		t.Fatalf("batch=%v", seen)
	}
	value, err := devalue.Parse(answer.Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	values := field(t, value, "_").([]any)
	if answer.Type != "result" || len(values) != 2 || field(t, values[0], "symbol") != "ALPHA" || field(t, values[1], "symbol") != "BETA" || field(t, values[0], "batchSize") != float64(2) || field(t, values[1], "batchSize") != float64(2) {
		t.Fatalf("answer=%+v", answer)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}
func TestNativeLiveStreamsFirstValueAndCancelsProducer(t *testing.T) {
	stopped := make(chan struct{})
	fn := NewLiveQueryNoArg(testModule, "nativeLive", func(ctx context.Context, yield func(string) error) error {
		defer close(stopped)
		if err := yield("literal-first-557"); err != nil {
			return err
		}
		<-ctx.Done()
		return ctx.Err()
	})
	handler, close := nativeRemoteFixture(t, fn)
	defer close()
	handle := beginNative(t, handler)
	server := httptest.NewServer(handler)
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	raw, _ := json.Marshal(map[string]any{"handle": handle, "module": testModule, "name": "nativeLive"})
	request, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/_skgo/dev/remote", strings.NewReader(string(raw)))
	request.Header.Set("Authorization", "Bearer secret")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := bufio.NewReader(response.Body).ReadString('\n')
	if err != nil || !strings.Contains(frame, "literal-first-557") {
		t.Fatalf("frame=%s err=%v", frame, err)
	}
	cancel()
	response.Body.Close()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("live producer survived callback cancellation")
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeUnenhancedFormKeepsKitResultAndIssues(t *testing.T) {
	type input struct {
		Name string `json:"name"`
	}
	fn := NewForm(testModule, "nativeForm", func(ctx context.Context, arg input) (string, error) {
		if arg.Name == "bad" {
			return "", Invalidf("name", "literal validation-771")
		}
		return "literal saved-882 " + arg.Name, nil
	})
	handler, close := nativeRemoteFixture(t, fn)
	defer close()
	for _, name := range []string{"Ada", "bad"} {
		controls := url.Values{}
		controls.Set("name/"+fn.id, name)
		var begin prerenderRequestAnswer
		buildOperation(t, handler, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/form?/remote=" + url.QueryEscape(fn.id), Method: "POST", Headers: http.Header{"Content-Type": {"application/x-www-form-urlencoded"}, "Origin": {"http://app.test"}}, Body: []byte(controls.Encode())}, &begin)
		if !begin.Resolve || begin.Handle == "" {
			t.Fatalf("begin=%+v", begin)
		}
		var answer struct {
			Type    string `json:"type"`
			Data    string `json:"data"`
			Invalid bool   `json:"invalid"`
		}
		buildOperation(t, handler, "/_skgo/dev/remote", map[string]string{"handle": begin.Handle, "module": testModule, "name": "nativeForm"}, &answer)
		if answer.Type != "result" || answer.Invalid != (name == "bad") {
			t.Fatalf("answer=%+v", answer)
		}
		if name == "Ada" && !strings.Contains(answer.Data, "literal saved-882 Ada") {
			t.Fatal(answer.Data)
		}
		if name == "bad" && !strings.Contains(answer.Data, "literal validation-771") {
			t.Fatal(answer.Data)
		}
		buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": begin.Handle}, nil)
	}
}

func TestNativeParentTransportRestoresGoTypeBeforeParentRead(t *testing.T) {
	type money struct {
		Cents int `json:"cents"`
	}
	transport := Transport{"Money": {Type: reflect.TypeOf(money{}), Encode: func(value any) (any, error) { return []any{float64(value.(money).Cents)}, nil }, Decode: func(value any) (any, error) { return money{Cents: int(value.([]any)[0].(float64))}, nil }}}
	load := NewServerLoad(LoadSpec{Module: "child", Run: func(ctx context.Context) (any, error) {
		parent, err := Parent[struct {
			Amount money `json:"amount"`
		}](ctx)
		return map[string]any{"literalParentCents": parent.Amount.Cents}, err
	}})
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
	loads, err := NewLoads(LoadConfig{Origin: "http://app.test", Transport: transport}, load)
	if err != nil {
		t.Fatal(err)
	}
	remotes, err := NewRemotes(RemoteConfig{Origin: "http://app.test", Transport: transport})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewSSR(development, Manifest{}, loads, remotes, SSROptions{}); err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(development)
	if err != nil {
		t.Fatal(err)
	}
	handler, close, err := development.Handler(loads.Intercept(remotes.Intercept(pages)))
	if err != nil {
		t.Fatal(err)
	}
	defer close()
	handle := beginNative(t, handler)
	var answer prerenderLoadOutput
	buildOperation(t, handler, "/_skgo/dev/load", map[string]string{"handle": handle, "module": "child", "url": "http://app.test/page", "parentData": `[{"amount":1},["Money",2],[3],771]`}, &answer)
	if answer.Error != nil || !strings.Contains(string(answer.Data), "771") {
		t.Fatalf("answer=%+v", answer)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeRemoteErrorKeepsConfiguredPublicBody(t *testing.T) {
	fn := NewQueryNoArg(testModule, "nativeFailure", func(context.Context) (string, error) { return "", errors.New("private-database-detail") })
	handler, close := nativeRemoteFixture(t, fn)
	defer close()
	handle := beginNative(t, handler)
	var answer map[string]any
	buildOperation(t, handler, "/_skgo/dev/remote", map[string]string{"handle": handle, "module": testModule, "name": "nativeFailure"}, &answer)
	raw, _ := json.Marshal(answer)
	if !strings.Contains(string(raw), "literal-safe-441") || !strings.Contains(string(raw), "literal-case-552") || strings.Contains(string(raw), "private-database-detail") {
		t.Fatal(string(raw))
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestNativeUnenhancedFormBindsTypedPageCaller(t *testing.T) {
	fn := NewForm(testModule, "typedNativeForm", func(ctx context.Context, arg struct {
		Name string `json:"name"`
	}) (string, error) {
		route, values := RemoteCallerValues(EventFrom(ctx))
		number, ok := values["n"].(fixtureNumber)
		if !ok {
			return "typed caller absent", nil
		}
		return route + " " + number.Label() + " " + arg.Name, nil
	})
	const module = "src/routes/typed/[n=Number]/+page.server.ts"
	manifest := Manifest{Remotes: []string{fn.id}, Loads: []string{module}, Nodes: []string{module}, Routes: []ManifestRoute{{ID: "/typed/[n=Number]", Pattern: `^/typed/([^/]+?)/?$`, Params: []ManifestParam{{Name: "n", Matcher: "Number"}}, Page: &ManifestPage{Leaf: 0}}}}
	matchers := map[string]ParamMatcher{"Number": func(raw string) (any, bool) {
		if raw == "00042" {
			return fixtureNumber(42), true
		}
		return nil, false
	}}
	cfg := manifest.LoadConfig("http://app.test")
	cfg.Matchers = matchers
	loads, err := NewLoads(cfg, NewServerLoad(LoadSpec{Module: module, Run: func(context.Context) (any, error) { return map[string]any{}, nil }}))
	if err != nil {
		t.Fatal(err)
	}
	remoteCfg := manifest.RemoteConfig("http://app.test")
	fn.callerMatchers = CallerMatchers(matchers)
	fn.callerRoutes = CallerRoutes{"/typed/[n=Number]": {Params: []ManifestParam{{Name: "n", Matcher: "Number"}}, NewParams: func(map[string]any) (any, error) { return struct{}{}, nil }}}
	remotes, err := NewRemotes(remoteCfg, fn)
	if err != nil {
		t.Fatal(err)
	}
	development := NewNativeDevelopment(fstest.MapFS{}, "secret")
	if _, err := NewSSR(development, manifest, loads, remotes, SSROptions{}); err != nil {
		t.Fatal(err)
	}
	pages, err := NewStaticHandler(development)
	if err != nil {
		t.Fatal(err)
	}
	handleCfg := manifest.HandleConfig()
	handleCfg.Origin = "http://app.test"
	handleCfg.Matchers = matchers
	handleCfg.Loads = loads
	hook := hookMiddleware(func(ctx context.Context, event hookEvent, resolve hookResolve) (*http.Response, error) {
		return resolve(ctx, event)
	})
	handler, cleanup, err := development.Handler(hook.Intercept(handleCfg, func(*Event) (struct{}, error) { return struct{}{}, nil }, loads.Intercept(remotes.Intercept(pages))))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	for _, suffix := range []string{"", `/"key-713"`} {
		controls := url.Values{}
		controls.Set("name/"+fn.id, "literal-name-824")
		var begin prerenderRequestAnswer
		buildOperation(t, handler, "/_skgo/dev/begin", prerenderRequestInput{URL: "http://app.test/typed/00042?/remote=" + url.QueryEscape(fn.id+suffix), Method: "POST", RouteID: "/typed/[n=Number]", Headers: http.Header{"Origin": {"http://app.test"}, "Content-Type": {"application/x-www-form-urlencoded"}}, Body: []byte(controls.Encode())}, &begin)
		var answer struct {
			Type string `json:"type"`
			Data string `json:"data"`
		}
		buildOperation(t, handler, "/_skgo/dev/remote", map[string]string{"handle": begin.Handle, "module": testModule, "name": "typedNativeForm"}, &answer)
		if answer.Type != "result" || !strings.Contains(answer.Data, "/typed/[n=Number] number:42 literal-name-824") {
			t.Fatalf("key=%q answer=%+v", suffix, answer)
		}
		buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": begin.Handle}, nil)
	}
}
