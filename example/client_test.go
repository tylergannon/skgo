package example_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tylergannon/polytype"
	"github.com/tylergannon/skgo"
	generated "github.com/tylergannon/skgo/example/internal/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/client"
	optional "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5xxa5djn5xgc3a"
)

// Captured from the pinned Kit 3.0.0-next.27 serialize_binary_form function
// with devalue 5.9.2, not from skgo's encoder.
const kitOptionalOmitted = "ADYAAAAAAFtbMSwzXSx7Im5hbWUiOjJ9LCJmaXh0dXJlIix7InJlbW90ZV9yZWZyZXNoZXMiOjR9LFtdXQ=="
const kitOptionalExplicit = "AGEAAAAAAFtbMSw2XSx7Im5hbWUiOjIsImNvdW50IjozLCJlbmFibGVkIjo0LCJsYWJlbCI6NX0sImZpeHR1cmUiLDAsZmFsc2UsIiIseyJyZW1vdGVfcmVmcmVzaGVzIjo3fSxbXV0="

func formHandler(t *testing.T) http.Handler {
	t.Helper()
	remotes, err := skgo.NewRemotes(skgo.RemoteConfig{}, generated.Remotes()...)
	if err != nil {
		t.Fatal(err)
	}
	return remotes
}

func checkOptionalCalls(t *testing.T, c client.Client, firstOperation int) {
	t.Helper()
	omitted, err := c.Submit(context.Background(), optional.Input{Name: "fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if omitted != (optional.Result{Name: "fixture", Count: "absent", Enabled: "absent", Label: "absent", Operations: firstOperation}) {
		t.Fatalf("omitted: %+v", omitted)
	}
	explicit, err := c.Submit(context.Background(), optional.Input{
		Name:    "fixture",
		Count:   polytype.Optional[int]{Present: true, Value: 0},
		Enabled: polytype.Optional[bool]{Present: true, Value: false},
		Label:   polytype.Optional[string]{Present: true, Value: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if explicit != (optional.Result{Name: "fixture", Count: "present: 0", Enabled: "present: false", Label: `present: ""`, Operations: firstOperation + 1}) {
		t.Fatalf("explicit zero/false/empty: %+v", explicit)
	}
	_, err = c.Submit(context.Background(), optional.Input{})
	var invalid *skgo.Invalid
	if !errors.As(err, &invalid) || len(invalid.Issues) != 1 || invalid.Issues[0] != (skgo.Issue{Field: "name", Message: "A name is required"}) {
		t.Fatalf("field error: %v", err)
	}
}

func TestGeneratedFormClientOverHTTPAndUDS(t *testing.T) {
	handler := formHandler(t)
	httpServer := httptest.NewServer(handler)
	defer httpServer.Close()
	t.Run("HTTP", func(t *testing.T) {
		checkOptionalCalls(t, client.Client{FormClient: skgo.FormClient{BaseURL: httpServer.URL}}, 1)
	})

	socketDir, err := os.MkdirTemp("/tmp", "skgo-client-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDir) })
	socket := filepath.Join(socketDir, "skgo.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	server := &http.Server{Handler: handler}
	go server.Serve(listener)
	defer server.Close()
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	t.Run("UDS", func(t *testing.T) {
		checkOptionalCalls(t, client.Client{FormClient: skgo.FormClient{BaseURL: "http://skgo", HTTPClient: &http.Client{Transport: transport}}}, 3)
	})
}

func TestGeneratedFormClientWithOriginCheck(t *testing.T) {
	server := httptest.NewUnstartedServer(nil)
	origin := "http://" + server.Listener.Addr().String()
	remotes, err := skgo.NewRemotes(skgo.RemoteConfig{Origin: origin}, generated.Remotes()...)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = remotes
	server.Start()
	defer server.Close()
	c := client.Client{FormClient: skgo.FormClient{BaseURL: server.URL}}
	result, err := c.Submit(context.Background(), optional.Input{Name: "origin"})
	if err != nil || result.Name != "origin" {
		t.Fatalf("origin-protected Form: %+v, %v", result, err)
	}
}

func TestGeneratedFormClientMatchesKitOptionalRequestBytes(t *testing.T) {
	var bodies [][]byte
	handler := formHandler(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		bodies = append(bodies, body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	c := client.Client{FormClient: skgo.FormClient{BaseURL: server.URL}}
	inputs := []optional.Input{
		{Name: "fixture"},
		{Name: "fixture", Count: polytype.Optional[int]{Present: true}, Enabled: polytype.Optional[bool]{Present: true}, Label: polytype.Optional[string]{Present: true}},
	}
	for _, input := range inputs {
		if _, err := c.Submit(context.Background(), input); err != nil {
			t.Fatal(err)
		}
	}
	for i, golden := range []string{kitOptionalOmitted, kitOptionalExplicit} {
		want, err := base64.StdEncoding.DecodeString(golden)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bodies[i], want) {
			t.Fatalf("request %d differs from Kit: got %x, want %x", i, bodies[i], want)
		}
	}
}

func TestGeneratedFormClientErrorsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/submit") {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"type":"error","error":{"status":409,"message":"already started"}}`)
		}
	}))
	defer server.Close()
	c := client.Client{FormClient: skgo.FormClient{BaseURL: server.URL}}
	_, err := c.Submit(context.Background(), optional.Input{Name: "fixture"})
	var status *skgo.HTTPError
	if !errors.As(err, &status) || status.Status != 409 || status.Message != "already started" {
		t.Fatalf("HTTP-200 error envelope: %v", err)
	}

	malformed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { fmt.Fprint(w, `{"type":"result","data":"[{}]"}`) }))
	defer malformed.Close()
	c.BaseURL = malformed.URL
	_, err = c.Submit(context.Background(), optional.Input{Name: "fixture"})
	if err == nil {
		t.Fatal("malformed result became zero-valued success")
	}

	entered := make(chan struct{})
	release := make(chan struct{})
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer func() { close(release); slow.Close() }()
	c.BaseURL = slow.URL
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan error, 1)
	go func() { _, err := c.Submit(ctx, optional.Input{Name: "fixture"}); finished <- err }()
	<-entered
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("network wait ignored cancellation")
	}
}

