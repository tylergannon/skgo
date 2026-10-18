package skgo

import (
	"context"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRemoteSchemaValidationHookCarriers(t *testing.T) {
	for _, kind := range []Kind{KindQuery, KindCommand, KindBatch, KindLive, KindPrerender} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			bodies, hooks := 0, 0
			check := func(call Call) error {
				if call.Arg != "valid" {
					return call.RejectArgument(fmt.Errorf("private catalog schema /name"))
				}
				return nil
			}
			fn := NewRemote(RemoteSpec{Kind: kind, Module: testModule, Name: "validation", Call: func(ctx context.Context, c Call) (any, error) {
				if err := check(c); err != nil {
					return nil, err
				}
				bodies++
				return "valid-body-441", nil
			}, Batch: func(ctx context.Context, calls []Call) ([]any, error) {
				for _, c := range calls {
					if err := check(c); err != nil {
						return nil, err
					}
				}
				bodies++
				return []any{"valid-body-441"}, nil
			}, Live: func(ctx context.Context, c Call, yield func(any) error) error {
				if err := check(c); err != nil {
					return err
				}
				bodies++
				return yield("valid-body-441")
			}})
			hook := func(ctx context.Context, c CaughtError) map[string]any {
				hooks++
				if c.Kind != "validation" || c.Status != 400 || c.Message != "Bad Request" || c.Err != nil || len(c.Issues) != 1 || c.Issues[0].Message != "private catalog schema /name" {
					t.Fatalf("caught=%+v", c)
				}
				return map[string]any{"message": "Choose a catalog name", "reference": "validation-552"}
			}
			rs := testRemotes(t, RemoteConfig{Development: true, HandleError: hook}, fn)
			request := func(value string) *httptest.ResponseRecorder {
				encoded := payload(t, value)
				path := rs.Prefix() + fn.id
				method, body := "GET", ""
				switch kind {
				case KindCommand:
					method = "POST"
					body = `{"payload":"` + encoded + `"}`
				case KindBatch:
					method = "POST"
					body = `{"payloads":["` + encoded + `"]}`
				case KindPrerender:
					path += "/" + encoded
				default:
					path += "?payload=" + encoded
				}
				req := httptest.NewRequest(method, path, strings.NewReader(body))
				rec := httptest.NewRecorder()
				rs.ServeHTTP(rec, req)
				return rec
			}
			rec := request("invalid")
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"message":"Choose a catalog name"`) || !strings.Contains(rec.Body.String(), `"reference":"validation-552"`) || strings.Contains(rec.Body.String(), "private catalog") || hooks != 1 || bodies != 0 {
				t.Fatalf("invalid status=%d body=%s hooks=%d bodies=%d", rec.Code, rec.Body.String(), hooks, bodies)
			}
			rec = request("valid")
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "valid-body-441") || bodies != 1 || hooks != 1 {
				t.Fatalf("valid status=%d body=%s hooks=%d bodies=%d", rec.Code, rec.Body.String(), hooks, bodies)
			}
		})
	}
}

func TestRemoteSchemaValidationDefaultsAndClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		hook HandleError
		want string
	}{
		{"absent", nil, `"message":"Bad Request"`},
		{"nil", func(context.Context, CaughtError) map[string]any { return nil }, `"message":"Bad Request"`},
		{"partial", func(context.Context, CaughtError) map[string]any { return map[string]any{"reference": "partial-773"} }, `"reference":"partial-773"`},
		{"panic", func(context.Context, CaughtError) map[string]any { panic("private hook panic") }, `"message":"Internal Error"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fn := NewRemote(RemoteSpec{Kind: KindQuery, Module: testModule, Name: "reject", Call: func(context.Context, Call) (any, error) {
				return nil, (Call{}).RejectArgument(fmt.Errorf("private decoder"))
			}})
			rs := testRemotes(t, RemoteConfig{HandleError: tc.hook}, fn)
			rec := httptest.NewRecorder()
			rs.ServeHTTP(rec, httptest.NewRequest("GET", rs.Prefix()+fn.id, nil))
			if !strings.Contains(rec.Body.String(), tc.want) || !strings.Contains(rec.Body.String(), `"status":400`) || strings.Contains(rec.Body.String(), "private") {
				t.Fatal(rec.Body.String())
			}
		})
	}
	hooks := 0
	hook := func(context.Context, CaughtError) map[string]any {
		hooks++
		return map[string]any{"message": "WRONG classification"}
	}
	for _, returned := range []error{BadRequest(fmt.Errorf("application bad request")), Errorf(400, "ordinary app error")} {
		fn := NewRemote(RemoteSpec{Kind: KindQuery, Module: testModule, Name: "app", Call: func(context.Context, Call) (any, error) { return nil, returned }})
		rs := testRemotes(t, RemoteConfig{HandleError: hook}, fn)
		for _, query := range []string{"", "?payload=malformed"} {
			rec := httptest.NewRecorder()
			rs.ServeHTTP(rec, httptest.NewRequest("GET", rs.Prefix()+fn.id+query, nil))
			if rec.Code != 200 || strings.Contains(rec.Body.String(), "WRONG") {
				t.Fatal(rec.Body.String())
			}
		}
	}
	if hooks != 0 {
		t.Fatalf("non-schema errors called schema hook %d times", hooks)
	}
	form := NewForm(testModule, "formIssues", func(context.Context, struct {
		From string `json:"from"`
	}) (string, error) {
		return "", Invalidf("from", "business issue-884")
	})
	rs := testRemotes(t, RemoteConfig{HandleError: hook}, form)
	rec := formRequest(t, rs, form, formGoldenSubmission)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "business issue-884") || hooks != 0 {
		t.Fatalf("form=%s hooks=%d", rec.Body.String(), hooks)
	}
	if caught, ok := validationCaught(RefuseArgument(Call{Present: true})); ok {
		t.Fatalf("no-schema argument refusal classified validation: %+v", caught)
	}
}

func TestNativeSchemaValidationHandledOnce(t *testing.T) {
	calls := 0
	fn := NewRemote(RemoteSpec{Kind: KindQuery, Module: testModule, Name: "nativeValidation", Call: func(context.Context, Call) (any, error) {
		return nil, (Call{}).RejectArgument(fmt.Errorf("private native issue"))
	}})
	handler, close := nativeRemoteFixtureWithHook(t, func(ctx context.Context, c CaughtError) map[string]any {
		calls++
		if c.Kind != "validation" {
			t.Fatalf("kind=%s", c.Kind)
		}
		return map[string]any{"message": "native validation-441", "reference": "native-552"}
	}, fn)
	defer close()
	// Use the same configured app hook in the native renderer's remote registry.
	// The private callback preserves the handled public body for Kit.
	handle := beginNative(t, handler)
	var answer struct {
		Type  string
		Body  map[string]any
		Error *HTTPError
	}
	buildOperation(t, handler, "/_skgo/dev/remote", map[string]string{"handle": handle, "module": testModule, "name": "nativeValidation"}, &answer)
	if answer.Type != "error" || answer.Error.Status != 400 || calls != 1 || answer.Body["message"] != "native validation-441" || answer.Body["reference"] != "native-552" {
		t.Fatalf("answer=%+v", answer)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}
