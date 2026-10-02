package example_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// The /destinations page's Go load fetches one of each kind of destination.
// Every expectation is a literal this file states; none is read back from the
// load that produced the answer.

type answer struct {
	Path        string
	Status      int
	ContentType string
	Body        string
}

// devalueObject resolves the one shape this page's data takes: an object of
// strings, numbers and nested arrays of such objects, stored kit-style as a
// flat array of values with integer references.
func devalueAt(t *testing.T, data []json.RawMessage, i int) any {
	t.Helper()
	var raw any
	if err := json.Unmarshal(data[i], &raw); err != nil {
		t.Fatalf("decoding data[%d]: %v", i, err)
	}
	switch v := raw.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, ref := range v {
			out[k] = devalueAt(t, data, int(ref.(float64)))
		}
		return out
	case []any:
		out := make([]any, len(v))
		for j, ref := range v {
			out[j] = devalueAt(t, data, int(ref.(float64)))
		}
		return out
	default:
		return v
	}
}

func destinationAnswers(t *testing.T, h http.Handler, session string) []answer {
	t.Helper()
	rec := getAs(t, h, "/destinations/__data.json?x-sveltekit-invalidated=01", session)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var env struct {
		Nodes []struct {
			Data []json.RawMessage `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(strings.SplitN(rec.Body.String(), "\n", 2)[0]), &env); err != nil {
		t.Fatalf("decoding the data response: %v", err)
	}
	root := devalueAt(t, env.Nodes[1].Data, 0).(map[string]any)
	var out []answer
	for _, item := range root["answers"].([]any) {
		m := item.(map[string]any)
		out = append(out, answer{m["path"].(string), int(m["status"].(float64)), m["contentType"].(string), m["body"].(string)})
	}
	return out
}

func TestALoadFetchesEveryKindOfDestinationTheAppHas(t *testing.T) {
	h := newProdHandler(t)
	session := businesslogic.Default.SignIn("ada")
	defer businesslogic.Default.SignOut(session)

	for _, tc := range []struct {
		who     string
		session string
		visitor string
		whoami  string
	}{{"a guest", "", "guest", `\"user\":2},\"\"`}, {"ada", session, "ada", `\"ada\"`}} {
		answers := destinationAnswers(t, h, tc.session)
		type want struct {
			path, contentType string
			contains          []string
			excludes          []string
		}
		wants := []want{
			{"/request-fetch", "text/html; charset=utf-8",
				[]string{"<!doctype html>", `<p data-testid="request-fetch-fact">harbour-lamp-4096</p>`, `<p data-testid="request-fetch-visitor">Fetched as ` + tc.visitor + `</p>`}, nil},
			{"/request-fetch/__data.json?x-sveltekit-invalidated=01", "application/json",
				[]string{`"harbour-lamp-4096"`, `"` + tc.visitor + `"`}, nil},
			{"/_app/remote/4cga8b/whoami", "application/json",
				[]string{`"type":"result"`, tc.whoami}, nil},
			{"/robots.txt", "text/plain; charset=utf-8", []string{"User-agent: *\nDisallow:\n"}, nil},
			{"/prerender/atlas", "text/html; charset=utf-8", []string{"Go entry load: atlas"}, nil},
			{"/_app/remote/ks8sip/buildReceipt/WyJhdGxhcyJd", "application/json", []string{"Go prerender remote: atlas"}, nil},
			{"/shadow/fixed", "text/html; charset=utf-8", []string{"prerendered:fixed"}, []string{"dynamic-rest"}},
			{"/shadow/other", "text/plain; charset=utf-8", []string{"dynamic-rest:other"}, nil},
			{"/api/request-fetch", "application/json", []string{`{"fact":"harbour-lamp-4096","visitor":"` + tc.visitor + `"}`}, nil},
		}
		if len(answers) != len(wants) {
			t.Fatalf("%s: %d answers, want %d", tc.who, len(answers), len(wants))
		}
		for i, w := range wants {
			got := answers[i]
			if got.Path != w.path || got.Status != 200 || got.ContentType != w.contentType {
				t.Errorf("%s: %s answered %d %q (path %q), want 200 %q", tc.who, w.path, got.Status, got.ContentType, got.Path, w.contentType)
			}
			for _, c := range w.contains {
				if !strings.Contains(got.Body, c) {
					t.Errorf("%s: %s body does not contain %q", tc.who, w.path, c)
				}
			}
			for _, c := range w.excludes {
				if strings.Contains(got.Body, c) {
					t.Errorf("%s: %s body contains %q", tc.who, w.path, c)
				}
			}
		}
	}
}

func TestAPrerenderedPageWinsOverADynamicServerRouteThatAlsoMatchesIt(t *testing.T) {
	h := newProdHandler(t)

	rec := getAs(t, h, "/shadow/fixed", "")
	if body := rec.Body.String(); !strings.Contains(body, "prerendered:fixed") || strings.Contains(body, "dynamic-rest") {
		t.Errorf("GET /shadow/fixed was not the prerendered page: %.200s", body)
	}
	rec = getAs(t, h, "/shadow/other", "")
	if got := rec.Body.String(); got != "dynamic-rest:other" {
		t.Errorf("GET /shadow/other = %q, want the dynamic route's answer", got)
	}
}

func TestANestedUniversalFetchOfARenderedPageIsInTheColdDocument(t *testing.T) {
	h := newProdHandler(t)
	session := businesslogic.Default.SignIn("ada")
	defer businesslogic.Default.SignOut(session)

	body := getAs(t, h, "/nested-universal", session).Body.String()
	for _, want := range []string{
		`<h1 data-testid="title">Nested universal fetch</h1>`,
		`<p data-testid="nested-fact">harbour-lamp-4096</p>`,
		`<p data-testid="nested-visitor">ada</p>`,
		`<p data-testid="nested-status">200</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document does not contain %s", want)
		}
	}
}

// With one runtime per depth, every nested render has to find a runtime that
// is not held by the render waiting on it, and concurrent visitors each get
// their own values back.
func TestNestedRendersFinishUnderCapacityPressureWithEachVisitorsOwnValues(t *testing.T) {
	for _, runtimes := range []int{1, 2} {
		h, _, err := example.NewHandlerSized(prodDist(t), "", prodOrigin, runtimes)
		if err != nil {
			t.Fatal(err)
		}
		const visitors = 12
		var wg sync.WaitGroup
		failures := make(chan string, visitors*2)
		for i := 0; i < visitors; i++ {
			user := fmt.Sprintf("visitor-%d-%d", runtimes, i)
			session := businesslogic.Default.SignIn(user)
			defer businesslogic.Default.SignOut(session)
			wg.Add(1)
			go func() {
				defer wg.Done()
				for _, path := range []string{"/nested-universal", "/destinations"} {
					req := httptest.NewRequest(http.MethodGet, path, nil)
					req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
					rec := httptest.NewRecorder()
					h.ServeHTTP(rec, req)
					want := `data-path="/api/request-fetch"`
					if path == "/nested-universal" {
						want = `<p data-testid="nested-visitor">` + user + `</p>`
					}
					if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), want) {
						failures <- fmt.Sprintf("%s as %s: status %d, missing %s", path, user, rec.Code, want)
					}
				}
			}()
		}
		done := make(chan struct{})
		go func() { wg.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(60 * time.Second):
			t.Fatalf("%d runtime(s): nested renders deadlocked under %d concurrent visitors", runtimes, visitors)
		}
		close(failures)
		for f := range failures {
			t.Errorf("%d runtime(s): %s", runtimes, f)
		}
	}
}
