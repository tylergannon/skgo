package skgo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type buildLocals struct {
	Value string
	Calls int
}

func buildOperation(t *testing.T, handler http.Handler, path string, input any, answer any) {
	t.Helper()
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", path, bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 {
		t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
	}
	if answer != nil {
		if err := json.Unmarshal(response.Body.Bytes(), answer); err != nil {
			t.Fatalf("%s: %v: %s", path, err, response.Body.String())
		}
	}
}
func buildBoundary(h RequestMiddleware[struct{}, buildLocals]) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return h.Intercept(HandleConfig{ClientAddress: func(*http.Request) (string, error) { return "", fmt.Errorf("unavailable during build") }}, func(*Event) (struct{}, error) { return struct{}{}, nil }, next)
	}
}
func TestPrerenderLogicalRequestSharesLocalsAndRealResponse(t *testing.T) {
	var hooks atomic.Int32
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		hooks.Add(1)
		if event.Request().Header.Get("Authorization") != "app-token" || event.Request().Header.Get("X-Application") != "incoming" {
			return nil, fmt.Errorf("logical request headers lost")
		}
		event.Locals = &buildLocals{Value: event.URL().Query().Get("value"), Calls: 10}
		if err := event.SetHeader("X-Before", "before"); err != nil {
			return nil, err
		}
		if err := event.SetCookie("before", "cookie", CookieOptions{Path: "/"}); err != nil {
			return nil, err
		}
		response, err := resolve(ctx, event)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != 201 || response.Header.Get("X-Kit") != "actual" {
			return nil, fmt.Errorf("not the real Kit response: %d %v", response.StatusCode, response.Header)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		response.Body = io.NopCloser(strings.NewReader(string(body) + ":" + event.Locals.Value + ":after"))
		response.Header.Set("X-After", fmt.Sprint(event.Locals.Calls))
		return response, nil
	})
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		locals := RequestLocals[buildLocals](ctx)
		locals.Calls++
		event := EventFrom(ctx)
		if _, err := event.ClientAddress(); err == nil {
			return nil, fmt.Errorf("fabricated build client address")
		}
		cookie, _ := event.Cookie("before")
		return map[string]any{"value": locals.Value, "calls": locals.Calls, "cookie": cookie}, nil
	}})
	remote := NewRemote(RemoteSpec{Kind: KindPrerender, Module: "remote", Name: "value", Call: func(ctx context.Context, _ Call) (any, error) {
		locals := RequestLocals[buildLocals](ctx)
		locals.Calls++
		return locals.Value + ":" + fmt.Sprint(locals.Calls), nil
	}})
	sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
	defer sessions.close()
	handler := prerenderHandler("secret", nil, []*ServerLoad{load}, []*Remote{remote}, nil, sessions)
	var first, second prerenderRequestAnswer
	buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/page?value=alpha", Headers: http.Header{"authorization": {"app-token"}, "x-application": {"incoming"}}}, &first)
	buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/page?value=beta", Headers: http.Header{"authorization": {"app-token"}, "x-application": {"incoming"}}}, &second)
	for i, answer := range []prerenderRequestAnswer{first, second} {
		value := []string{"alpha", "beta"}[i]
		if !answer.Resolve || answer.Headers.Get("X-Before") != "before" || len(answer.Cookies) != 1 || answer.Cookies[0].Value != "cookie" {
			t.Fatalf("begin effects: %+v", answer)
		}
		var output prerenderLoadOutput
		buildOperation(t, handler, "/load", map[string]any{"handle": answer.Handle, "module": "page", "url": "http://app.test/page"}, &output)
		if output.Error != nil || !bytes.Contains(output.Data, []byte(`"`+value+`"`)) || !bytes.Contains(output.Data, []byte(`11`)) || !bytes.Contains(output.Data, []byte(`"cookie"`)) {
			t.Fatalf("shared load: %+v", output)
		}
		var result struct {
			Data string `json:"data"`
		}
		buildOperation(t, handler, "/remote", map[string]any{"handle": answer.Handle, "module": "remote", "name": "value", "url": "http://app.test/page"}, &result)
		if result.Data != `[{"_":1},"`+value+`:12"]` {
			t.Fatalf("shared remote: %s", result.Data)
		}
		var final prerenderRequestAnswer
		buildOperation(t, handler, "/response", map[string]any{"handle": answer.Handle, "response": prerenderResponse{Status: 201, Headers: http.Header{"x-kit": {"actual"}}, Body: []byte("actual Kit body")}}, &final)
		if final.Response == nil || final.Response.Status != 201 || string(final.Response.Body) != "actual Kit body:"+value+":after" || final.Response.Headers.Get("X-After") != "12" {
			t.Fatalf("final response: %+v", final)
		}
		buildOperation(t, handler, "/end", map[string]string{"handle": answer.Handle}, nil)
	}
	if hooks.Load() != 2 || len(sessions.entries) != 0 {
		t.Fatalf("logical lifecycle hooks=%d entries=%d", hooks.Load(), len(sessions.entries))
	}
}
func TestPrerenderRefusalsSkipResolutionAndPreserveMeaning(t *testing.T) {
	for _, kind := range []string{"redirect", "error", "response"} {
		t.Run(kind, func(t *testing.T) {
			hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
				event.SetCookie("refusal", "kept", CookieOptions{Path: "/"})
				switch kind {
				case "redirect":
					return nil, &Redirect{Status: 307, Location: "/literal-target"}
				case "error":
					return nil, Errorf(403, "literal refusal")
				default:
					return NewResponse(202, http.Header{"X-Selected": {"literal"}}, strings.NewReader("selected body")), nil
				}
			})
			sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
			defer sessions.close()
			handler := prerenderHandler("secret", nil, nil, nil, nil, sessions)
			var answer prerenderRequestAnswer
			buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/unmatched"}, &answer)
			if answer.Resolve {
				t.Fatal("refusal entered resolve")
			}
			switch kind {
			case "redirect":
				if answer.Redirect == nil || answer.Redirect.Status != 307 || answer.Redirect.Location != "/literal-target" || len(answer.Cookies) != 1 || answer.Cookies[0].Value != "kept" {
					t.Fatalf("redirect: %+v", answer)
				}
			case "error":
				if answer.Error == nil || answer.Error.Status != 403 || answer.Error.Message != "literal refusal" {
					t.Fatalf("error: %+v", answer)
				}
			case "response":
				if answer.Response == nil || answer.Response.Status != 202 || string(answer.Response.Body) != "selected body" || answer.Response.Headers.Get("X-Selected") != "literal" {
					t.Fatalf("response: %+v", answer)
				}
			}
			buildOperation(t, handler, "/end", map[string]string{"handle": answer.Handle}, nil)
			if len(sessions.entries) != 0 {
				t.Fatal("refused request leaked")
			}
		})
	}
}

