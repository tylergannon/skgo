package skgo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/tylergannon/polytype/devalue"
)

func TestPrerenderTypedLoadConvertsRawPathToNamedMatcherResult(t *testing.T) {
	const module = "src/routes/orders/[n=Number]/+page.server.ts"
	load := NewServerLoad(LoadSpec{Module: module, Matchers: map[string]ParamMatcher{
		"Number": func(raw string) (any, bool) {
			n, err := strconv.ParseInt(raw, 10, 64)
			return fixtureNumber(n), err == nil
		},
	}, Run: func(ctx context.Context) (any, error) {
		return struct {
			Label string `json:"label"`
		}{LoadParamValue[fixtureNumber](EventFrom(ctx), "n").Label()}, nil
	}})
	answer := callPrerenderLoad(t, PrerenderLoadInput{Module: module, URL: "http://example.test/orders/00042", RouteID: "/orders/[n=Number]",
		RoutePath: "/orders/00042", RoutePattern: `^/orders/([^/]+?)/?$`, RouteParams: []ManifestParam{{Name: "n", Matcher: "Number"}},
	}, load)
	value, err := devalue.Parse(string(answer.Data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := field(t, value, "label"); got != "number:42" {
		t.Fatalf("named matcher method = %v, want number:42", got)
	}
}

func callPrerenderLoad(t *testing.T, input PrerenderLoadInput, load *ServerLoad) prerenderLoadOutput {
	t.Helper()
	request, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := RunPrerenderLoad(bytes.NewReader(request), &output, nil, load); err != nil {
		t.Fatal(err)
	}
	var answer prerenderLoadOutput
	if err := json.Unmarshal(output.Bytes(), &answer); err != nil {
		t.Fatalf("invalid build bridge answer %s: %v", output.String(), err)
	}
	return answer
}

func TestPrerenderLoadReceivesKitsEventAndReturnsGoEffects(t *testing.T) {
	const module = "src/routes/items/[id]/+page.server.ts"
	load := NewServerLoad(LoadSpec{Module: module, Run: func(ctx context.Context) (any, error) {
		e := EventFrom(ctx)
		parent, err := Parent[struct {
			Name string `json:"name"`
		}](ctx)
		if err != nil {
			return nil, err
		}
		cookie, _ := e.Cookie("incoming")
		if err := e.SetHeader("X-Prerender", "from Go"); err != nil {
			return nil, err
		}
		if err := e.SetCookie("seen", "yes", CookieOptions{Path: "/"}); err != nil {
			return nil, err
		}
		return struct {
			Receipt string `json:"receipt"`
		}{parent.Name + ":" + e.Param("id") + ":" + e.RouteID() + ":" + e.URL().Query().Get("mode") + ":" + cookie}, nil
	}})
	answer := callPrerenderLoad(t, PrerenderLoadInput{
		Module: module, URL: "http://example.test/items/42?mode=preview", RouteID: "/items/[id]",
		Params: map[string]string{"id": "42"}, Parent: map[string]any{"name": "parent"},
		Headers: http.Header{"Cookie": {"incoming=hello"}},
	}, load)
	if answer.Failure != nil {
		t.Fatalf("successful Go load acquired a build failure diagnostic: %q", *answer.Failure)
	}
	value, err := devalue.Parse(string(answer.Data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := field(t, value, "receipt"); got != "parent:42:/items/[id]:preview:hello" {
		t.Errorf("Go load receipt = %v", got)
	}
	if got := answer.Headers.Get("X-Prerender"); got != "from Go" {
		t.Errorf("response header = %q", got)
	}
	if len(answer.Cookies) != 1 || answer.Cookies[0].Name != "seen" || answer.Cookies[0].Value != "yes" {
		t.Errorf("response cookies = %+v", answer.Cookies)
	}
}

func TestPrerenderLoadReturnsGoErrorAndRedirect(t *testing.T) {
	const module = "src/routes/decide/+page.server.ts"
	for _, test := range []struct {
		name string
		err  error
	}{
		{"error", Errorf(403, "forbidden")},
		{"redirect", &Redirect{Status: 303, Location: "/elsewhere"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			load := NewServerLoad(LoadSpec{Module: module, Run: func(context.Context) (any, error) {
				return nil, test.err
			}})
			answer := callPrerenderLoad(t, PrerenderLoadInput{
				Module: module, URL: "http://example.test/decide", RouteID: "/decide",
			}, load)
			if test.name == "error" && (answer.Error == nil || answer.Error.Status != 403 || answer.Error.Message != "forbidden") {
				t.Errorf("Go error = %+v", answer.Error)
			}
			if answer.Failure != nil {
				t.Errorf("intentional %s acquired an unexpected build failure diagnostic: %q", test.name, *answer.Failure)
			}
			if test.name == "redirect" && (answer.Redirect == nil || answer.Redirect.Status != 303 || answer.Redirect.Location != "/elsewhere") {
				t.Errorf("Go redirect = %+v", answer.Redirect)
			}
		})
	}
}

func TestPrerenderLoadPreservesUnexpectedFailureForBuildDiagnostic(t *testing.T) {
	const module = "src/routes/decide/+page.server.ts"
	const message = "inventory database unavailable"
	load := NewServerLoad(LoadSpec{Module: module, Run: func(context.Context) (any, error) {
		return nil, errors.New(message)
	}})
	answer := callPrerenderLoad(t, PrerenderLoadInput{
		Module: module, URL: "http://example.test/decide", RouteID: "/decide",
	}, load)
	if answer.Error == nil || answer.Error.Status != http.StatusInternalServerError || answer.Error.Message != "Internal Error" {
		t.Fatalf("unexpected Go failure changed the Kit error payload: %+v", answer.Error)
	}
	if answer.Failure == nil || *answer.Failure != message {
		t.Fatalf("build diagnostic failure = %v, want %q", answer.Failure, message)
	}
}
