package serverhooks

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/skgo/params"
	"io"
	"net/http"
	"os"
	"strings"
)

var Handle = params.Middleware(func(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (response *http.Response, err error) {
	defer func() {
		if err != nil {
			fmt.Fprintln(os.Stderr, "lifecycle fixture hook:", err)
		}
	}()
	event.Locals.Calls = 10
	event.Locals.Value = event.URL().Path
	if strings.HasPrefix(event.URL().Path, "/lifecycle/") && !event.IsSubRequest() {
		if err := event.SetCookie("erase", "original", skgo.CookieOptions{Path: "/"}); err != nil {
			return nil, err
		}
	}
	response, err = resolve(ctx, event)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(event.URL().Path, "/lifecycle/") && !event.IsSubRequest() {
		if response.StatusCode != 200 || !strings.Contains(response.Header.Get("Content-Type"), "text/html") {
			return nil, fmt.Errorf("not Kit's rendered response: %d %v", response.StatusCode, response.Header)
		}
		if event.Locals.Calls != 13 {
			return nil, fmt.Errorf("hook ran again or callbacks lost locals: %d", event.Locals.Calls)
		}
		flavor, _ := event.Cookie("flavor")
		_, hidden := event.Cookie("hidden")
		_, foreign := event.Cookie("foreign")
		_, erase := event.Cookie("erase")
		if flavor != "scoped%20raw" || hidden || foreign || erase {
			return nil, fmt.Errorf("after-hook cookie synchronization: flavor=%q hidden=%v foreign=%v erase=%v", flavor, hidden, foreign, erase)
		}
		request, err := http.NewRequestWithContext(ctx, "GET", "/lifecycle-cookies/verify", nil)
		if err != nil {
			return nil, err
		}
		fetched, err := event.Fetch(ctx, request)
		if err != nil {
			return nil, err
		}
		verified, err := io.ReadAll(fetched.Body)
		fetched.Body.Close()
		if err != nil || fetched.StatusCode != 200 || string(verified) != "cookie-forwarding:root" {
			return nil, fmt.Errorf("after-hook cookie forwarding: %d %s %v", fetched.StatusCode, verified, err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			return nil, err
		}
		if !strings.Contains(string(body), "layout-11") || !strings.Contains(string(body), "page-12") || !strings.Contains(string(body), "remote-13") {
			return nil, fmt.Errorf("after hook did not receive real rendered values: %s", body)
		}
		response.Body = io.NopCloser(strings.NewReader(string(body) + "<!-- Go after hook: " + event.Locals.Value + " cookies:scoped%20raw deleted cookie-forwarding:root -->"))
		response.Header.Del("Content-Length")
	}
	return response, nil
})
