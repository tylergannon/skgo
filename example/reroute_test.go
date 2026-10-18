//go:build skgo_qualification

package example_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGoRerouteSelectsApplicationRoutes(t *testing.T) {
	origin := os.Getenv("SKGO_ROUTE_ORIGIN")
	var handler http.Handler
	if origin == "" {
		handler = newProdHandler(t)
	}
	request := func(path, accept string) (int, string) {
		t.Helper()
		if handler != nil {
			r := httptest.NewRequest("GET", path, nil)
			r.Header.Set("Accept", accept)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, r)
			return rec.Code, rec.Body.String()
		}
		r, err := http.NewRequest("GET", origin+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Accept", accept)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, string(body)
	}
	for _, test := range []struct{ path, receipt string }{
		{"/go-alias/page?receipt=handler", "page|route=/rerouted/[number=Order]|param=Order #73|url=/go-alias/page?receipt=handler"},
		{"/go-alias/page/__data.json?receipt=handler&x-sveltekit-invalidated=11", "page|route=/rerouted/[number=Order]|param=Order #73|url=/go-alias/page?receipt=handler"},
		{"/go-alias/endpoint?receipt=handler", "endpoint|route=/api/rerouted/[number=Order]|param=91|url=/go-alias/endpoint?receipt=handler"},
		{"/rerouted/42?receipt=direct", "page|route=/rerouted/[number=Order]|param=Order #42|url=/rerouted/42?receipt=direct"},
		{"/api/rerouted/12?receipt=direct", "endpoint|route=/api/rerouted/[number=Order]|param=12|url=/api/rerouted/12?receipt=direct"},
	} {
		t.Run(test.path, func(t *testing.T) {
			status, body := request(test.path, "text/html")
			if status != 200 || !strings.Contains(body, test.receipt) {
				t.Fatalf("status=%d want %q in %s", status, test.receipt, body)
			}
		})
	}
	status, body := request("/unmapped-reroute-control", "text/html")
	if status != 404 || !strings.Contains(body, "Not Found") {
		t.Fatalf("unmapped control status=%d body=%s", status, body)
	}
}
