package skgo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

// This registration has the same decode-before-call boundary the generator
// emits; the application body is counted separately from decoder refusals.
func validationFixture(kind Kind, called *int) *Remote {
	decode := func(call Call) error {
		if err := RequireArgument(call); err != nil {
			return err
		}
		if _, ok := call.Arg.(string); !ok {
			return BadRequest(errors.New("private /account/token: expected string"))
		}
		return nil
	}
	return NewRemote(RemoteSpec{Module: testModule, Name: "validation", Kind: kind,
		Call: func(_ context.Context, call Call) (any, error) {
			if err := decode(call); err != nil {
				return nil, err
			}
			*called++
			return "accepted", nil
		},
		Batch: func(_ context.Context, calls []Call) ([]any, error) {
			for _, call := range calls {
				if err := decode(call); err != nil {
					return nil, err
				}
			}
			*called++
			return []any{"accepted"}, nil
		},
		Live: func(_ context.Context, call Call, yield func(any) error) error {
			if err := decode(call); err != nil {
				return err
			}
			*called++
			return yield("accepted")
		},
	})
}

func TestRemoteValidationHookAtHandler(t *testing.T) {
	for _, kind := range []Kind{KindQuery, KindCommand, KindBatch, KindLive, KindPrerender} {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			called, hooked := 0, 0
			fn := validationFixture(kind, &called)
			rs := testRemotes(t, RemoteConfig{Development: true, HandleError: func(ctx context.Context, caught CaughtError) map[string]any {
				hooked++
				if caught.Kind != "validation" || caught.Status != 400 || caught.Message != "Bad Request" || caught.Err != nil || !reflect.DeepEqual(caught.Issues, []Issue{{Message: "private /account/token: expected string"}}) {
					t.Fatalf("caught=%+v", caught)
				}
				if ev := EventFrom(ctx); ev == nil || ev.mutable {
					t.Fatal("hook requires read-only request event")
				}
				return map[string]any{"status": 422, "message": "Please update this application", "code": "STALE_671"}
			}}, fn)
			path := rs.Prefix() + fn.ID() + "?payload=WzQyXQ"
			method, body := "GET", ""
			if kind == KindCommand {
				method, body = "POST", `{"payload":"WzQyXQ"}`
			}
			if kind == KindBatch {
				method, body = "POST", `{"payloads":["WzQyXQ"]}`
			}
			if kind == KindPrerender {
				path = rs.Prefix() + fn.ID() + "/WzQyXQ"
			}
			r := httptest.NewRequest(method, path, strings.NewReader(body))
			if kind == KindCommand {
				r.Header.Set("Referer", "http://example.com/")
			}
			w := httptest.NewRecorder()
			rs.ServeHTTP(w, r)
			want := `{"error":{"code":"STALE_671","message":"Please update this application","status":422},"type":"error"}`
			got := strings.TrimSpace(w.Body.String())
			if kind == KindLive {
				got = strings.TrimSpace(strings.TrimPrefix(got, "data: "))
			}
			var actual, expected any
			if json.Unmarshal([]byte(got), &actual) != nil || json.Unmarshal([]byte(want), &expected) != nil || !reflect.DeepEqual(actual, expected) || w.Code != 200 || called != 0 || hooked != 1 {
				t.Fatalf("status=%d called=%d hooked=%d body=%s", w.Code, called, hooked, got)
			}
		})
	}
}

