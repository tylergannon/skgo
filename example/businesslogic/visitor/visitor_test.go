package visitor_test

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/example"
	"github.com/tylergannon/skgo/example/businesslogic/visitor"
	"github.com/tylergannon/skgo/example/web"
)

const origin = "http://127.0.0.1:8080"

// count is the live count the /todos document was rendered with.
var count = regexp.MustCompile(`data-testid="count"[^>]*>(\d+)<`)

func handler(t *testing.T) http.Handler {
	t.Helper()
	dist, err := fs.Sub(web.Build, "build")
	if err != nil {
		t.Fatal(err)
	}
	h, _, err := example.NewHandler(dist, "", origin)
	if err != nil {
		t.Fatalf("assembling the production stack: %v", err)
	}
	return h
}

// page asks for /todos. browser adds the header every browser sends and a Go
// test, curl or the generated client does not.
func page(t *testing.T, h http.Handler, cookie *http.Cookie, browser bool) (body string, minted *http.Cookie) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/todos", nil)
	if browser {
		req.Header.Set("Sec-Fetch-Site", "none")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /todos: status %d", rec.Code)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == visitor.Cookie {
			minted = c
		}
	}
	return rec.Body.String(), minted
}

func add(t *testing.T, h http.Handler, cookie *http.Cookie, text string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/todos", strings.NewReader(`{"text":"`+text+`"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	if cookie != nil {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/todos %q: status %d: %s", text, rec.Code, rec.Body.String())
	}
}

func renderedCount(t *testing.T, body string) string {
	t.Helper()
	m := count.FindStringSubmatch(body)
	if m == nil {
		t.Fatal("the /todos document carried no rendered count")
	}
	return m[1]
}

// Each browser gets a list of its own, from the first document it is sent, and
// everything that is not a browser keeps sharing the one list it always has.
// The numbers are the store's fixtures — four todos a signed-out visitor may
// see — plus what this test added, not anything read back from the page.
func TestEachBrowserKeepsItsOwnTodos(t *testing.T) {
	h := handler(t)

	// Written by a caller that is not a browser, so it lands on the shared
	// list — and must never reach a browser's.
	const shared = "written by something that is not a browser"
	add(t, h, nil, shared)

	first, ada := page(t, h, nil, true)
	if ada == nil {
		t.Fatal("a browser's first page carried no visitor cookie")
	}
	// The document that hands the cookie out is already the new visitor's:
	// its queries ran before the browser had the cookie to send.
	if strings.Contains(first, shared) {
		t.Error("a browser's first document showed the shared list")
	}
	if got := renderedCount(t, first); got != "4" {
		t.Errorf("a new browser's first document counted %s todos, want the 4 fixtures", got)
	}

	const mine = "added by the first browser"
	add(t, h, ada, mine)
	again, reissued := page(t, h, ada, true)
	if reissued != nil {
		t.Errorf("a browser that sent its visitor cookie was handed another: %v", reissued)
	}
	if !strings.Contains(again, mine) || renderedCount(t, again) != "5" {
		t.Errorf("the first browser does not see its own todo counted: count %s", renderedCount(t, again))
	}

	other, grace := page(t, h, nil, true)
	if grace == nil || grace.Value == ada.Value {
		t.Fatalf("a second browser was not given an id of its own: %v", grace)
	}
	if strings.Contains(other, mine) || renderedCount(t, other) != "4" {
		t.Errorf("a second browser sees the first browser's list: count %s", renderedCount(t, other))
	}

	nobody, handed := page(t, h, nil, false)
	if handed != nil {
		t.Errorf("a caller that is not a browser was handed a visitor cookie: %v", handed)
	}
	if !strings.Contains(nobody, shared) {
		t.Error("a caller that is not a browser no longer sees the shared list it wrote to")
	}
	if strings.Contains(nobody, mine) {
		t.Error("the shared list shows a browser's own todo")
	}
}
