package devserver

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPublicHandoffWaitsBeforeDispatchAndFinishesAdmittedFetches(t *testing.T) {
	var mu sync.Mutex
	var effects []string
	old, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "literal-old-nested-fetch-347")
	}))
	oldTarget, err := socketTarget(old)
	if err != nil {
		t.Fatal(err)
	}
	defer oldTarget.Transport.CloseIdleConnections()
	started, finish := make(chan struct{}), make(chan struct{})
	front, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/finite" {
			close(started)
			<-finish
			// Like a Go -> Kit -> Go private continuation, this descendant of an
			// admitted request does not re-enter the public admission barrier.
			request, _ := http.NewRequest("GET", oldTarget.URL.String(), nil)
			response, err := oldTarget.Transport.RoundTrip(request)
			if err != nil {
				http.Error(w, err.Error(), 500)
				return
			}
			defer response.Body.Close()
			io.Copy(w, response.Body)
			return
		}
		fmt.Fprint(w, "literal-frontend-ready-347")
	}))
	next, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		effects = append(effects, string(body))
		mu.Unlock()
		fmt.Fprint(w, "literal-command-committed-347")
	}))
	route := &router{}
	if err := route.setApp(old); err != nil {
		t.Fatal(err)
	}
	if err := route.setFrontend(front, nil); err != nil {
		t.Fatal(err)
	}
	public := httptest.NewServer(route)
	defer public.Close()
	defer route.requests.close()
	type result struct {
		status int
		body   string
		err    error
	}
	request := func(method, path, body string, ctx context.Context) <-chan result {
		done := make(chan result, 1)
		go func() {
			req, _ := http.NewRequestWithContext(ctx, method, public.URL+path, strings.NewReader(body))
			response, err := http.DefaultClient.Do(req)
			if err != nil {
				done <- result{err: err}
				return
			}
			defer response.Body.Close()
			raw, err := io.ReadAll(response.Body)
			done <- result{response.StatusCode, string(raw), err}
		}()
		return done
	}
	finite := request("GET", "/finite", "", context.Background())
	<-started
	drained := make(chan error, 1)
	go func() { drained <- route.requests.pause(context.Background(), time.Second) }()
	waitForPause(t, &route.requests)
	command := request("POST", "/_app/remote/save", "literal-once-347", context.Background())
	document := request("GET", "/document", "", context.Background())
	data := request("GET", "/document/__data.json", "", context.Background())
	canceled, cancel := context.WithCancel(context.Background())
	abandoned := request("POST", "/_app/remote/save", "must-never-execute-347", canceled)
	cancel()
	if actual := <-abandoned; actual.err == nil {
		t.Fatalf("canceled request succeeded: %+v", actual)
	}
	// An HMR request still reaches Vite while admission is closed.
	client := &http.Client{Timeout: time.Second}
	for _, path := range []string{"/__skgo/dev/hmr", "/base/__skgo/dev/hmr"} {
		hmr, _ := http.NewRequest("GET", public.URL+path, nil)
		hmr.Header.Set("Upgrade", "websocket")
		hmr.Header.Set("Sec-WebSocket-Protocol", "vite-hmr")
		response, err := client.Do(hmr)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("HMR status = %d", response.StatusCode)
		}
	}
	for _, waiting := range []<-chan result{command, document, data} {
		select {
		case got := <-waiting:
			t.Fatalf("dispatched before replacement readiness: %+v", got)
		default:
		}
	}
	close(finish)
	if actual := <-finite; actual.err != nil || actual.status != 200 || actual.body != "literal-old-nested-fetch-347" {
		t.Fatalf("admitted request = %+v", actual)
	}
	if err := <-drained; err != nil {
		t.Fatal(err)
	}
	if err := route.setApp(next); err != nil {
		t.Fatal(err)
	}
	route.requests.resume()
	for index, waiting := range []<-chan result{command, document, data} {
		actual := <-waiting
		want := "literal-frontend-ready-347"
		if index == 0 {
			want = "literal-command-committed-347"
		}
		if actual.err != nil || actual.status != 200 || actual.body != want {
			t.Fatalf("resumed request = %+v, want %s", actual, want)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(effects) != 1 || effects[0] != "literal-once-347" {
		t.Fatalf("committed effects = %q", effects)
	}
}

func TestPublicHandoffBoundsOpenResponseStream(t *testing.T) {
	ended := make(chan struct{})
	socket, _ := privateHTTP(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "literal-open-stream-347")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(ended)
	}))
	route := &router{}
	if err := route.setApp(socket); err != nil {
		t.Fatal(err)
	}
	public := httptest.NewServer(route)
	defer public.Close()
	defer route.requests.close()
	response, err := http.Get(public.URL + "/_app/remote/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(response.Body); done <- err }()
	if err := route.requests.pause(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("open private stream was not canceled at drain deadline")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("incomplete stream reported successful completion")
		}
	case <-time.After(time.Second):
		t.Fatal("open public stream survived bounded retirement")
	}
}
