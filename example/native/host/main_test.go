package main

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"github.com/tylergannon/devalue/v5"
)

// Real pooled renderer, assets and Go endpoints; this is host HTTP behavior,
// not simulator/device or WebView lifecycle proof.
func TestLocalHostStartStopAndRestart(t *testing.T) {
	defer SKGoHostStop()
	client := &http.Client{Timeout: 5 * time.Second}
	for range 2 {
		origin := start()
		if !strings.HasPrefix(origin, "http://localhost:") {
			t.Fatalf("origin = %q", origin)
		}
		if got := start(); got != origin {
			t.Fatalf("repeated start = %q; want same active host %q", got, origin)
		}
		response, err := client.Get(origin + "/todos")
		if err != nil {
			t.Fatal(err)
		}
		bytes, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 || !strings.Contains(string(bytes), `data-testid="title">Todos</h1>`) {
			t.Fatalf("page status=%d body=%s", response.StatusCode, bytes)
		}
		SKGoHostStop()
		if response, err := client.Get(origin + "/todos"); err == nil {
			response.Body.Close()
			t.Fatal("stopped host still responds")
		}
	}
}

func TestLocalHostSessionCookie(t *testing.T) {
	defer SKGoHostStop()
	origin := start()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	payload := base64.RawURLEncoding.EncodeToString([]byte(`["Host probe"]`))
	request, err := http.NewRequest(http.MethodPost, origin+"/_app/remote/4cga8b/signIn",
		strings.NewReader(`{"payload":"`+payload+`","refreshes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Origin", origin)
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("sign-in status = %d", response.StatusCode)
	}
	var session *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "skgo_session" {
			session = cookie
		}
	}
	if session == nil || session.Secure || !session.HttpOnly {
		t.Fatalf("local session cookie = %#v; want a non-Secure HttpOnly cookie", session)
	}
	response, err = client.Get(origin + "/_app/remote/4cga8b/whoami")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var envelope struct{ Type, Data string }
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || envelope.Type != "result" {
		t.Fatalf("whoami status=%d envelope=%+v", response.StatusCode, envelope)
	}
	value, err := devalue.Parse(envelope.Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	field := func(value any, name string) any {
		t.Helper()
		object, ok := value.(*devalue.Object)
		if !ok {
			t.Fatalf("field %q: expected an object, got %T", name, value)
		}
		value, ok = object.Get(name)
		if !ok {
			t.Fatalf("missing field %q", name)
		}
		return value
	}
	queries := field(value, "q")
	node := field(queries, "4cga8b/whoami/")
	signedIn := field(node, "v")
	if user := field(signedIn, "user"); user != "Host probe" {
		t.Fatalf("whoami user = %v; want Host probe", user)
	}
}
