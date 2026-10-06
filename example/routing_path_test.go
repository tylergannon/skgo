package example_test

import (
	"encoding/json"
	"github.com/tylergannon/polytype/devalue"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func pathMarkupContains(body, literal string) bool {
	pattern := strings.Replace(regexp.QuoteMeta(literal), `">`, `"[^>]*>`, 1)
	return regexp.MustCompile(pattern).MatchString(body)
}

func TestSharedPathDecoding(t *testing.T) {
	h := newProdHandler(t)
	for _, tc := range []struct{ segment, value string }{
		{"%2525", "%25"}, {"a%2Fb", "a/b"}, {"%2F", "/"}, {"%E2%9C%93", "✓"},
	} {
		t.Run(tc.segment, func(t *testing.T) {
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/items/"+tc.segment, nil))
			if rec.Code != 200 {
				t.Fatalf("item document: %d %s", rec.Code, rec.Body.String())
			}
			for _, want := range []string{
				`data-testid="title">Item ` + tc.value + `</h1>`,
				`data-testid="load-item-name">Widget ` + tc.value + `</p>`,
				`data-testid="item-name">Widget ` + tc.value + `</p>`,
			} {
				if !pathMarkupContains(rec.Body.String(), want) {
					t.Errorf("item document lacks literal %q", want)
				}
			}
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/middleware/"+tc.segment, nil))
			if rec.Code != 200 || !pathMarkupContains(rec.Body.String(), `data-testid="mw-slug-param">`+tc.value+`</p>`) {
				t.Fatalf("middleware document: want literal %q: %d %s", tc.value, rec.Code, rec.Body.String())
			}
			rec = httptest.NewRecorder()
			h.ServeHTTP(rec, httptest.NewRequest("GET", "/middleware/"+tc.segment+"/__data.json", nil))
			var wire struct {
				Nodes []struct{ Data json.RawMessage }
			}
			if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &wire) != nil || len(wire.Nodes) != 2 {
				t.Fatalf("data: %d %s", rec.Code, rec.Body.String())
			}
			tree, err := devalue.Parse(string(wire.Nodes[1].Data), nil)
			if err != nil {
				t.Fatal(err)
			}
			data, ok := tree.(*devalue.Object)
			if !ok {
				t.Fatalf("load data %T", tree)
			}
			slug, _ := data.Get("slug")
			route, _ := data.Get("route")
			isData, _ := data.Get("data")
			if slug != tc.value || route != "/middleware/[slug]" || isData != true {
				t.Fatalf("data: slug=%v route=%v isData=%v, want %q /middleware/[slug] true", slug, route, isData, tc.value)
			}
		})
	}
	for _, path := range []string{"/items/%FF", "/middleware/%C0%AF/__data.json", "/middleware/%ED%A0%80"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("malformed %s: %d %s", path, rec.Code, rec.Body.String())
		}
	}
}
