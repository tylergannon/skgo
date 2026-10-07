package devrender_test

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	generated "github.com/tylergannon/skgo/example/internal/skgo"
)

// The receipts are authored literals, independent of GET/SSR output.
func TestNativeTypedCallerForms(t *testing.T) {
	h := handler
	id := ""
	for _, remote := range generated.Remotes() {
		if remote.Name() == "sendMessage" {
			id = remote.ID()
		}
	}
	if id == "" {
		t.Fatal("sendMessage is not registered")
	}
	for _, caller := range []struct{ path, receipt string }{
		{"/typed-load/42", "number:params.NumberParam_OrderNumber:{42}"},
		{"/typed-load/0", "number:params.NumberParam_OrderNumber:{0}"},
		{"/items/42", "id:string:42"},
		{"/items/%2525", "id:string:%25"},
		{"/items/a%2Fb", "id:string:a/b"},
		{"/items/%2F", "id:string:/"},
		{"/items/%E2%9C%93", "id:string:✓"},
		{"/contact", "absent"},
	} {
		for _, keyed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/keyed=%t", caller.path, keyed), func(t *testing.T) {
				action := id
				prefix := ""
				if keyed {
					action += `/"k1"`
					prefix = "keyed-"
				}
				fields := url.Values{
					"from/" + id:  {"Native fixture"},
					"email/" + id: {"native@example.test"},
					"body/" + id:  {"Literal native caller payload."},
				}
				req := httptest.NewRequest("POST", caller.path+"?"+url.Values{"/remote": {action}}.Encode(), strings.NewReader(fields.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				req.Header.Set("Origin", origin)
				req.Header.Set("Accept", "text/html")
				// Native caller state comes from the page, even if enhanced
				// caller headers try to name another route.
				req.Header.Set("x-sveltekit-pathname", "/items/spoof")
				rec := httptest.NewRecorder()
				h.ServeHTTP(rec, req)
				if rec.Code != 200 {
					t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
				}
				for _, want := range []string{
					`data-testid="` + prefix + `receipt-caller">` + caller.receipt + `</p>`,
					`data-testid="` + prefix + `receipt-body">Literal native caller payload.</p>`,
				} {
					if !callerMarkupContains(rec.Body.String(), want) {
						t.Errorf("missing literal %q in returned document", want)
					}
				}
				if keyed {
					if !callerMarkupContains(rec.Body.String(), `data-testid="keyed-receipt-key">carried key: k1</p>`) {
						t.Error("key was lost")
					}
					if strings.Contains(rec.Body.String(), `data-testid="receipt-caller"`) {
						t.Error("keyed result leaked into bare form")
					}
				}
			})
		}
	}
}

func TestNativeTypedCallerKeyInputPrecedence(t *testing.T) {
	h := handler
	id := ""
	for _, remote := range generated.Remotes() {
		if remote.Name() == "sendMessage" {
			id = remote.ID()
		}
	}
	if id == "" {
		t.Fatal("sendMessage is not registered")
	}
	fields := url.Values{"from/" + id: {"Key precedence"}, "email/" + id: {"key@example.test"}, "body/" + id: {"Literal keyed input wins."}, "id/" + id: {"submitted-id"}}
	req := httptest.NewRequest("POST", "/typed-load/42?"+url.Values{"/remote": {id + `/"k1"`}}.Encode(), strings.NewReader(fields.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Origin", origin)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	for _, want := range []string{`data-testid="keyed-receipt-key">carried key: submitted-id</p>`, `data-testid="keyed-receipt-caller">number:params.NumberParam_OrderNumber:{42}</p>`} {
		if rec.Code != 200 || !callerMarkupContains(rec.Body.String(), want) {
			t.Fatalf("missing %q: %d %s", want, rec.Code, rec.Body.String())
		}
	}
}

// Dev adds scoped CSS classes; the receipt text remains an exact literal.
func callerMarkupContains(body, literal string) bool {
	pattern := strings.Replace(regexp.QuoteMeta(literal), `">`, `"[^>]*>`, 1)
	return regexp.MustCompile(pattern).MatchString(body)
}
