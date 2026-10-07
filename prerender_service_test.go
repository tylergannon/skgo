package skgo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/polytype/devalue"
)

func callBuildOperation(path string, in io.Reader, out io.Writer, transport Transport, loads []*ServerLoad, remotes []*Remote) error {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1"+path, in)
	request.Header.Set("Authorization", "Bearer fixture-secret")
	response := httptest.NewRecorder()
	prerenderHandler("fixture-secret", transport, loads, remotes, nil).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		return fmt.Errorf("prerender HTTP %d: %s", response.Code, response.Body.String())
	}
	_, err := io.Copy(out, response.Body)
	return err
}

func TestPrerenderServiceRequiresAuthentication(t *testing.T) {
	invoked := false
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "test.remote.ts", Name: "item", Call: func(context.Context, Call) (any, error) { invoked = true; return "result", nil }})
	for _, credential := range []string{"", "Bearer incorrect"} {
		request := httptest.NewRequest(http.MethodPost, "/remote", strings.NewReader(`{"module":"test.remote.ts","name":"item","url":"http://app.test/"}`))
		request.Header.Set("Authorization", credential)
		response := httptest.NewRecorder()
		prerenderHandler("fixture-secret", nil, nil, []*Remote{fn}, nil).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || invoked {
			t.Fatalf("unauthorized request status=%d invoked=%v", response.Code, invoked)
		}
	}
}

func TestPrerenderServiceConcurrentCallsKeepOriginalRequestsSeparate(t *testing.T) {
	const module = "src/routes/item/+page.server.ts"
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	load := NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
		event := EventFrom(ctx)
		entered <- struct{}{}
		<-release
		incoming, _ := event.Cookie("incoming")
		if event.Request().Header.Get("Authorization") != "application-token" {
			return nil, fmt.Errorf("helper authentication leaked")
		}
		event.SetHeader("X-Result", incoming)
		event.SetCookie("outgoing", incoming, CookieOptions{Path: "/"})
		return map[string]any{"value": event.URL().Host + ":" + incoming}, nil
	}})
	handler := prerenderHandler("fixture-secret", nil, []*ServerLoad{load}, nil, nil)
	var wg sync.WaitGroup
	for _, value := range []string{"alpha", "beta"} {
		wg.Add(1)
		go func(value string) {
			defer wg.Done()
			input, _ := json.Marshal(PrerenderLoadInput{Module: module, URL: "http://" + value + ".test/item", Headers: http.Header{"Cookie": {"incoming=" + value}, "Authorization": {"application-token"}}})
			request := httptest.NewRequest(http.MethodPost, "/load", strings.NewReader(string(input)))
			request.Header.Set("Authorization", "Bearer fixture-secret")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			var got prerenderLoadOutput
			if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
				t.Error(err)
				return
			}
			tree, err := devalue.Parse(string(got.Data), nil)
			if err != nil {
				t.Errorf("answer %s: %v", response.Body.String(), err)
				return
			}
			if field(t, tree, "value") != value+".test:"+value || got.Headers.Get("X-Result") != value || len(got.Cookies) != 1 || got.Cookies[0].Value != value {
				t.Errorf("crossed request state: %+v data=%v", got, tree)
			}
		}(value)
	}
	<-entered
	<-entered
	close(release)
	wg.Wait()
}

func TestPrerenderServiceDisconnectCancelsInputs(t *testing.T) {
	entered, canceled := make(chan struct{}), make(chan struct{})
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "cancel.remote.ts", Name: "item", Call: func(context.Context, Call) (any, error) { return nil, nil }, Inputs: func(ctx context.Context, _ Call) ([]any, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}})
	server := httptest.NewServer(prerenderHandler("fixture-secret", nil, nil, []*Remote{fn}, nil))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/inputs", strings.NewReader(`{"module":"cancel.remote.ts","name":"item"}`))
	request.Header.Set("Authorization", "Bearer fixture-secret")
	done := make(chan struct{})
	go func() {
		response, _ := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		close(done)
	}()
	<-entered
	cancel()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP disconnect did not cancel producer")
	}
	<-done
}

