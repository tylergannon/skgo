package example_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

// The same supplied routing fixtures run against the embedded production
// handler, or an explicitly selected real development session. HTTP outcomes
// stay Go assertions; the browser scenarios cover Kit's client navigation.
func TestRoutingOptionAndMatcherContracts(t *testing.T) {
	origin := os.Getenv("SKGO_ROUTE_ORIGIN")
	var handler http.Handler
	if origin == "" {
		handler = newProdHandler(t)
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	request := func(path, accept string) (int, http.Header, string) {
		t.Helper()
		if handler != nil {
			req := httptest.NewRequest("GET", path, nil)
			req.Header.Set("Accept", accept)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			return rec.Code, rec.Header(), rec.Body.String()
		}
		req, err := http.NewRequest("GET", origin+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Accept", accept)
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		return res.StatusCode, res.Header, string(body)
	}
	for _, test := range []struct{ path, receipt string }{
		{"/routing/optional", "optional|omitted"}, {"/routing/optional/42", "optional|Order #42"},
		{"/routing/fallback/7", "matched|Order #7"}, {"/routing/fallback/no-order", "fallback|no-order"},
		{"/routing/reject/42", "accepted-only|Order #42"},
		{"/routing/slash/always/", "always|/routing/slash/always/"}, {"/routing/slash/never", "never|/routing/slash/never"},
		{"/routing/slash/ignore", "ignore|/routing/slash/ignore"}, {"/routing/slash/ignore/", "ignore|/routing/slash/ignore/"},
		{"/routing/slash/mixed/", "mixed-page|/routing/slash/mixed/"},
	} {
		t.Run(test.path, func(t *testing.T) {
			status, _, body := request(test.path, "text/html")
			if status != 200 || !strings.Contains(body, `data-testid="routing-receipt"`) || !strings.Contains(body, test.receipt) {
				t.Fatalf("status=%d want receipt %q in %s", status, test.receipt, body)
			}
			status, _, body = request(strings.TrimSuffix(test.path, "/")+"/__data.json?x-sveltekit-invalidated=111111&x-sveltekit-trailing-slash="+func() string {
				if strings.HasSuffix(test.path, "/") {
					return "1"
				}
				return "0"
			}(), "application/json")
			if status != 200 || !strings.Contains(body, test.receipt) {
				t.Fatalf("data status=%d want receipt %q in %s", status, test.receipt, body)
			}
		})
	}
	for _, test := range []struct{ path, accept, location string }{
		{"/routing/slash/always?q=a%2Bb", "text/html", "./always/?q=a%2Bb"},
		{"/routing/slash/never/?q=a%2Bb", "text/html", "../never?q=a%2Bb"},
		{"/routing/slash/mixed?q=a%2Bb", "text/html", "./mixed/?q=a%2Bb"},
		{"/routing/slash/mixed?q=a%2Bb", "text/plain", "./mixed/?q=a%2Bb"},
		{"/routing/slash/api/?q=a%2Bb", "text/plain", "../api?q=a%2Bb"},
	} {
		status, header, body := request(test.path, test.accept)
		if status != 308 || header.Get("Location") != test.location || header.Get("X-Sveltekit-Normalize") != "1" {
			t.Fatalf("%s %s: status=%d headers=%v body=%s", test.accept, test.path, status, header, body)
		}
	}
	for _, test := range []struct{ path, receipt string }{
		{"/routing/slash/api", "endpoint-default|/routing/slash/api"},
		{"/routing/slash/mixed/", "mixed-endpoint|/routing/slash/mixed/"},
	} {
		status, _, body := request(test.path, "text/plain")
		if status != 200 || body != test.receipt {
			t.Fatalf("endpoint %s status=%d body=%s", test.path, status, body)
		}
	}
	for _, path := range []string{"/routing/optional/rejected", "/routing/reject/no-order"} {
		status, _, body := request(path, "text/html")
		if status != 404 || !strings.Contains(body, "Not Found") {
			t.Fatalf("matcher rejection %s status=%d body=%s", path, status, body)
		}
	}
}