type loseReply struct {
	base     http.RoundTripper
	requests atomic.Int32
}

func (l *loseReply) RoundTrip(r *http.Request) (*http.Response, error) {
	l.requests.Add(1)
	res, err := l.base.RoundTrip(r)
	if err != nil {
		return nil, err
	}
	res.Body.Close()
	return nil, fmt.Errorf("reply lost after handler ran")
}

func TestGeneratedFormClientDoesNotReplayLostReply(t *testing.T) {
	var handled atomic.Int32
	h := formHandler(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
		handled.Add(1)
	}))
	defer server.Close()
	lost := &loseReply{base: http.DefaultTransport}
	c := client.Client{FormClient: skgo.FormClient{BaseURL: server.URL, HTTPClient: &http.Client{Transport: lost}}}
	_, err := c.Submit(context.Background(), optional.Input{Name: "fixture"})
	if err == nil || !strings.Contains(err.Error(), "reply lost") {
		t.Fatalf("lost reply: %v", err)
	}
	if got := handled.Load(); got != 1 {
		t.Fatalf("handler completed %d times, want one", got)
	}
	if got := lost.requests.Load(); got != 1 {
		t.Fatalf("client sent %d requests, want one", got)
	}
}

func TestGeneratedFormClientDoesNotFollowMutationRedirect(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var first, replayed atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/again" {
					replayed.Add(1)
					return
				}
				first.Add(1)
				w.Header().Set("Location", "/again")
				w.WriteHeader(status)
			}))
			defer server.Close()
			shared := &http.Client{}
			c := client.Client{FormClient: skgo.FormClient{BaseURL: server.URL, HTTPClient: shared}}
			_, err := c.Submit(context.Background(), optional.Input{Name: "fixture"})
			if err == nil || !strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status)) {
				t.Fatalf("redirect: %v", err)
			}
			if first.Load() != 1 || replayed.Load() != 0 {
				t.Fatalf("first=%d replayed=%d", first.Load(), replayed.Load())
			}
			if shared.CheckRedirect != nil {
				t.Fatal("mutated caller's HTTP client")
			}
		})
	}
}