func TestPrerenderServiceOwnerEOFStopsListenerAndCancelsWork(t *testing.T) {
	owner, helper := net.Pipe()
	defer owner.Close()
	defer helper.Close()
	entered, canceled := make(chan struct{}), make(chan struct{})
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "owner.remote.ts", Name: "item", Call: func(ctx context.Context, _ Call) (any, error) {
		close(entered)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	}})
	done := make(chan error, 1)
	go func() {
		done <- servePrerenderService(context.Background(), helper, nil, nil, []*Remote{fn}, testPrerenderOptions(nil))
	}()
	var ready struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(owner).Decode(&ready); err != nil {
		t.Fatal(err)
	}
	if len(ready.Secret) != 64 || !strings.HasPrefix(ready.URL, "http://127.0.0.1:") {
		t.Fatalf("readiness: %+v", ready)
	}
	handle := beginTestPrerender(t, ready.URL, ready.Secret)
	request, _ := http.NewRequest(http.MethodPost, ready.URL+"/remote", strings.NewReader(`{"module":"owner.remote.ts","name":"item","url":"http://app.test/","handle":"`+handle+`"}`))
	request.Header.Set("Authorization", "Bearer "+ready.Secret)
	responseDone := make(chan struct{})
	go func() {
		response, _ := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		close(responseDone)
	}()
	<-entered
	owner.Close()
	select {
	case <-canceled:
	case <-time.After(2 * time.Second):
		t.Fatal("owner EOF did not cancel callback")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("helper did not stop")
	}
	<-responseDone
	if connection, err := net.DialTimeout("tcp", strings.TrimPrefix(ready.URL, "http://"), time.Second); err == nil {
		connection.Close()
		t.Fatal("helper listener remains open")
	}
}

