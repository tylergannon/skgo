package migration

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/tylergannon/skgo"
)

func TestMigrationHandler(t *testing.T) {
	ext := os.Getenv("MIGRATION_EXT")
	// Literal Kit 3.1.0 hashes of src/lib/data/data.remote.ts and .js.
	hash := "1332tjg"
	if ext == "js" {
		hash = "3v3jv6"
	} else if ext != "ts" {
		t.Fatal("MIGRATION_EXT must be ts or js")
	}
	dist := os.DirFS("web/build")
	manifest, err := skgo.ReadManifest(dist)
	if err != nil {
		t.Fatal(err)
	}
	module := "src/routes/receipt/+page.server." + ext
	if !slices.Equal(manifest.Remotes, []string{hash + "/receipt"}) ||
		!slices.Equal(manifest.Loads, []string{module}) || !slices.Equal(manifest.Actions, []string{module}) ||
		!slices.Contains(manifest.Nodes, module) {
		t.Fatalf("manifest does not describe the selected language: %+v", manifest)
	}
	handler, err := NewHandler(dist)
	if err != nil {
		t.Fatal(err)
	}
	get := func(path string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
		}
		return w
	}
	if got := get("/api/receipt").Body.String(); got != "Go endpoint: migration" {
		t.Fatal(got)
	}
	// The JSON string is the authored argument, not a value discovered from
	// the generated registry or a previous response.
	payload := base64.RawURLEncoding.EncodeToString([]byte(`["handler"]`))
	remote := get("/_app/remote/" + hash + "/receipt?payload=" + payload).Body.String()
	var result struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	wantRemote := `[{"_":1,"q":4},{"message":2,"count":3},"Go remote: handler",7,{"` + hash + `/receipt/WyJoYW5kbGVyIl0":5},{"v":1}]`
	if err := json.Unmarshal([]byte(remote), &result); err != nil || result.Type != "result" || result.Data != wantRemote {
		t.Fatalf("remote result: %s", remote)
	}
	data := get("/receipt/__data.json").Body.String()
	if data != "{\"type\":\"data\",\"nodes\":[null,{\"type\":\"data\",\"data\":[{\"count\":1,\"message\":2},11,\"Go load: migration\"],\"uses\":{}}]}\n" {
		t.Fatalf("server data: %s", data)
	}
	document := get("/receipt").Body.String()
	if !strings.Contains(document, `id="load">Go load: migration (11)</p>`) || !strings.Contains(document, "Request Go remote") {
		t.Fatalf("Go-rendered document omitted its usable receipt: %s", document)
	}
	r := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:8080/receipt?/save", strings.NewReader(""))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.Header.Set("Origin", "http://127.0.0.1:8080")
	r.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `id="action">Go action: saved</p>`) {
		t.Fatalf("Go action: %d %s", w.Code, w.Body.String())
	}
	t.Logf("%s: literal remote %s/receipt, server data, rendered load, action and endpoint all answered through the production handler", ext, hash)
}
