package nativehost

import (
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"testing"
)

func TestRecoveryRetainsApplicationAndOrigin(t *testing.T) {
	var created, requests atomic.Int32
	h := New(func(origin string) (http.Handler, error) {
		created.Add(1)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, requests.Add(1)) }), nil
	})
	origin, err := h.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer h.Stop()
	get := func(path, want string, status int) {
		t.Helper()
		resp, err := http.Get(origin + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != status || string(b) != want {
			t.Fatalf("%d %q", resp.StatusCode, b)
		}
	}
	get("/__skgo_health", "", 204)
	get("/", "1", 200)
	h.mu.Lock()
	h.listener.Close()
	h.mu.Unlock()
	if err := h.Recover(); err != nil {
		t.Fatal(err)
	}
	if got, err := h.Start(); got != origin || err != nil {
		t.Fatalf("origin %s %v", got, err)
	}
	get("/", "2", 200)
	if created.Load() != 1 {
		t.Fatalf("recreated application %d times", created.Load())
	}
	if err := h.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := h.Recover(); err == nil {
		t.Fatal("recovered stopped host")
	}
}
