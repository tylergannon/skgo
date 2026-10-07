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

var lifecycleHandle = params.Middleware(func(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (response *http.Response, err error) {
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

var Handle = params.Sequence(
	params.Middleware(func(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
		if event.URL().Path != "/options" && event.URL().Path != "/options-rejected" {
			return resolve(ctx, event)
		}
		return resolve(ctx, event, skgo.ResolveOptions{
			TransformPageChunk: func(ctx context.Context, html string, done bool) (string, error) {
				if !done || (event.URL().Path == "/options" && !event.Locals.AssetsReady) {
					return "", fmt.Errorf("transform lost load state")
				}
				return strings.ReplaceAll(html, "INNER_TOKEN", "inner-outer"), nil
			},
			FilterSerializedResponseHeaders: func(name, value string) bool {
				return name == "x-public" || (name == "x-late" && event.Locals.HeadersReady)
			},
			Preload: func(input skgo.PreloadInput) bool {
				return input.Type == "font" && input.Filename == "src/routes/options/fixture.woff2" && event.Locals.AssetsReady
			},
		})
	}),
	params.Middleware(func(ctx context.Context, event params.RequestEvent, resolve params.Resolve) (*http.Response, error) {
		if event.URL().Path != "/options" && event.URL().Path != "/options-rejected" {
			return resolve(ctx, event)
		}
		return resolve(ctx, event, skgo.ResolveOptions{
			TransformPageChunk: func(ctx context.Context, html string, done bool) (string, error) {
				return strings.ReplaceAll(html, "TRANSFORM_TOKEN", "INNER_TOKEN"), nil
			},
			FilterSerializedResponseHeaders: func(string, string) bool { panic("inner filter must not run") },
			Preload:                         func(skgo.PreloadInput) bool { panic("inner preload must not run") },
		})
	}), lifecycleHandle,
)