func TestPrerenderServiceLoadSettlesTransportedDeferredValue(t *testing.T) {
	const module = "src/routes/prices/+page.server.ts"
	transport := Transport{"Money": {Type: reflect.TypeFor[money](), Encode: func(value any) (any, error) { return []any{float64(value.(money).Cents)}, nil }, Decode: func(value any) (any, error) { return money{Cents: int(value.([]any)[0].(float64))}, nil }}}
	load := NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
		return struct {
			Immediate money           `json:"immediate"`
			Later     Deferred[money] `json:"later"`
		}{money{Cents: 1250}, Async(ctx, func(context.Context) (money, error) { return money{Cents: 990}, nil })}, nil
	}})
	request := `{"module":"src/routes/prices/+page.server.ts","url":"http://app.test/prices"}`
	var output strings.Builder
	if err := callBuildOperation("/load", strings.NewReader(request), &output, transport, []*ServerLoad{load}, nil); err != nil {
		t.Fatal(err)
	}
	var answer prerenderLoadOutput
	if err := json.Unmarshal([]byte(output.String()), &answer); err != nil {
		t.Fatal(err)
	}
	tree, err := devalue.Parse(string(answer.Data), map[string]func(any) (any, error){"Money": func(value any) (any, error) { return transport["Money"].Decode(value) }, "Promise": func(value any) (any, error) { return value, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if got := field(t, tree, "immediate"); got != (money{Cents: 1250}) {
		t.Fatalf("immediate=%#v", got)
	}
	if len(answer.Chunks) != 1 || answer.Chunks[0].ID != 1 || answer.Chunks[0].Error != "" {
		t.Fatalf("settled chunks=%+v", answer.Chunks)
	}
	chunk, err := devalue.Parse(string(answer.Chunks[0].Data), map[string]func(any) (any, error){"Money": func(value any) (any, error) { return transport["Money"].Decode(value) }})
	if err != nil || chunk != (money{Cents: 990}) {
		t.Fatalf("deferred=%#v error=%v", chunk, err)
	}
}

func TestPrerenderServiceRemoteSharesTransportAndPublicSanitization(t *testing.T) {
	transport := Transport{"Money": {Type: reflect.TypeFor[money](), Encode: func(value any) (any, error) { return []any{float64(value.(money).Cents)}, nil }, Decode: func(value any) (any, error) { return money{Cents: int(value.([]any)[0].(float64))}, nil }}}
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "price.remote.ts", Name: "price", Call: func(ctx context.Context, call Call) (any, error) {
		if EventFrom(ctx).Request().Header.Get("X-Fail") == "yes" {
			return nil, fmt.Errorf("private database detail")
		}
		value, err := transport["Money"].Decode(call.Arg)
		if err != nil {
			return nil, err
		}
		return call.Transported(value)
	}})
	registry, err := NewRemotes(RemoteConfig{Dev: true, Transport: transport}, fn)
	if err != nil {
		t.Fatal(err)
	}
	// The argument codec receives Kit's transport tag and the result encoder uses
	// that same tag, on both the build and public HTTP boundaries.
	encoded, err := devalue.StringifyWith(money{Cents: 780}, transport.reducers())
	if err != nil {
		t.Fatal(err)
	}
	payload := base64.RawURLEncoding.EncodeToString([]byte(encoded))
	input, _ := json.Marshal(map[string]any{"module": "price.remote.ts", "name": "price", "url": "http://app.test/price", "payload": payload})
	var build strings.Builder
	if err := callBuildOperation("/remote", strings.NewReader(string(input)), &build, transport, nil, []*Remote{fn}); err != nil {
		t.Fatal(err)
	}
	public := httptest.NewRecorder()
	registry.ServeHTTP(public, httptest.NewRequest(http.MethodGet, registry.Prefix()+fn.id+"/"+payload, nil))
	var buildResult, publicResult remoteResponse
	json.Unmarshal([]byte(build.String()), &buildResult)
	json.Unmarshal(public.Body.Bytes(), &publicResult)
	if buildResult.Type != "result" || publicResult.Type != "result" || buildResult.Data != publicResult.Data {
		t.Fatalf("build=%s public=%s", build.String(), public.Body.String())
	}
	value, err := devalue.Parse(buildResult.Data, map[string]func(any) (any, error){"Money": func(value any) (any, error) { return transport["Money"].Decode(value) }})
	if err != nil || field(t, value, "_") != (money{Cents: 780}) {
		t.Fatalf("result=%#v error=%v", value, err)
	}
	failedBuildInput := `{"module":"price.remote.ts","name":"price","url":"http://app.test/price","headers":{"x-fail":["yes"]}}`
	var failedBuild strings.Builder
	if err := callBuildOperation("/remote", strings.NewReader(failedBuildInput), &failedBuild, transport, nil, []*Remote{fn}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(failedBuild.String(), `"diagnostic":"private database detail"`) {
		t.Fatalf("build lost original header or failure detail: %s", failedBuild.String())
	}
	request := httptest.NewRequest(http.MethodGet, registry.Prefix()+fn.id, nil)
	request.Header.Set("X-Fail", "yes")
	failed := httptest.NewRecorder()
	registry.ServeHTTP(failed, request)
	if strings.Contains(failed.Body.String(), "private database detail") || !strings.Contains(failed.Body.String(), "Internal Error") {
		t.Fatalf("public error leaked: %s", failed.Body.String())
	}
}

func TestPrerenderServiceAbandonedCallbackFailsHelper(t *testing.T) {
	owner, helper := net.Pipe()
	defer owner.Close()
	defer helper.Close()
	entered := make(chan struct{})
	fn := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "timeout.remote.ts", Name: "item", Call: func(ctx context.Context, _ Call) (any, error) { close(entered); <-ctx.Done(); return nil, ctx.Err() }})
	done := make(chan error, 1)
	go func() {
		done <- servePrerenderService(context.Background(), helper, nil, nil, []*Remote{fn}, testPrerenderOptions(nil))
	}()
	var ready struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(owner).Decode(&ready); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	handle := beginTestPrerender(t, ready.URL, ready.Secret)
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, ready.URL+"/remote", strings.NewReader(`{"module":"timeout.remote.ts","name":"item","url":"http://app.test/","handle":"`+handle+`"}`))
	request.Header.Set("Authorization", "Bearer "+ready.Secret)
	clientDone := make(chan struct{})
	go func() {
		response, _ := http.DefaultClient.Do(request)
		if response != nil {
			response.Body.Close()
		}
		close(clientDone)
	}()
	<-entered
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "timeout.remote.ts#item") || !strings.Contains(err.Error(), "callback canceled") {
			t.Fatalf("abandoned callback helper result=%v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("abandoned callback did not fail helper")
	}
	<-clientDone
	if connection, err := net.DialTimeout("tcp", strings.TrimPrefix(ready.URL, "http://"), time.Second); err == nil {
		connection.Close()
		t.Fatal("abandoned callback left helper serving")
	}
}

func testPrerenderOptions(h RequestMiddleware[struct{}, struct{}]) PrerenderServiceOptions {
	return PrerenderServiceOptions{BindRequest: func(next http.Handler) http.Handler {
		return h.Intercept(HandleConfig{}, func(*Event) (struct{}, error) { return struct{}{}, nil }, next)
	}}
}
func beginTestPrerender(t *testing.T, server, secret string) string {
	t.Helper()
	req, _ := http.NewRequest("POST", server+"/begin", strings.NewReader(`{"url":"http://app.test/","method":"GET"}`))
	req.Header.Set("Authorization", "Bearer "+secret)
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var answer prerenderRequestAnswer
	if err := json.NewDecoder(response.Body).Decode(&answer); err != nil || !answer.Resolve || answer.Handle == "" {
		t.Fatalf("begin: %+v %v", answer, err)
	}
	return answer.Handle
}
