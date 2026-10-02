package example_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic"
)

// The /request-fetch page's Go load calls the Go endpoint /api/request-fetch
// with Event.Fetch. The expected value is a literal this file states: nothing
// in the app but that endpoint's constant can produce it, so seeing it in the
// document means the fetch really ran and its answer came back.

func getAs(t *testing.T, h http.Handler, path string, session string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if session != "" {
		req.AddCookie(&http.Cookie{Name: example.SessionCookie, Value: session})
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestALoadsRequestFetchIsInTheColdDocument(t *testing.T) {
	h := newProdHandler(t)

	rec := getAs(t, h, "/request-fetch", "")
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, body)
	}
	for _, want := range []string{
		`<p data-testid="request-fetch-fact">harbour-lamp-4096</p>`,
		`<p data-testid="request-fetch-visitor">Fetched as guest</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the document does not contain %s", want)
		}
	}
}

func TestTheLoadsFetchCarriesTheVisitorsSession(t *testing.T) {
	h := newProdHandler(t)
	session := businesslogic.Default.SignIn("ada")
	defer businesslogic.Default.SignOut(session)

	body := getAs(t, h, "/request-fetch", session).Body.String()
	if !strings.Contains(body, `<p data-testid="request-fetch-visitor">Fetched as ada</p>`) {
		t.Error("the endpoint did not see the visitor's session cookie on the load's subrequest")
	}
	// An unknown session is a guest, so the line above is the cookie's doing.
	body = getAs(t, h, "/request-fetch", "not-a-session").Body.String()
	if !strings.Contains(body, `Fetched as guest`) {
		t.Error("an unknown session was not a guest")
	}
}

func TestAClientNavigationGetsTheLoadsFetchedValueFromGo(t *testing.T) {
	h := newProdHandler(t)

	rec := getAs(t, h, "/request-fetch/__data.json?x-sveltekit-invalidated=01", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"harbour-lamp-4096"`) || !strings.Contains(body, `"guest"`) {
		t.Errorf("the data response does not carry the fetched values: %s", body)
	}
	// Kit records no dependency for a fetch made by a server load.
	if !strings.Contains(body, `"uses":{}`) {
		t.Errorf("the fetch recorded a dependency: %s", body)
	}
}

func TestTheEndpointAnswersABrowserTheSameWay(t *testing.T) {
	h := newProdHandler(t)

	rec := getAs(t, h, "/api/request-fetch", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got, want := strings.TrimSpace(rec.Body.String()), `{"fact":"harbour-lamp-4096","visitor":"guest"}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}
