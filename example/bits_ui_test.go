package example_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestBitsUIRendersThroughTheProductionHandler(t *testing.T) {
	response := get(t, newProdHandler(t), "/bits-ui")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body)
	}
	for _, want := range []string{"Bits UI server rendering", "Open library dialog", "Library row 1</p>", "Library row 20</p>", `data-scroll-area-viewport`, `data-dialog-trigger`, `>closed</p>`} {
		if !strings.Contains(response.Body.String(), want) {
			t.Errorf("SSR document does not contain %q: %s", want, response.Body.String())
		}
	}
}
