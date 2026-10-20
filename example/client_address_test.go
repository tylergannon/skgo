package example_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tylergannon/skgo/example"
)

func TestFlyClientAddressReadsTheProxyHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if _, err := example.FlyClientAddress(req); err == nil {
		t.Fatal("a request without the header was given an address")
	}
	req.Header.Set("Fly-Client-IP", "203.0.113.7")
	if got, err := example.FlyClientAddress(req); err != nil || got != "203.0.113.7" {
		t.Fatalf("got %q, %v; want 203.0.113.7", got, err)
	}
}
