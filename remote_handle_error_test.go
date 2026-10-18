package skgo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/formdata"
	"github.com/tylergannon/skgo/internal/ssr"
)

func TestRemoteFailuresUseApplicationErrorPolicy(t *testing.T) {
	for _, kind := range []Kind{KindQuery, KindBatch, KindCommand, KindForm, KindLive, KindPrerender} {
		for _, outcome := range []string{"returned", "panic", "known", "handled", "redirect"} {
			for _, hooked := range []bool{false, true} {
				name := remoteTraceKind(kind) + "/" + outcome
				if hooked {
					name += "/hook"
				}
				t.Run(name, func(t *testing.T) {
					private := errors.New("literal-private-cause-354")
					fail := func() error {
						switch outcome {
						case "panic":
							panic(private)
						case "known":
							return Errorf(409, "literal-known-message-354")
						case "handled":
							return &ssr.HandledError{Body: &ssr.Error{Status: 410, Message: "literal-recorded-message-354", Extra: map[string]any{"supportId": "literal-recorded-id-354"}}}
						case "redirect":
							return &Redirect{Status: 303, Location: "/literal-next-354"}
						default:
							return private
						}
					}
					fn := NewRemote(RemoteSpec{Kind: kind, Module: testModule, Name: "failure",
						Call:  func(context.Context, Call) (any, error) { return nil, fail() },
						Batch: func(context.Context, []Call) ([]any, error) { return nil, fail() },
						Live:  func(context.Context, Call, func(any) error) error { return fail() },
					})
					calls, panics := 0, 0
					cfg := RemoteConfig{Development: kind == KindPrerender, OnPanic: func(id string, value any, stack []byte) {
						panics++
						if id != fn.ID() || value != private || !strings.Contains(string(stack), "remote_handle_error_test.go") {
							t.Errorf("panic report lost original cause/identity/stack: %s %v %s", id, value, stack)
						}
					}}
					if hooked {
						cfg.HandleError = func(ctx context.Context, caught CaughtError) map[string]any {
							calls++
							event := EventFrom(ctx)
							if event == nil || event.Request().Header.Get("X-Policy-Probe") != "literal-event-354" {
								t.Error("error hook lost its remote request event")
							} else if err := event.SetCookie("hook-cookie", "forbidden", CookieOptions{}); err == nil {
								t.Error("error hook received a mutable event")
							}
							if outcome == "known" {
								if caught.Kind != "app" || caught.Status != 409 || caught.Message != "literal-known-message-354" || caught.Err != nil {
									t.Errorf("known error classification: %+v", caught)
								}
								return map[string]any{"supportId": "literal-hook-id-354"}
							}
							if caught.Kind != "unknown" || caught.Status != 500 || caught.Message != "Internal Error" || caught.Err == nil || !strings.Contains(caught.Err.Error(), private.Error()) {
								t.Errorf("unexpected failure lost private cause or safe fallback: %+v", caught)
							}
							if outcome == "returned" && !errors.Is(caught.Err, private) {
								t.Errorf("returned original error lost: %v", caught.Err)
							}
							return map[string]any{"status": 503, "message": "literal-public-message-354", "supportId": "literal-hook-id-354"}
						}
					}
					rs := testRemotes(t, cfg, fn)
					rec := remotePolicyRequest(t, rs, fn)
					if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" || rec.Header().Get("Set-Cookie") != "" {
						t.Fatalf("remote error envelope headers/status changed: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
					}
					raw := rec.Body.String()
					if strings.Contains(raw, private.Error()) {
						t.Fatalf("private cause leaked: %s", raw)
					}
					if kind == KindLive {
						raw = strings.TrimSpace(strings.TrimPrefix(raw, "data: "))
					}
					if outcome == "redirect" && kind != KindCommand {
						if !strings.Contains(raw, "literal-next-354") || !strings.Contains(raw, `"type":"`+map[bool]string{true: "redirect", false: "result"}[kind == KindLive]+`"`) {
							t.Fatalf("redirect behavior changed: %s", raw)
						}
					} else {
						var got struct {
							Type  string         `json:"type"`
							Error map[string]any `json:"error"`
						}
						if err := json.Unmarshal([]byte(raw), &got); err != nil {
							t.Fatal(err)
						}
						status, message, support := float64(500), "Internal Error", any(nil)
						if outcome == "known" {
							status, message = 409, "literal-known-message-354"
						} else if outcome == "handled" {
							status, message, support = 410, "literal-recorded-message-354", "literal-recorded-id-354"
						}
						if hooked && outcome != "handled" && outcome != "redirect" {
							support = "literal-hook-id-354"
							if outcome != "known" {
								status, message = 503, "literal-public-message-354"
							}
						}
						if got.Type != "error" || got.Error["status"] != status || got.Error["message"] != message || got.Error["supportId"] != support {
							t.Fatalf("public error: %s; want status=%v message=%s supportId=%v", raw, status, message, support)
						}
					}
					wantCalls := 0
					if hooked && outcome != "handled" && outcome != "redirect" {
						wantCalls = 1
					}
					wantPanics := 0
					if outcome == "panic" {
						wantPanics = 1
					}
					if calls != wantCalls || panics != wantPanics {
						t.Fatalf("hook calls=%d panic reports=%d, want %d/%d", calls, panics, wantCalls, wantPanics)
					}
				})
			}
		}
	}
}

