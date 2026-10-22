package remoteclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tylergannon/devalue/v5"
)

func TestTransportErrorsPreserveCauseWithoutCommandWording(t *testing.T) {
	offline := errors.New("offline")
	c := Client{BaseURL: "https://app.test", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { return nil, offline })}}
	for _, kind := range []string{"query", "command"} {
		_, err := c.Call(context.Background(), "_app/remote/", "hash/read", kind, devalue.Undefined)
		if !errors.Is(err, offline) || strings.Contains(err.Error(), "command") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

func TestRemoteRequestsAndEnvelopes(t *testing.T) {
	requests := 0
	client := Client{BaseURL: "https://app.test/shop", Headers: http.Header{"Authorization": {"Bearer test-token"}, "X-Api-Key": {"test-key"}}, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		requests++
		if r.Header.Get("Authorization") != "Bearer test-token" || r.Header.Get("X-API-Key") != "test-key" {
			t.Fatal("application authentication headers were not forwarded")
		}
		if r.URL.Path != "/shop/kit/remote/hash/read" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Method == "GET" {
			if r.URL.Query().Get("payload") != "" {
				t.Fatalf("no-input query=%s", r.URL)
			}
		} else {
			if r.Header.Get("Origin") != "https://app.test" {
				t.Fatalf("origin=%s", r.Header.Get("Origin"))
			}
			var body struct {
				Payload   string
				Refreshes []string
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Payload == "" || body.Refreshes == nil || len(body.Refreshes) != 0 {
				t.Fatalf("body=%+v", body)
			}
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"result","data":"[{\"_\":1},\"observed\"]"}`))}, nil
	})}}
	for _, kind := range []string{"query", "command"} {
		input := any(devalue.Undefined)
		if kind == "command" {
			input = devalue.NewObject("title", "Ship MCP")
		}
		value, err := client.Call(context.Background(), "kit/remote/", "hash/read", kind, input)
		if err != nil || value != "observed" {
			t.Fatalf("%s=%v %v", kind, value, err)
		}
	}
	if requests != 2 {
		t.Fatalf("requests=%d", requests)
	}
}

func TestRemoteFailures(t *testing.T) {
	for _, body := range []string{`{"type":"error","error":{"status":403,"message":"Forbidden"}}`, `{"type":"result","data":"[42]"}`, `{"type":"other"}`, `<html>Login</html>`} {
		calls := 0
		c := Client{BaseURL: "https://app.test", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		if _, err := c.Call(context.Background(), "_app/remote/", "hash/write", "command", "title"); err == nil {
			t.Fatalf("accepted %s", body)
		}
		if calls != 1 {
			t.Fatalf("command sent %d times", calls)
		}
	}
}

func TestRequestCallbackRunsForEachCallAndCanStopDispatch(t *testing.T) {
	prepared, sent := 0, 0
	rejected := errors.New("credentials unavailable")
	c := Client{BaseURL: "https://app.test", PrepareRequest: func(r *http.Request) error {
		prepared++
		if prepared == 3 {
			return rejected
		}
		r.Header.Set("X-API-Key", "fresh-key")
		if r.Header.Get("Accept") != "application/json" {
			t.Fatal("callback ran before standard headers")
		}
		return nil
	}, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		sent++
		if r.Header.Get("X-API-Key") != "fresh-key" {
			t.Fatal("missing prepared header")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"result","data":"[{\"_\":1},\"ok\"]"}`))}, nil
	})}}
	for _, kind := range []string{"query", "command"} {
		if _, err := c.Call(context.Background(), "_app/remote/", "hash/call", kind, devalue.Undefined); err != nil {
			t.Fatal(err)
		}
	}
	_, err := c.Call(context.Background(), "_app/remote/", "hash/call", "command", devalue.Undefined)
	if !errors.Is(err, rejected) || !errors.Is(err, ErrNotSent) || prepared != 3 || sent != 2 {
		t.Fatalf("prepared=%d sent=%d err=%v", prepared, sent, err)
	}
}

func TestBaseURLIncludesDeploymentPath(t *testing.T) {
	for _, tt := range []struct{ base, want string }{
		{"https://app.test", "https://app.test/configured/assets/remote/hash/write"},
		{"https://app.test/shop/", "https://app.test/shop/configured/assets/remote/hash/write"},
		{"https://app.test/shop%2Fbranch", "https://app.test/shop%2Fbranch/configured/assets/remote/hash/write"},
		{"https://app.test/shop%2F", "https://app.test/shop%2F/configured/assets/remote/hash/write"},
	} {
		t.Run(tt.base, func(t *testing.T) {
			c := Client{BaseURL: tt.base, HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				if r.URL.String() != tt.want || r.Header.Get("Origin") != "https://app.test" {
					t.Fatalf("URL=%s origin=%s", r.URL, r.Header.Get("Origin"))
				}
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"type":"result","data":"[{\"_\":1},\"ok\"]"}`))}, nil
			})}}
			if got, err := c.Call(t.Context(), "configured/assets/remote/", "hash/write", "command", "title"); err != nil || got != "ok" {
				t.Fatalf("result=%v err=%v", got, err)
			}
		})
	}
}
