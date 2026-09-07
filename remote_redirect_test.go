package skgo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A caller has to be able to say which redirect they mean. Kit's
// `redirect(status, location)` takes one and validates it; skgo's Redirect
// carried only a location, so 301, 303 and 308 were all the same value.

func TestNewRedirectAcceptsExactlyTheStatusesKitAccepts(t *testing.T) {
	// exports/index.js: `if (isNaN(status) || status < 300 || status > 308)
	// throw new Error('Invalid status code')`.
	for status := 300; status <= 308; status++ {
		if _, err := NewRedirect(status, "/somewhere"); err != nil {
			t.Errorf("NewRedirect(%d): %v", status, err)
		}
	}
	for _, status := range []int{0, 200, 299, 309, 400, 500} {
		r, err := NewRedirect(status, "/somewhere")
		if err == nil {
			t.Errorf("NewRedirect(%d) built %+v; kit refuses it", status, r)
			continue
		}
		if !strings.Contains(err.Error(), "300-308") {
			t.Errorf("NewRedirect(%d) said %q, which does not say what is allowed", status, err)
		}
	}
}

// Kit's Redirect constructor refuses a location `new Headers({location})`
// would reject. A control character in a location is response splitting.
func TestNewRedirectRefusesALocationThatCannotBeAHeader(t *testing.T) {
	for _, location := range []string{"/ok\r\nX-Injected: 1", "/ok\nSet-Cookie: a=b", "/ok\x00"} {
		if r, err := NewRedirect(302, location); err == nil {
			t.Errorf("NewRedirect accepted %q and built %+v", location, r)
		}
	}
	if _, err := NewRedirect(302, "/perfectly ordinary?a=b#c"); err != nil {
		t.Errorf("NewRedirect refused an ordinary location: %v", err)
	}
}

func TestARedirectSaysWhichRedirectItIs(t *testing.T) {
	r, err := NewRedirect(308, "/moved")
	if err != nil {
		t.Fatalf("NewRedirect: %v", err)
	}
	if got := r.Error(); !strings.Contains(got, "308") || !strings.Contains(got, "/moved") {
		t.Errorf("Error() = %q, want it to name the status and the location", got)
	}
}

// The status is deliberately not on the wire, and that is kit's protocol.
// A remote-function redirect is serialised into the *success* envelope as a
// bare location — `runtime/server/remote-functions.js` drops `error.status` —
// and kit's client calls `goto()` with it. Emitting a status here would be
// inventing a field kit's client does not read.
func TestARemoteRedirectCarriesKitsLocationOnlyEnvelope(t *testing.T) {
	rs := testRemotes(t, RemoteConfig{}, NewQueryNoArg(testModule, "gone",
		func(ctx context.Context) (todo, error) {
			r, err := NewRedirect(303, "/elsewhere")
			if err != nil {
				return todo{}, err
			}
			return todo{}, r
		}))

	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, rs.Prefix()+kithashID(rs, "gone"), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	kind, data, _ := envelope(t, body)
	if kind != "result" {
		t.Fatalf("a redirect came back as %s: %s", kind, body)
	}
	if got, _ := object(t, data).Get("redirect"); got != "/elsewhere" {
		t.Errorf("redirect = %v, want /elsewhere: %s", got, body)
	}
	if strings.Contains(string(body), "303") {
		t.Errorf("the status reached the wire; kit's client does not read one: %s", body)
	}
}
