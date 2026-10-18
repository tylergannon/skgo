package skgo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPrerenderFetchRefusesUnavailableDestinationsAndKeepsEndpointIsolation(t *testing.T) {
	manifest := Manifest{Base: "/base", AppDir: "assets", Routes: []ManifestRoute{
		{ID: "/page", Pattern: `^/page/?$`, Page: &ManifestPage{Leaf: 0}},
		{ID: "/page.html", Pattern: `^/page\.html$`, Page: &ManifestPage{Leaf: 0}},
		{ID: "/matched/[value=Number]", Pattern: `^/matched/([^/]+?)/?$`, Params: []ManifestParam{{Name: "value", Matcher: "Number"}}, Page: &ManifestPage{Leaf: 0}},
		{ID: "/shared", Pattern: `^/shared/?$`, Page: &ManifestPage{Leaf: 0}, Endpoint: &ManifestEndpoint{Methods: []string{"GET", "POST", "PUT"}}},
	}}
	hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
		event.Locals = &buildLocals{Value: event.URL().Path, Calls: 10}
		return resolve(ctx, event)
	})
	endpoint := func(w http.ResponseWriter, r *http.Request) {
		locals := RequestLocals[buildLocals](r.Context())
		if locals.Value != "/base/shared" || locals.Calls != 10 {
			t.Errorf("fetch inherited caller locals: %+v", locals)
		}
		locals.Calls++
		fmt.Fprint(w, "literal-endpoint-own-locals-10")
	}
	cases := []struct {
		method, path, accept, refusal string
		status                        int
		body                          string
	}{
		{"GET", "/base/page", "", "page destination GET /base/page", 0, ""},
		{"HEAD", "/base/page", "", "page destination HEAD /base/page", 0, ""},
		{"POST", "/base/page", "", "page destination POST /base/page", 0, ""},
		{"GET", "/base/page/__data.json", "", "data destination GET /base/page/__data.json", 0, ""},
		{"GET", "/base/page.html__data.json", "", "data destination GET /base/page.html__data.json", 0, ""},
		{"GET", "/base/assets/remote/known/value", "", "remote destination GET /base/assets/remote/known/value", 0, ""},
		{"GET", "/base/shared", "text/html", "page destination GET /base/shared", 0, ""},
		{"POST", "/base/shared", "text/html", "page destination POST /base/shared", 0, ""},
		{"GET", "/base/shared", "application/json", "", 200, "literal-endpoint-own-locals-10"},
		{"HEAD", "/base/shared", "application/json", "", 200, ""},
		{"POST", "/base/shared", "application/json", "", 200, "literal-endpoint-own-locals-10"},
		{"PUT", "/base/shared", "text/html", "", 200, "literal-endpoint-own-locals-10"},
		{"DELETE", "/base/shared", "application/json", "", 405, "DELETE method not allowed"},
		{"GET", "/base/matched/42", "", "page destination GET /base/matched/42", 0, ""},
		{"GET", "/base/matched/no", "", "", 404, "404 page not found\n"},
		{"GET", "/base/absent", "", "", 404, "404 page not found\n"},
		{"GET", "/base/absent/__data.json", "", "", 404, "404 page not found\n"},
	}
	load := NewServerLoad(LoadSpec{Module: "page", Run: func(ctx context.Context) (any, error) {
		locals := RequestLocals[buildLocals](ctx)
		locals.Calls = 88
		for _, tc := range cases {
			request, _ := http.NewRequest(tc.method, tc.path, nil)
			if tc.accept != "" {
				request.Header.Set("Accept", tc.accept)
			}
			response, err := EventFrom(ctx).Fetch(ctx, request)
			if tc.refusal != "" {
				want := "skgo: prerender Event.Fetch cannot dispatch " + tc.refusal + ": Kit request context is unavailable"
				if err == nil || err.Error() != want || response != nil {
					t.Errorf("%s %s: response=%v err=%v, want %q", tc.method, tc.path, response, err, want)
				}
				continue
			}
			if err != nil {
				t.Errorf("%s %s: %v", tc.method, tc.path, err)
				continue
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil || response.StatusCode != tc.status || string(body) != tc.body {
				t.Errorf("%s %s: status=%d body=%q err=%v", tc.method, tc.path, response.StatusCode, body, err)
			}
		}
		if locals.Calls != 88 {
			t.Errorf("subrequest changed caller locals: %+v", locals)
		}
		return "literal-all-fetches-completed", nil
	}})
	sessions := &prerenderRequests{ctx: context.Background(), entries: map[string]*prerenderRequest{}, options: PrerenderServiceOptions{BindRequest: buildBoundary(hook), Matchers: map[string]ParamMatcher{"Number": func(raw string) (any, bool) { return raw, raw == "42" }}, Endpoints: []*Endpoint{NewEndpoint("/shared", "GET", endpoint), NewEndpoint("/shared", "POST", endpoint), NewEndpoint("/shared", "PUT", endpoint)}}}
	defer sessions.close()
	handler := prerenderHandler("secret", nil, []*ServerLoad{load}, nil, nil, sessions)
	var begin prerenderRequestAnswer
	buildOperation(t, handler, "/begin", prerenderRequestInput{URL: "http://app.test/base/source", Method: "GET", Manifest: manifest}, &begin)
	var output prerenderLoadOutput
	buildOperation(t, handler, "/load", map[string]any{"handle": begin.Handle, "module": "page", "url": "http://app.test/base/source"}, &output)
	if output.Error != nil || !strings.Contains(string(output.Data), "literal-all-fetches-completed") {
		t.Fatalf("load did not complete: %+v", output)
	}
	buildOperation(t, handler, "/end", map[string]string{"handle": begin.Handle}, nil)
}
