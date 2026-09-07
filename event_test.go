package skgo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tylergannon/skgo/internal/remotearg"
)

// postCommand invokes a command the way kit's client does and returns the
// recorder so a test can read both body and headers.
func postCommand(t *testing.T, rs *Remotes, fn *Remote, arg any, refreshes []string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := remotearg.StringifyCommandArg(arg)
	if err != nil {
		t.Fatalf("stringifying command arg: %v", err)
	}
	body := `{"payload":` + quote(payload) + `,"refreshes":` + jsonList(refreshes) + `}`
	req := httptest.NewRequest(http.MethodPost, rs.Prefix()+fn.ID(), strings.NewReader(body))
	req.Header.Set("Origin", rs.cfg.Origin)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)
	return rec
}

func quote(s string) string {
	raw, err := jsonBytes(s)
	if err != nil {
		panic(err)
	}
	return string(raw)
}

func jsonList(items []string) string {
	raw, err := jsonBytes(items)
	if err != nil {
		panic(err)
	}
	if items == nil {
		return "[]"
	}
	return string(raw)
}

func TestCommandSetsCookieWithKitDefaults(t *testing.T) {
	login := NewCommand(testModule, "login", func(ctx context.Context, user string) (string, error) {
		if err := EventFrom(ctx).SetCookie("session", user, CookieOptions{MaxAge: 3600}); err != nil {
			return "", err
		}
		return user, nil
	})
	rs := testRemotes(t, RemoteConfig{Origin: "https://example.com"}, login)

	rec := postCommand(t, rs, login, "ada", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	got := rec.Header().Values("Set-Cookie")
	if len(got) != 1 {
		t.Fatalf("Set-Cookie headers = %v, want exactly one", got)
	}
	// Kit's defaults: httpOnly, path "/", sameSite lax, secure off localhost.
	for _, want := range []string{"session=ada", "Max-Age=3600", "Path=/", "HttpOnly", "SameSite=Lax", "Secure"} {
		if !strings.Contains(got[0], want) {
			t.Errorf("Set-Cookie %q is missing %q", got[0], want)
		}
	}
}

func TestCommandDeleteCookieExpiresIt(t *testing.T) {
	logout := NewCommandNoArg(testModule, "logout", func(ctx context.Context) (bool, error) {
		return true, EventFrom(ctx).DeleteCookie("session", CookieOptions{})
	})
	rs := testRemotes(t, RemoteConfig{Origin: "http://localhost:8080"}, logout)

	rec := postCommand(t, rs, logout, nil, nil, &http.Cookie{Name: "session", Value: "ada"})
	got := rec.Header().Get("Set-Cookie")
	// Kit deletes with `maxAge: 0` and an empty value; net/http spells a
	// negative MaxAge as `Max-Age=0`.
	if !strings.HasPrefix(got, "session=;") || !strings.Contains(got, "Max-Age=0") {
		t.Fatalf("Set-Cookie = %q, want an expiring empty session cookie", got)
	}
	// An app served over plain HTTP from localhost gets non-Secure cookies,
	// exactly as kit's own `secure` default does.
	if strings.Contains(got, "Secure") {
		t.Errorf("Set-Cookie = %q, want no Secure attribute on http://localhost", got)
	}
}

func TestCookieWrittenInCallIsVisibleToTheSameCall(t *testing.T) {
	var seen string
	login := NewCommand(testModule, "login", func(ctx context.Context, user string) (string, error) {
		if err := EventFrom(ctx).SetCookie("session", user, CookieOptions{}); err != nil {
			return "", err
		}
		seen, _ = EventFrom(ctx).Cookie("session")
		return user, nil
	})
	rs := testRemotes(t, RemoteConfig{}, login)

	postCommand(t, rs, login, "ada", nil, &http.Cookie{Name: "session", Value: "stale"})
	if seen != "ada" {
		t.Fatalf("cookie read back as %q, want the value just written", seen)
	}
}

func TestDeletedCookieReadsAsAbsent(t *testing.T) {
	var present bool
	logout := NewCommandNoArg(testModule, "logout", func(ctx context.Context) (bool, error) {
		if err := EventFrom(ctx).DeleteCookie("session", CookieOptions{}); err != nil {
			return false, err
		}
		_, present = EventFrom(ctx).Cookie("session")
		return true, nil
	})
	rs := testRemotes(t, RemoteConfig{}, logout)

	postCommand(t, rs, logout, nil, nil, &http.Cookie{Name: "session", Value: "ada"})
	if present {
		t.Fatal("a cookie deleted in this call still reads as present")
	}
}

func TestRefreshedQuerySeesTheCookieTheCommandJustSet(t *testing.T) {
	// This is the single-flight sign-in: `login(...).updates(whoami())` must
	// come back carrying the signed-in answer, or the page shows the
	// signed-out one until the next navigation.
	whoami := NewQueryNoArg(testModule, "whoami", func(ctx context.Context) (string, error) {
		user, _ := EventFrom(ctx).Cookie("session")
		return user, nil
	})
	login := NewCommand(testModule, "login", func(ctx context.Context, user string) (string, error) {
		return user, EventFrom(ctx).SetCookie("session", user, CookieOptions{})
	})
	rs := testRemotes(t, RemoteConfig{}, whoami, login)

	rec := postCommand(t, rs, login, "ada", []string{whoami.ID() + "/"})
	_, data, _ := envelope(t, rec.Body.Bytes())
	if got := field(t, node(t, data, whoami.ID()+"/"), "v"); got != "ada" {
		t.Fatalf("refreshed whoami = %#v, want %q", got, "ada")
	}
}

func TestRelativeCookiePathIsRefused(t *testing.T) {
	// Kit: "Cookies set in remote functions must have an absolute path".
	bad := NewCommandNoArg(testModule, "bad", func(ctx context.Context) (bool, error) {
		if err := EventFrom(ctx).SetCookie("session", "ada", CookieOptions{Path: "todos"}); err != nil {
			return false, err
		}
		return true, nil
	})
	rs := testRemotes(t, RemoteConfig{}, bad)

	rec := postCommand(t, rs, bad, nil, nil)
	kind, _, httpErr := envelope(t, rec.Body.Bytes())
	if kind != "error" {
		t.Fatalf("envelope type = %q, want an error", kind)
	}
	if msg, _ := httpErr["message"].(string); !strings.Contains(msg, "absolute path") {
		t.Fatalf("error message = %q, want it to name the absolute-path rule", msg)
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
}

func TestQueryCannotWriteCookies(t *testing.T) {
	// Kit throws "Cannot set cookies in `query` or `prerender` functions"
	// because a query's result is cached by its argument and replayed from
	// that cache, so the cookie would be written once and then quietly
	// skipped. skgo returns the refusal instead of throwing it.
	var setErr, deleteErr error
	q := NewQueryNoArg(testModule, "whoami", func(ctx context.Context) (string, error) {
		e := EventFrom(ctx)
		setErr = e.SetCookie("session", "ada", CookieOptions{})
		deleteErr = e.DeleteCookie("session", CookieOptions{})
		user, _ := e.Cookie("session")
		return user, nil
	})
	rs := testRemotes(t, RemoteConfig{}, q)

	req := httptest.NewRequest(http.MethodGet, rs.Prefix()+q.ID(), nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "ada"})
	rec := httptest.NewRecorder()
	rs.ServeHTTP(rec, req)

	for _, err := range []error{setErr, deleteErr} {
		if err == nil {
			t.Fatal("a query wrote a cookie")
		}
		if !strings.Contains(err.Error(), "only a command") {
			t.Fatalf("error = %q, want it to name the rule", err)
		}
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 0 {
		t.Fatalf("Set-Cookie = %v, want none", got)
	}
	_, data, _ := envelope(t, rec.Body.Bytes())
	if got := field(t, data, "_"); got != "ada" {
		t.Fatalf("whoami = %#v, want the request cookie it may still read", got)
	}
}

func TestARefreshedQueryStillCannotWriteCookies(t *testing.T) {
	// The refreshes a command resolves share its cookie jar, so they must not
	// also inherit its permission to write.
	var refreshErr error
	whoami := NewQueryNoArg(testModule, "whoami", func(ctx context.Context) (string, error) {
		refreshErr = EventFrom(ctx).SetCookie("session", "eve", CookieOptions{})
		return "", nil
	})
	login := NewCommand(testModule, "login", func(ctx context.Context, user string) (string, error) {
		return user, EventFrom(ctx).SetCookie("session", user, CookieOptions{})
	})
	rs := testRemotes(t, RemoteConfig{}, whoami, login)

	rec := postCommand(t, rs, login, "ada", []string{whoami.ID() + "/"})
	if refreshErr == nil {
		t.Fatal("a query refreshed by a command wrote a cookie")
	}
	if got := rec.Header().Values("Set-Cookie"); len(got) != 1 || !strings.Contains(got[0], "session=ada") {
		t.Fatalf("Set-Cookie = %v, want only the command's cookie", got)
	}
}

func TestEventOutsideARemoteFunction(t *testing.T) {
	// A helper shared with ordinary server code must not have to branch: every
	// method is safe on the nil event EventFrom returns outside a call.
	e := EventFrom(context.Background())
	if e != nil {
		t.Fatalf("EventFrom(background) = %v, want nil", e)
	}
	if v, ok := e.Cookie("session"); ok || v != "" {
		t.Fatalf("Cookie on a nil event = %q, %v", v, ok)
	}
	if e.Request() != nil {
		t.Fatal("Request on a nil event is not nil")
	}
	if err := e.SetCookie("session", "ada", CookieOptions{}); err == nil {
		t.Fatal("SetCookie on a nil event succeeded")
	}
}

func TestSecureCookieDefaultMirrorsKit(t *testing.T) {
	for _, tc := range []struct {
		origin string
		dev    bool
		want   bool
	}{
		{"http://localhost:5173", false, false},
		{"http://localhost", false, false},
		{"https://localhost", false, true},
		// Kit's check is on the literal hostname "localhost", so the loopback
		// address gets Secure cookies.
		{"http://127.0.0.1:8080", false, true},
		{"https://example.com", false, true},
		{"http://example.com", false, true},
		{"https://example.com", true, false},
		{"", false, true},
	} {
		if got := secureCookieDefault(tc.origin, tc.dev); got != tc.want {
			t.Errorf("secureCookieDefault(%q, dev=%v) = %v, want %v", tc.origin, tc.dev, got, tc.want)
		}
	}
}

func TestRegistryRefusesToStartWhenTheBuildDisagrees(t *testing.T) {
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })

	manifest := Manifest{AppDir: "_app", Remotes: []string{q.ID(), "abc123/vanished"}}
	_, err := NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"), q)
	if err == nil {
		t.Fatal("NewRemotes accepted a registry missing a function the frontend calls")
	}
	if !strings.Contains(err.Error(), "abc123/vanished") {
		t.Fatalf("error = %q, want it to name the missing id", err)
	}

	manifest = Manifest{AppDir: "_app", Remotes: []string{}}
	if _, err := NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"), q); err == nil {
		t.Fatal("NewRemotes accepted a registry serving a function the frontend never calls")
	}

	manifest = Manifest{AppDir: "_app", Remotes: []string{q.ID()}}
	if _, err := NewRemotes(manifest.RemoteConfig("http://127.0.0.1:8080"), q); err != nil {
		t.Fatalf("NewRemotes rejected a matching registry: %v", err)
	}
}

func TestDevModeSkipsTheBuildCheck(t *testing.T) {
	// In dev the client comes from vite, not from `build/`, so the manifest's
	// list describes the last production build and says nothing about what is
	// running.
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	cfg := Manifest{AppDir: "_app", Remotes: []string{"abc123/vanished"}}.RemoteConfig("")
	cfg.Dev = true
	if _, err := NewRemotes(cfg, q); err != nil {
		t.Fatalf("NewRemotes refused in dev mode: %v", err)
	}
}

func TestHandBuiltConfigIsNotCheckedAgainstAManifest(t *testing.T) {
	q := NewQueryNoArg(testModule, "getTodos", func(ctx context.Context) (int, error) { return 1, nil })
	if _, err := NewRemotes(RemoteConfig{}, q); err != nil {
		t.Fatalf("NewRemotes refused a config that never came from a build: %v", err)
	}
}