func remotePolicyRequest(t *testing.T, rs *Remotes, fn *Remote) *httptest.ResponseRecorder {
	t.Helper()
	method, body, contentType := http.MethodGet, "", "application/json"
	switch fn.kind {
	case KindBatch:
		method, body = http.MethodPost, `{"payloads":[]}`
	case KindCommand:
		method, body = http.MethodPost, `{"payload":""}`
	case KindForm:
		raw, err := base64.StdEncoding.DecodeString(formGoldenSubmission)
		if err != nil {
			t.Fatal(err)
		}
		method, body, contentType = http.MethodPost, string(raw), formdata.ContentType
	}
	r := httptest.NewRequest(method, rs.Prefix()+fn.ID(), strings.NewReader(body))
	r.Header.Set("Content-Type", contentType)
	r.Header.Set("X-Policy-Probe", "literal-event-354")
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, r)
	return rec
}

func TestRemoteHookFailureKeepsSafeFallback(t *testing.T) {
	fn := NewQueryNoArg(testModule, "failure", func(context.Context) (string, error) { return "", Errorf(409, "literal-known-message-354") })
	rs := testRemotes(t, RemoteConfig{HandleError: func(context.Context, CaughtError) map[string]any { panic("literal-private-hook-354") }}, fn)
	rec := remotePolicyRequest(t, rs, fn)
	kind, _, got := envelope(t, rec.Body.Bytes())
	if kind != "error" || got["status"] != float64(409) || got["message"] != "Internal Error" || strings.Contains(rec.Body.String(), "literal-private-hook-354") {
		t.Fatalf("panicking hook lost safe fallback: %s", rec.Body.String())
	}
}

func TestRemoteAbortBypassesErrorPolicyAndPanicReport(t *testing.T) {
	fn := NewQueryNoArg(testModule, "abort", func(context.Context) (string, error) { panic(http.ErrAbortHandler) })
	calls := 0
	rs := testRemotes(t, RemoteConfig{
		HandleError: func(context.Context, CaughtError) map[string]any { calls++; return nil },
		OnPanic:     func(string, any, []byte) { calls++ },
	}, fn)
	defer func() {
		if got := recover(); got != http.ErrAbortHandler || calls != 0 {
			t.Fatalf("abort was handled or reported: panic=%v callback calls=%d", got, calls)
		}
	}()
	remotePolicyRequest(t, rs, fn)
}

func TestRemoteRefreshErrorsKeepCommandAnswerAndPublicFields(t *testing.T) {
	for _, live := range []bool{false, true} {
		for _, panicked := range []bool{false, true} {
			name := "query"
			if live {
				name = "live"
			}
			if panicked {
				name += "/panic"
			}
			t.Run(name, func(t *testing.T) {
				fail := func() error {
					if panicked {
						panic("literal-private-refresh-354")
					}
					return errors.New("literal-private-refresh-354")
				}
				query := func(context.Context) (string, error) { return "", fail() }
				producer := func(context.Context, func(string) error) error { return fail() }
				fn := NewQueryNoArg(testModule, "failure", query)
				if live {
					fn = NewLiveQueryNoArg(testModule, "failure", producer)
				}
				command := NewCommandNoArg(testModule, "act", func(ctx context.Context) (string, error) {
					if live {
						return "literal-command-completed-354", ReconnectRequestedNoArg(ctx, producer)
					}
					return "literal-command-completed-354", RefreshRequestedNoArg(ctx, query)
				})
				calls := 0
				rs := testRemotes(t, RemoteConfig{
					HandleError: func(ctx context.Context, caught CaughtError) map[string]any {
						calls++
						if caught.Kind != "unknown" || caught.Err == nil || EventFrom(ctx) == nil {
							t.Errorf("refresh error lost original error/event: %+v", caught)
						}
						return map[string]any{"supportId": "literal-refresh-hook-354", "message": "literal-refresh-public-354"}
					},
					OnPanic: func(string, any, []byte) {},
				}, fn, command)
				body, _ := json.Marshal(map[string]any{"payload": "", "refreshes": []string{fn.ID() + "/"}})
				r := httptest.NewRequest(http.MethodPost, rs.Prefix()+command.ID(), strings.NewReader(string(body)))
				rec := httptest.NewRecorder()
				rs.ServeHTTP(rec, r)
				kind, data, _ := envelope(t, rec.Body.Bytes())
				if kind != "result" || field(t, data, "_") != "literal-command-completed-354" || calls != 1 {
					t.Fatalf("refresh failure destroyed command result or skipped hook: %s calls=%d", rec.Body.String(), calls)
				}
				key := "q"
				if live {
					key = "l"
				}
				public := field(t, field(t, field(t, data, key), fn.ID()+"/"), "e")
				if field(t, public, "status") != float64(500) || field(t, public, "message") != "literal-refresh-public-354" || field(t, public, "supportId") != "literal-refresh-hook-354" || strings.Contains(rec.Body.String(), "literal-private-refresh-354") {
					t.Fatalf("refresh error lost safe public shape: %s", rec.Body.String())
				}
			})
		}
	}
}
