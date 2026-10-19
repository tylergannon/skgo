//go:build skgo_qualification

package example_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

// idOfRemote finds the `<hash>/<name>` id of a remote function in the
// generated registry, so a test never hard-codes the hash Kit derives from the
// file's path.
func idOfRemote(t *testing.T, name string) string {
	t.Helper()
	for _, fn := range generated.Remotes() {
		if strings.HasSuffix(fn.ID(), "/"+name) {
			return fn.ID()
		}
	}
	t.Fatalf("no remote function named %s is registered", name)
	return ""
}

// encodeArgument encodes a devalue document as Kit's client sends it:
// base64url without padding.
func encodeArgument(document string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(document))
}

// remoteResult reads the devalue document out of Kit's remote envelope.
func remoteResult(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var envelope struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil || envelope.Type != "result" {
		t.Fatalf("not a result envelope (%v): %s", err, rec.Body.String())
	}
	return envelope.Data
}

// TestAQueryAnswersAsKitsClientCallsIt is a GET with the argument in
// ?payload=. The answer carries the value and the cache entry Kit's client
// files it under, keyed by the id and the payload.
func TestAQueryAnswersAsKitsClientCallsIt(t *testing.T) {
	h := newTestHandler(t)
	id, payload := idOfRemote(t, "shout"), encodeArgument(`["hello"]`)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/_app/remote/"+id+"?payload="+payload, nil))
	want := `[{"_":1,"q":2},"HELLO!",{"` + id + "/" + payload + `":3},{"v":1}]`
	if got := remoteResult(t, rec); got != want {
		t.Fatalf("shout answered %s, want %s", got, want)
	}
}

// TestACommandAnswersAsKitsClientPostsIt is a JSON POST from the
// application's own origin.
func TestACommandAnswersAsKitsClientPostsIt(t *testing.T) {
	h := newTestHandler(t)
	body := `{"payload":"` + encodeArgument(`[21]`) + `","refreshes":[]}`
	req := httptest.NewRequest(http.MethodPost, "/_app/remote/"+idOfRemote(t, "double"), strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if got := remoteResult(t, rec); got != `[{"_":1},42]` {
		t.Fatalf("double answered %s", got)
	}
}