func TestPrerenderInternalFetchCookiesKeepScopeAndTombstones(t *testing.T) {
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		if err := event.SetCookie("erase", "original", CookieOptions{Path: "/"}); err != nil {
			return nil, err
		}
		response, err := resolve(ctx, event)
		if err == nil {
			if _, seen := event.Cookie("response-only"); seen {
				return nil, fmt.Errorf("arbitrary response header mutated event cookies")
			}
		}
		return response, err
	})
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		event := EventFrom(ctx)
		flavor, _ := event.Cookie("flavor")
		_, erase := event.Cookie("erase")
		_, hidden := event.Cookie("hidden")
		_, foreign := event.Cookie("foreign")
		return map[string]any{"flavor": flavor, "erase": erase, "hidden": hidden, "foreign": foreign}, nil
	}})
	sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
	defer sessions.close()
	handler := prerenderHandler("secret", nil, []*ServerLoad{load}, nil, nil, sessions)
	var begin prerenderRequestAnswer
	buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/lifecycle/alpha"}, &begin)
	buildOperation(t, handler, "/cookies", map[string]any{"handle": begin.Handle, "url": "http://app.test/cookies/set", "cookies": []string{
		"flavor=root; Path=/; HttpOnly",
		"flavor=scoped%20raw; Path=/lifecycle; Domain=app.test; SameSite=Strict; Secure; Max-Age=60",
		"hidden=wrong-path; Path=/outside", "foreign=wrong-domain; Domain=other.test; Path=/", "erase=; Path=/; Max-Age=0",
	}}, nil)
	var output prerenderLoadOutput
	buildOperation(t, handler, "/load", map[string]any{"handle": begin.Handle, "module": "page", "url": "http://app.test/lifecycle/alpha"}, &output)
	if output.Error != nil || string(output.Data) != `[{"erase":1,"flavor":2,"foreign":1,"hidden":1},false,"scoped%20raw"]` {
		t.Fatalf("load cookie values: %s %v", output.Data, output.Error)
	}
	var scoped, deletion *prerenderCookie
	for i := range output.Cookies {
		cookie := &output.Cookies[i]
		if cookie.Name == "flavor" && cookie.Path == "/lifecycle" {
			scoped = cookie
		}
		if cookie.Name == "erase" {
			deletion = cookie
		}
	}
	if scoped == nil || scoped.Value != "scoped%20raw" || scoped.Domain != "app.test" || !scoped.Raw || !scoped.Secure || scoped.SameSite != int(http.SameSiteStrictMode) || scoped.MaxAge == nil || *scoped.MaxAge != 60 {
		t.Fatalf("scoped cookie attributes: %+v", scoped)
	}
	if deletion == nil || !deletion.Raw || deletion.MaxAge == nil || *deletion.MaxAge != 0 {
		t.Fatalf("deletion tombstone: %+v", deletion)
	}
	var final prerenderRequestAnswer
	buildOperation(t, handler, "/response", map[string]any{"handle": begin.Handle, "response": prerenderResponse{Status: 200, Headers: http.Header{"Set-Cookie": {"response-only=header; Path=/"}}, Body: []byte("actual body")}}, &final)
	if final.Response == nil || final.Response.Status != 200 || string(final.Response.Body) != "actual body" {
		t.Fatalf("after-hook response: %+v", final)
	}
	buildOperation(t, handler, "/end", map[string]string{"handle": begin.Handle}, nil)
}
func TestPrerenderEndCancelsDeferredCallbackAndReleasesState(t *testing.T) {
	entered := make(chan struct{})
	var after atomic.Bool
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		response, err := resolve(ctx, event)
		after.Store(true)
		return response, err
	})
	load := NewServerLoad(LoadSpec{Module: "deferred", Run: func(ctx context.Context) (any, error) {
		return map[string]any{"later": Async(ctx, func(ctx context.Context) (string, error) { close(entered); <-ctx.Done(); return "", ctx.Err() })}, nil
	}})
	sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
	defer sessions.close()
	handler := prerenderHandler("secret", nil, []*ServerLoad{load}, nil, nil, sessions)
	var answer prerenderRequestAnswer
	buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/page"}, &answer)
	done := make(chan struct{})
	go func() {
		defer close(done)
		buildOperation(t, handler, "/load", map[string]any{"handle": answer.Handle, "module": "deferred", "url": "http://app.test/page"}, nil)
	}()
	<-entered
	ended := make(chan struct{})
	go func() {
		defer close(ended)
		buildOperation(t, handler, "/end", map[string]string{"handle": answer.Handle}, nil)
	}()
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("cancellation failed to drain request")
	}
	<-done
	if after.Load() {
		t.Fatal("cancelled resolve supplied a fabricated response to after logic")
	}
	if len(sessions.entries) != 0 {
		t.Fatal("cancelled request leaked")
	}
}

