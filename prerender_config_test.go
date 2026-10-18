package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestPrerenderConfiguredErrorPolicyUsesLogicalRequest(t *testing.T) {
	for _, surface := range []string{"load", "render", "refusal"} {
		t.Run(surface, func(t *testing.T) {
			calls := 0
			policy := func(ctx context.Context, caught CaughtError) map[string]any {
				calls++
				if RequestLocals[buildLocals](ctx).Value != "literal-locals-278" || EventFrom(ctx).RouteID() != "/policy" {
					t.Error("error hook lost originating event or locals")
				}
				if caught.Kind != "unknown" || caught.Err == nil || !strings.Contains(caught.Err.Error(), "literal-private-278") {
					t.Errorf("original failure missing: %+v", caught)
				}
				return map[string]any{"message": "literal-safe-278", "supportId": "literal-public-support-278"}
			}
			hook := RequestMiddleware[struct{}, buildLocals](func(ctx context.Context, event RequestEvent[struct{}, buildLocals], resolve RequestResolve[struct{}, buildLocals]) (*http.Response, error) {
				event.Locals.Value = "literal-locals-278"
				if surface == "refusal" {
					return nil, errors.New("literal-private-278")
				}
				return resolve(ctx, event)
			})
			options := PrerenderServiceOptions{BindRequest: buildBoundary(hook), HandleError: policy}
			sessions := &prerenderRequests{ctx: context.Background(), options: options, entries: map[string]*prerenderRequest{}}
			defer sessions.close()
			load := NewServerLoad(LoadSpec{Module: "page", Run: func(context.Context) (any, error) { return nil, errors.New("literal-private-278") }})
			handler := prerenderHandler("secret", nil, []*ServerLoad{load}, nil, nil, sessions)
			var begin prerenderRequestAnswer
			buildOperation(t, handler, "/begin", prerenderRequestInput{Method: "GET", URL: "http://app.test/policy", RouteID: "/policy"}, &begin)
			var public json.RawMessage
			switch surface {
			case "refusal":
				public = begin.Body
			case "load":
				var answer prerenderLoadOutput
				buildOperation(t, handler, "/load", map[string]any{"handle": begin.Handle, "module": "page", "url": "http://app.test/policy", "routeId": "/policy"}, &answer)
				if answer.Failure != nil {
					t.Fatalf("configured safe body bypassed by private diagnostic: %+v", answer)
				}
				public = answer.Body
			case "render":
				buildOperation(t, handler, "/error", map[string]any{"handle": begin.Handle, "kind": "unknown", "cause": "literal-private-278", "body": map[string]any{"status": 500, "message": "literal-private-278"}}, &public)
			}
			if calls != 1 {
				t.Fatalf("error hook called %d times; want 1", calls)
			}
			if string(public) != `{"message":"literal-safe-278","status":500,"supportId":"literal-public-support-278"}` {
				t.Fatalf("public error body: %s", public)
			}
			buildOperation(t, handler, "/end", map[string]any{"handle": begin.Handle}, nil)
		})
	}
}
