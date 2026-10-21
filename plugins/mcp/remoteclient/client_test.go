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
	c := Client{Origin: "https://app.test", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) { return nil, offline })}}
	for _, kind := range []string{"query", "command"} {
		_, err := c.Call(context.Background(), "hash/read", kind, devalue.Undefined)
		if !errors.Is(err, offline) || strings.Contains(err.Error(), "command") {
			t.Fatalf("%s: %v", kind, err)
		}
	}
}

type transport func(*http.Request) (*http.Response, error)

func (t transport) RoundTrip(r *http.Request) (*http.Response, error) { return t(r) }

func TestRemoteRequestsAndEnvelopes(t *testing.T) {
	requests := 0
	client := Client{Origin: "https://app.test", Base: "/shop", AppDir: "kit", HTTP: &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
		requests++
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
		value, err := client.Call(context.Background(), "hash/read", kind, input)
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
		c := Client{Origin: "https://app.test", HTTP: &http.Client{Transport: transport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body))}, nil
		})}}
		if _, err := c.Call(context.Background(), "hash/write", "command", "title"); err == nil {
			t.Fatalf("accepted %s", body)
		}
		if calls != 1 {
			t.Fatalf("command sent %d times", calls)
		}
	}
}