func TestRemoteValidationDefaultsAndMissingArgument(t *testing.T) {
	for _, mode := range []string{"absent", "nil", "partial", "panic"} {
		for _, query := range []string{"", "?payload=WzQyXQ"} {
			t.Run(mode+query, func(t *testing.T) {
				called, hooked := 0, 0
				fn := validationFixture(KindQuery, &called)
				var hook HandleError
				if mode != "absent" {
					hook = func(_ context.Context, caught CaughtError) map[string]any {
						hooked++
						if caught.Kind != "validation" || len(caught.Issues) != 1 {
							t.Fatalf("caught=%+v", caught)
						}
						if mode == "panic" {
							panic("private-hook-failure")
						}
						if mode == "partial" {
							return map[string]any{"code": "RETRY_781"}
						}
						return nil
					}
				}
				rs := testRemotes(t, RemoteConfig{HandleError: hook}, fn)
				w := httptest.NewRecorder()
				rs.ServeHTTP(w, httptest.NewRequest("GET", rs.Prefix()+fn.ID()+query, nil))
				_, _, body := envelope(t, w.Body.Bytes())
				want := map[string]any{"status": float64(400), "message": "Bad Request"}
				if mode == "panic" {
					want["message"] = "Internal Error"
				}
				if mode == "partial" {
					want["code"] = "RETRY_781"
				}
				if !reflect.DeepEqual(body, want) || called != 0 || (mode != "absent" && hooked != 1) {
					t.Fatalf("body=%v called=%d hooked=%d", body, called, hooked)
				}
			})
		}
	}
}

func TestNativeValidationHookAtCallbackAndFrontDoor(t *testing.T) {
	called, hooked := 0, 0
	fn := validationFixture(KindQuery, &called)
	remotes := testRemotes(t, RemoteConfig{HandleError: func(_ context.Context, caught CaughtError) map[string]any {
		hooked++
		if caught.Kind != "validation" || len(caught.Issues) != 1 {
			t.Fatalf("caught=%+v", caught)
		}
		return map[string]any{"message": "Update the app", "code": "PUBLIC_229"}
	}}, fn)
	loads, err := NewLoads(LoadConfig{HandleError: func(context.Context, CaughtError) map[string]any { t.Fatal("validation used page hook"); return nil }})
	if err != nil {
		t.Fatal(err)
	}
	development := NewNativeDevelopment("secret")
	if _, err := development.NewSSR(fstest.MapFS{}, Manifest{}, loads, remotes, SSROptions{}); err != nil {
		t.Fatal(err)
	}
	pages, err := development.NewStaticHandler(fstest.MapFS{})
	if err != nil {
		t.Fatal(err)
	}
	handler, cleanup, err := development.Handler(loads.Intercept(remotes.Intercept(pages)))
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	handle := beginNative(t, handler)
	var answer struct {
		Type string         `json:"type"`
		Body map[string]any `json:"body"`
	}
	buildOperation(t, handler, "/_skgo/dev/remote", map[string]string{"handle": handle, "module": testModule, "name": "validation", "payload": "WzQyXQ"}, &answer)
	want := map[string]any{"status": float64(400), "message": "Update the app", "code": "PUBLIC_229"}
	if answer.Type != "error" || !reflect.DeepEqual(answer.Body, want) {
		t.Fatalf("callback=%+v", answer)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", remotes.Prefix()+fn.ID()+"?payload=WzQyXQ", nil))
	_, _, body := envelope(t, w.Body.Bytes())
	if w.Code != http.StatusOK || !reflect.DeepEqual(body, want) || called != 0 || hooked != 2 {
		t.Fatalf("front door=%s called=%d hooked=%d", w.Body, called, hooked)
	}
	buildOperation(t, handler, "/_skgo/dev/end", map[string]string{"handle": handle}, nil)
}

func TestRemoteValidationHookDoesNotReceiveProtocolRefusals(t *testing.T) {
	fn := NewQueryNoArg(testModule, "noArgument", func(context.Context) (string, error) {
		t.Fatal("rejected remote body ran")
		return "", nil
	})
	rs := testRemotes(t, RemoteConfig{HandleError: func(context.Context, CaughtError) map[string]any {
		t.Fatal("protocol refusal was classified as schema validation")
		return nil
	}}, fn)
	for _, query := range []string{"?payload=WzQyXQ", "?payload=not-devalue"} {
		w := httptest.NewRecorder()
		rs.ServeHTTP(w, httptest.NewRequest("GET", rs.Prefix()+fn.ID()+query, nil))
		_, _, body := envelope(t, w.Body.Bytes())
		if !reflect.DeepEqual(body, map[string]any{"status": float64(400), "message": "Bad Request"}) {
			t.Fatal(body)
		}
	}
}
