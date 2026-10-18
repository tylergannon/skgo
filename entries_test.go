package skgo

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBuildRouteEntriesBeforeRequestOwnership(t *testing.T) {
	type params struct {
		Slug    string  `json:"slug"`
		Version *string `json:"version,omitempty"`
	}
	for _, tc := range []struct {
		name   string
		fn     func() ([]params, error)
		status int
		body   string
	}{
		{"catalog", func() ([]params, error) { return []params{{Slug: "atlas"}, {Slug: "beacon"}}, nil }, 200, `{"entries":[{"slug":"atlas"},{"slug":"beacon"}]}`},
		{"empty", func() ([]params, error) { return nil, nil }, 200, `{"entries":[]}`},
		{"failure", func() ([]params, error) { return nil, fmt.Errorf("catalog unavailable") }, 500, "entries for src/routes/catalog/[slug]/+page.server.ts: catalog unavailable"},
		{"panic", func() ([]params, error) { panic("catalog panic") }, 500, "entries for src/routes/catalog/[slug]/+page.server.ts panicked: catalog panic"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			registrations := []*RouteEntries{NewRouteEntries("src/routes/catalog/[slug]/+page.server.ts", tc.fn)}
			handler := prerenderHandler("secret", nil, nil, nil, nil, &prerenderRequests{options: PrerenderServiceOptions{Entries: registrations}})
			request := httptest.NewRequest(http.MethodPost, "/entries", strings.NewReader(`{"module":"src/routes/catalog/[slug]/+page.server.ts"}`))
			request.Header.Set("Authorization", "Bearer secret")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.body) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}
