package example_test

import (
	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/ui"
	"io/fs"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSharedRendererDefaultFilter(t *testing.T) {
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := example.NewHandler(dist, nil)
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:8080/options-served", nil))
	body := response.Body.String()
	if response.Code != 200 || !strings.Contains(body, "<h1>Served default options</h1>") || !strings.Contains(body, `"x-default":"default-literal"`) || strings.Contains(body, `"x-public"`) || strings.Contains(body, `"x-denied"`) {
		t.Fatalf("served default parity: %d %s", response.Code, body)
	}
}

func TestSharedApplicationConfiguration(t *testing.T) {
	dist, err := fs.Sub(ui.Build, "build")
	if err != nil {
		t.Fatal(err)
	}
	handler, err := example.NewHandler(dist, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/shared-fetch", "/shared-fetch-served"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil))
		body := response.Body.String()
		if response.Code != 200 {
			t.Fatalf("%s: %d %s", path, response.Code, body)
		}
		for _, want := range []string{"<h1>Shared configuration</h1>", "Go:literal-hook-content-278|literal-api-278|literal-hook-header-278", "Kit:literal-hook-content-278|literal-api-278|literal-hook-header-278|default-literal", "Boundary:" + path, `"x-config-hook":"literal-hook-header-278"`, `"x-default":"default-literal"`} {
			if !strings.Contains(body, want) {
				t.Errorf("%s lacks %q: %s", path, want, body)
			}
		}
		if strings.Contains(body, "literal-private-header-275") {
			t.Errorf("%s leaked private header: %s", path, body)
		}
	}
	for _, path := range []string{"/shared-error", "/shared-error-served", "/shared-component", "/shared-component-served"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("GET", "http://127.0.0.1:8080"+path, nil))
		body := response.Body.String()
		want := "500|literal-safe-error-278|literal-support-278|11"
		if strings.Contains(path, "component") {
			want = "200|literal-safe-error-278|literal-support-278|11"
		}
		if response.Code != 200 || !strings.Contains(body, want) || strings.Contains(body, "literal-private-error-278") {
			t.Fatalf("%s public error body: %d %s", path, response.Code, body)
		}
	}
}