type brokenPrerenderBody struct{}

func (brokenPrerenderBody) Read([]byte) (int, error) { return 0, fmt.Errorf("literal body failure") }
func (brokenPrerenderBody) Close() error             { return nil }
func TestPrerenderBodyFailureCannotReturnSuccessfulTruncatedResponse(t *testing.T) {
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		return NewResponse(200, nil, brokenPrerenderBody{}), nil
	})
	sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
	defer sessions.close()
	raw, _ := json.Marshal(prerenderRequestInput{Method: "GET", URL: "http://app.test/"})
	request := httptest.NewRequest("POST", "/begin", bytes.NewReader(raw))
	request.Header.Set("Authorization", "Bearer secret")
	response := httptest.NewRecorder()
	prerenderHandler("secret", nil, nil, nil, nil, sessions).ServeHTTP(response, request)
	if response.Code != 500 || !strings.Contains(response.Body.String(), "literal body failure") || len(sessions.entries) != 0 {
		t.Fatalf("body error lost or request leaked: %d %s entries=%d", response.Code, response.Body.String(), len(sessions.entries))
	}
}

func TestPrerenderKeepsKitLogicalURLAndOriginalRequestDistinct(t *testing.T) {
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		if event.URL().String() != "https://public.test/page?keep=1" || event.Request().URL.String() != "http://kit-transport.test/page/__data.json?keep=1" || !event.IsDataRequest() || !event.IsSubRequest() {
			return nil, Errorf(500, "logical metadata lost")
		}
		return NewResponse(200, nil, strings.NewReader("literal metadata preserved")), nil
	})
	sessions := &prerenderRequests{ctx: context.Background(), options: PrerenderServiceOptions{BindRequest: buildBoundary(hook)}, entries: map[string]*prerenderRequest{}}
	defer sessions.close()
	var answer prerenderRequestAnswer
	buildOperation(t, prerenderHandler("secret", nil, nil, nil, nil, sessions), "/begin", prerenderRequestInput{Method: "GET", URL: "http://kit-transport.test/page/__data.json?keep=1", LogicalURL: "https://public.test/page?keep=1", IsSubRequest: true}, &answer)
	if answer.Response == nil || string(answer.Response.Body) != "literal metadata preserved" {
		t.Fatalf("metadata: %+v", answer)
	}
	sessions.end(answer.Handle)
}
