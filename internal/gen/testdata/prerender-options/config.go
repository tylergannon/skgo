package app

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo"
	"io"
	"net/http"
	"os"
	"strings"
)

func init() {
	Config = skgo.ApplicationConfig{
		FilterSerializedResponseHeaders: func(name, value string) bool { return SerializedHeader(name, value) || name == "x-config-hook" },
		HandleFetch: func(ctx context.Context, request *http.Request, next skgo.Fetch) (*http.Response, error) {
			if request.URL.Path != "/shared-alias/nested" {
				return next(request)
			}
			if skgo.RequestLocals[Locals](ctx) == nil || !strings.HasPrefix(skgo.RequestLocals[Locals](ctx).Value, "/shared-fetch") {
				return nil, fmt.Errorf("fetch hook lost page locals")
			}
			rewritten := request.Clone(ctx)
			destination := *request.URL
			destination.Path = "/shared-api"
			if strings.HasSuffix(skgo.RequestLocals[Locals](ctx).Value, "-served") {
				destination.Path = "/shared-live-api"
			}
			rewritten.URL = &destination
			response, err := next(rewritten)
			if err != nil {
				return nil, err
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				return nil, err
			}
			response.Body = io.NopCloser(strings.NewReader("literal-hook-content-278|" + string(body)))
			response.Header.Del("Content-Length")
			response.Header.Set("X-Config-Hook", "literal-hook-header-278")
			return response, nil
		},
		HandleError: func(ctx context.Context, caught skgo.CaughtError) map[string]any {
			if !strings.HasPrefix(skgo.RequestLocals[Locals](ctx).Value, "/shared-error") && !strings.HasPrefix(skgo.RequestLocals[Locals](ctx).Value, "/shared-component") {
				if caught.Err != nil {
					fmt.Fprintln(os.Stderr, caught.Err)
				}
				return nil
			}
			if caught.Kind != "unknown" || caught.Err == nil || !strings.Contains(caught.Err.Error(), "literal-private-error-278") || skgo.RequestLocals[Locals](ctx).Calls != 10 {
				panic("error hook lost original cause")
			}
			skgo.RequestLocals[Locals](ctx).Calls++
			status := 500
			if strings.Contains(caught.Err.Error(), "literal-private-error-278-component") {
				status = 200
			}
			return map[string]any{"status": status, "message": "literal-safe-error-278", "supportId": "literal-support-278", "hookCalls": 11}
		},
	}
}
