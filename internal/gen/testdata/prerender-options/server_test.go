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
