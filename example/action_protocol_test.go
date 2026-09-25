package example_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func actionRequest(h http.Handler, method, target, body string, headers http.Header, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	for name, values := range headers {
		for _, value := range values {
			req.Header.Add(name, value)
		}
	}
	for _, cookie := range cookies {
		req.AddCookie(cookie)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPageActionRequestProtocol(t *testing.T) {
	h := newProdHandler(t)
	page := actionRequest(h, http.MethodGet, "/actions", "", http.Header{"Accept": {"text/html"}})
	if page.Code != 200 || !strings.Contains(page.Body.String(), `<input name="name" value="Ada Lovelace"`) {
		t.Fatalf("literal Ada fixture did not render: status %d", page.Code)
	}
	cookies := page.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("fixture did not issue a workspace cookie")
	}
	form := url.Values{"name": {"Grace Hopper"}, "email": {"grace@example.test"}, "biography": {"Protocol fixture"}}.Encode()
	jsonHeaders := http.Header{"Accept": {"application/json"}, "X-Sveltekit-Action": {"true"}, "Content-Type": {"application/x-www-form-urlencoded"}, "Origin": {prodOrigin}}

	for _, tc := range []struct {
		name, target, contentType, wantMessage string
		status, errorStatus                    int
	}{
		{"missing selector", "/actions", "application/x-www-form-urlencoded", "No action with name 'default' found", 404, 404},
		{"missing named action", "/actions?/missing", "application/x-www-form-urlencoded", "No action with name 'missing' found", 404, 404},
		{"constructor", "/actions?/constructor", "application/x-www-form-urlencoded", "No action with name 'constructor' found", 404, 404},
		{"toString", "/actions?/toString", "application/x-www-form-urlencoded", "No action with name 'toString' found", 404, 404},
		{"reserved default", "/actions?/default", "application/x-www-form-urlencoded", "Something went wrong on our end.", 500, 500},
		{"unsupported content type", "/actions?/save", "application/json", "Form actions expect form-encoded data — received application/json", 415, 415},
		{"missing content type", "/actions?/save", "", "Form actions expect form-encoded data — received null", 415, 415},
	} {
		t.Run(tc.name, func(t *testing.T) {
			headers := jsonHeaders.Clone()
			if tc.contentType == "" {
				headers.Del("Content-Type")
			} else {
				headers.Set("Content-Type", tc.contentType)
			}
			rec := actionRequest(h, http.MethodPost, tc.target, form, headers, cookies...)
			if rec.Code != tc.status || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
				t.Fatalf("status %d, type %q, body %s", rec.Code, rec.Header().Get("Content-Type"), rec.Body.String())
			}
			var result struct {
				Type     string `json:"type"`
				Location string `json:"location"`
				Error    struct {
					Status  int    `json:"status"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Type != "error" || result.Error.Status != tc.errorStatus || result.Error.Message != tc.wantMessage || result.Location != "/actions" {
				t.Fatalf("unexpected action result: %+v", result)
			}
		})
	}
	rec := actionRequest(h, http.MethodPost, "/actions/signed-in", form, jsonHeaders, cookies...)
	if rec.Code != 405 || rec.Header().Get("Allow") != "GET" || !strings.Contains(rec.Body.String(), `"type":"error"`) {
		t.Fatalf("page without actions: status %d, Allow %q, body %s", rec.Code, rec.Header().Get("Allow"), rec.Body.String())
	}
	for _, contentType := range []string{"application/x-www-form-urlencoded", "multipart/form-data; boundary=fixture", "text/plain", "application/x-sveltekit-formdata"} {
		headers := jsonHeaders.Clone()
		headers.Set("Content-Type", contentType)
		rec := actionRequest(h, http.MethodPost, "/actions?/remote", "", headers, cookies...)
		if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"success"`) {
			t.Errorf("accepted form type %q: %d %s", contentType, rec.Code, rec.Body.String())
		}
	}

	// A refused cross-origin form must leave the literal fixture untouched.
	cross := jsonHeaders.Clone()
	cross.Set("Origin", "https://evil.test")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, cross, cookies...)
	if rec.Code != 403 || !strings.Contains(rec.Body.String(), "Cross-site POST form submissions are forbidden") {
		t.Fatalf("cross-origin form: %d %s", rec.Code, rec.Body.String())
	}
	after := actionRequest(h, http.MethodGet, "/actions", "", http.Header{"Accept": {"text/html"}}, cookies...)
	if after.Code != 200 || !strings.Contains(after.Body.String(), `<input name="name" value="Ada Lovelace"`) || strings.Contains(after.Body.String(), `<input name="name" value="Grace Hopper"`) {
		t.Fatalf("refused request mutated fixture: status %d", after.Code)
	}

	// A JSON body is not a CSRF form body. It reaches the action's 415.
	nonForm := jsonHeaders.Clone()
	nonForm.Set("Origin", "https://evil.test")
	nonForm.Set("Content-Type", "application/json")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, nonForm, cookies...)
	if rec.Code != 415 || !strings.Contains(rec.Body.String(), "received application/json") {
		t.Fatalf("non-form body: %d %s", rec.Code, rec.Body.String())
	}

	// Kit selects JSON by Accept and uses the action header separately to
	// disambiguate a sibling endpoint. A document request selects the page.
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, jsonHeaders, cookies...)
	if rec.Code != 200 || rec.Header().Get("X-Skgo-Action-Demo") != "profile-saved" || !strings.Contains(rec.Body.String(), `"type":"success"`) {
		t.Fatalf("enhanced action: %d %s", rec.Code, rec.Body.String())
	}
	noHeader := jsonHeaders.Clone()
	noHeader.Del("X-Sveltekit-Action")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, noHeader, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Endpoint POST answered by Go") {
		t.Fatalf("endpoint POST selection: %d %s", rec.Code, rec.Body.String())
	}
	native := jsonHeaders.Clone()
	native.Set("Accept", "text/html")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, native, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Saved Grace Hopper") || rec.Header().Get("X-Skgo-Action-Demo") != "profile-saved" {
		t.Fatalf("native page action: %d", rec.Code)
	}

	// No sibling POST exists here, so JSON reaches the default page action
	// without an action header. The header alone cannot change HTML to JSON.
	rec = actionRequest(h, http.MethodPost, "/actions/default", form, noHeader, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"success"`) {
		t.Fatalf("JSON fallback to page action: %d %s", rec.Code, rec.Body.String())
	}
	noAccept := jsonHeaders.Clone()
	noAccept.Del("Accept")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, noAccept, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `"type":"success"`) {
		t.Fatalf("absent Accept did not negotiate as */*: %d %s", rec.Code, rec.Body.String())
	}
	noAccept.Del("X-Sveltekit-Action")
	rec = actionRequest(h, http.MethodPost, "/actions?/save", form, noAccept, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Endpoint POST answered by Go") {
		t.Fatalf("absent Accept without action header: %d %s", rec.Code, rec.Body.String())
	}
	rec = actionRequest(h, http.MethodPost, "/actions?/remote", "", native, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Classic remote-named action answered by Go") {
		t.Fatalf("remote-named classic action: %d", rec.Code)
	}
	remote := remoteID(t, "sendRemoteNote")
	selector := "/actions?/remote=" + remote
	rec = actionRequest(h, http.MethodPost, selector, url.Values{"name/" + remote: {"Grace Hopper"}}.Encode(), native, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Remote Go form received Grace Hopper") {
		t.Fatalf("native remote selector: %d", rec.Code)
	}
	rec = actionRequest(h, http.MethodPost, selector, "", jsonHeaders, cookies...)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Classic remote-named action answered by Go") || !strings.Contains(rec.Body.String(), `"type":"success"`) {
		t.Fatalf("JSON classic selector precedence: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTrustedOriginReachesPageActionWithoutOpeningUntrustedMutation(t *testing.T) {
	h := newProdHandler(t)
	page := actionRequest(h, http.MethodGet, "/actions", "", http.Header{"Accept": {"text/html"}})
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `<input name="name" value="Ada Lovelace"`) {
		t.Fatalf("literal Ada fixture: status %d", page.Code)
	}
	cookies := page.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("fixture did not issue a workspace cookie")
	}
	checkName := func(want string) {
		t.Helper()
		after := actionRequest(h, http.MethodGet, "/actions", "", http.Header{"Accept": {"text/html"}}, cookies...)
		if after.Code != http.StatusOK || !strings.Contains(after.Body.String(), `<input name="name" value="`+want+`"`) {
			t.Fatalf("fixture name %q not rendered: status %d", want, after.Code)
		}
	}
	submit := func(origin, accept, name string, enhanced bool) *httptest.ResponseRecorder {
		t.Helper()
		headers := http.Header{
			"Origin":       {origin},
			"Accept":       {accept},
			"Content-Type": {"application/x-www-form-urlencoded"},
		}
		if enhanced {
			headers.Set("X-Sveltekit-Action", "true")
		}
		body := url.Values{"name": {name}, "email": {"fixture@example.test"}, "biography": {"Trusted origin fixture"}}.Encode()
		return actionRequest(h, http.MethodPost, "/actions?/save", body, headers, cookies...)
	}

	// The JSON request must select the page action even though /actions also
	// has a POST endpoint. Its action-only response header identifies that path.
	rec := submit("https://evil.test", "application/json", "Untrusted First", true)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Cross-site POST form submissions are forbidden") {
		t.Fatalf("untrusted JSON action: %d %s", rec.Code, rec.Body.String())
	}
	checkName("Ada Lovelace")
	rec = submit("https://trusted.test", "application/json", "Grace Hopper", true)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Skgo-Action-Demo") != "profile-saved" || !strings.Contains(rec.Body.String(), `"type":"success"`) || !strings.Contains(rec.Body.String(), "Saved Grace Hopper") {
		t.Fatalf("trusted JSON page action: %d %s", rec.Code, rec.Body.String())
	}
	checkName("Grace Hopper")
	endpointHeaders := http.Header{
		"Origin":       {"https://trusted.test"},
		"Accept":       {"application/json"},
		"Content-Type": {"application/x-www-form-urlencoded"},
	}
	endpointBody := url.Values{"name": {"Endpoint Only"}}.Encode()
	rec = actionRequest(h, http.MethodPost, "/actions?/save", endpointBody, endpointHeaders, cookies...)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Endpoint POST answered by Go") || rec.Header().Get("X-Skgo-Action-Demo") != "" {
		t.Fatalf("trusted sibling endpoint: %d %s", rec.Code, rec.Body.String())
	}
	checkName("Grace Hopper")

	// A native form selects the page by Accept and returns rendered action data.
	rec = submit("https://trusted.test", "text/html", "Katherine Johnson", false)
	if rec.Code != http.StatusOK || rec.Header().Get("X-Skgo-Action-Demo") != "profile-saved" || !strings.Contains(rec.Body.String(), "Saved Katherine Johnson") {
		t.Fatalf("trusted native page action: %d %s", rec.Code, rec.Body.String())
	}
	checkName("Katherine Johnson")
	rec = submit("https://evil.test", "text/html", "Untrusted Last", false)
	if rec.Code != http.StatusForbidden || !strings.Contains(rec.Body.String(), "Cross-site POST form submissions are forbidden") {
		t.Fatalf("untrusted native action: %d %s", rec.Code, rec.Body.String())
	}
	checkName("Katherine Johnson")
}

func TestPageOnlyOptionsAndAllow(t *testing.T) {
	h := newProdHandler(t)
	for _, tc := range []struct {
		path, optionsAllow, methodAllow string
	}{
		{"/actions/profiles/ada", "GET, HEAD, OPTIONS, POST", "GET, POST, OPTIONS, HEAD"},
		{"/actions/options/no-client", "GET, HEAD, OPTIONS, POST", "GET, POST, OPTIONS, HEAD"},
		{"/actions/signed-in", "GET, HEAD, OPTIONS", "GET, OPTIONS, HEAD"},
	} {
		rec := actionRequest(h, http.MethodOptions, tc.path, "", nil)
		if rec.Code != 204 || rec.Header().Get("Allow") != tc.optionsAllow || rec.Body.Len() != 0 {
			t.Errorf("OPTIONS %s: status %d, Allow %q, body %q", tc.path, rec.Code, rec.Header().Get("Allow"), rec.Body.String())
		}
		rec = actionRequest(h, http.MethodPut, tc.path, "", http.Header{"Origin": {prodOrigin}, "Content-Type": {"application/json"}})
		if rec.Code != 405 || rec.Header().Get("Allow") != tc.methodAllow || !strings.Contains(rec.Body.String(), "PUT method not allowed") {
			t.Errorf("PUT %s: status %d, Allow %q, body %q", tc.path, rec.Code, rec.Header().Get("Allow"), rec.Body.String())
		}
	}
	// A sibling endpoint owns OPTIONS even when it has no OPTIONS export.
	// Kit does not synthesize page OPTIONS for a route with an endpoint.
	for _, tc := range []struct{ path, allow string }{
		{"/actions", "POST"},
		{"/actions/default", "GET, HEAD"},
	} {
		rec := actionRequest(h, http.MethodOptions, tc.path, "", nil)
		if rec.Code != 405 || rec.Header().Get("Allow") != tc.allow {
			t.Errorf("shared OPTIONS %s: status %d, Allow %q", tc.path, rec.Code, rec.Header().Get("Allow"))
		}
	}
}
