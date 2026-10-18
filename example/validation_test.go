//go:build skgo_qualification

package example_test

import (
	"encoding/json"
	"github.com/tylergannon/devalue/v5"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestApplicationRemoteArgumentValidation(t *testing.T) {
	origin := os.Getenv("SKGO_VALIDATION_ORIGIN")
	var handler http.Handler
	if origin == "" {
		handler = newProdHandler(t)
	}
	request := func(name, document string) string {
		t.Helper()
		path := "/_app/remote/" + remoteID(t, name)
		if document != "" {
			path += "?payload=" + payload(document)
		}
		var status int
		var body []byte
		if handler != nil {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			status, body = rec.Code, rec.Body.Bytes()
		} else {
			response, err := http.Get(origin + path)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			status = response.StatusCode
			body, err = io.ReadAll(response.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		if status != 200 {
			t.Fatalf("%s status=%d body=%s", name, status, body)
		}
		return string(body)
	}
	body := request("validateCatalog", `[{"name":1},42]`)
	var rejected struct {
		Type  string         `json:"type"`
		Error map[string]any `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Type != "error" || len(rejected.Error) != 3 || rejected.Error["status"] != float64(400) || rejected.Error["message"] != "Choose a catalog name." || rejected.Error["reference"] != "catalog-validation-166" {
		t.Fatalf("invalid answer=%s", body)
	}
	body = request("validationBodyCalls", "")
	var count struct{ Type, Data string }
	if err := json.Unmarshal([]byte(body), &count); err != nil {
		t.Fatal(err)
	}
	tree, err := devalue.Parse(count.Data, nil)
	if err != nil {
		t.Fatal(err)
	}
	obj, ok := tree.(*devalue.Object)
	if !ok {
		t.Fatalf("count tree=%v", tree)
	}
	value, present := obj.Get("_")
	if count.Type != "result" || !present || value != float64(0) {
		t.Fatalf("invalid invoked Go body: %s", body)
	}
	if body = request("validateCatalog", `[{"name":1},"atlas"]`); !strings.Contains(body, "Go accepted atlas|body-calls=1") {
		t.Fatalf("valid receipt=%s", body)
	}
}
