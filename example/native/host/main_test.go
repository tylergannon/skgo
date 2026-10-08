package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// Real pooled renderer, assets and Go endpoints; this is host HTTP behavior,
// not simulator/device or WebView lifecycle proof.
func TestLocalHostStartStopAndRestart(t *testing.T) {
	defer SKGoHostStop()
	client := &http.Client{Timeout: 5 * time.Second}
	for range 2 {
		origin := start()
		if !strings.HasPrefix(origin, "http://127.0.0.1:") {
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
