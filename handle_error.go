// Server hook: the Go side of SvelteKit's `handleError`.
//
// Kit's `handle_error_and_jsonify` (runtime/server/errors.js) runs the app's
// `handleError` hook for every error that reaches its response policy —
// whatever kind it is, `error()` thrown on purpose or an ordinary bug — and
// merges what the hook returns over the error's own status and message. The
// hook is application code and application code is Go. This file shares that
// policy across documents, navigation data, remote-function responses and
// errors caught during component rendering.
package skgo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"

	"github.com/tylergannon/skgo/internal/ssr"
)

// CaughtError is the safe error and private diagnostic supplied to HandleError.
// It follows Kit's discriminated CaughtError while retaining validation
// decoder details privately in Issues.
type CaughtError struct {
	// Kind is "app", "unknown" or "validation".
	Kind string
	// Status and Message are the error's own fallback: what the visitor sees
	// if the hook returns nothing. For "app" this is the app's own status and
	// message; for "unknown" it is always 500 and "Internal Error".
	Status  int
	Message string
	// Err is the original error skgo caught, set only for kind "unknown" — an
	// "app" error's entire body already is Status and Message. A hook that
	// wants to log the real cause of an unknown failure, or classify it
	// further with errors.As, reads it from here; it is never shown to the
	// visitor unless the hook copies something from it into what it returns.
	Err error
	// Issues contains private decoder diagnostics only for kind "validation".
	Issues []ValidationIssue
}

// HandleError is skgo's mirror of kit's `handleError` hook
// (`exports/hooks/public.d.ts`, `HandleServerError`): the one place an app
// decides what a failed request exposes publicly, including rejected remote
// arguments. Validation refusals have safe status 400/message "Bad Request";
// decoder details are available only in Issues.
//
// It runs for errors from server loads, page rendering, actions, navigation data,
// server-route panics, remote functions, and their single-flight refreshes.
// Remote envelopes and live-query frames carry the same public fields as page
// errors. Redirects and already-handled errors bypass the hook. During production
// prerendering, recorded App.Error bodies are reused without running the hook
// again. Kit still invokes it for app-kind errors raised with Errorf.
//
// Returning nil keeps kit's own defaults exactly: an "app" error's message
// survives untouched, and an "unknown" error's stays the generic "Internal
// Error" a real one never leaks through. Returning a map adds or overrides
// fields on the JSON object the error page or remote caller sees —
// "status" and "message" included, if the hook chooses to replace them — the
// same partial-override rule kit's own type comment states: "the hook
// returns only the properties it wants to override; anything it omits ... is
// inherited from the caught error." A "status" entry must be an int and a
// "message" entry a string; anything else under those two keys is ignored,
// the same as a value kit's own type checker would reject.
//
// The hook must never panic — kit's own doc comment says so too. One that
// does is logged and treated as returning nothing, except that the message
// becomes "Internal Error" even for an "app" error: Kit's own catch does the
// same (`log_handle_error_hook_failure`), because a hook
// that failed to run is not a hook whose omissions can be trusted.
type HandleError func(ctx context.Context, caught CaughtError) map[string]any

// handleErrorAndJSONify is kit's `handle_error_and_jsonify`, narrowed to Go's
// synchronous hook: there is no async `handleError` here — a synchronous Go
// function cannot be one. A built-prerender error can reach this function as
// an already-handled body; that body takes kit's `HandledHttpError` fast path
// and is returned without running the hook again.
//
// fallback is the error already reduced to a status and a message, e.g. by
// asHTTPError; raw is the original error skgo caught, if one better than
// fallback is available, and may be nil when the caller has nothing beyond
// fallback itself (a framework failure skgo manufactured, such as a 404 for
// an unmatched route).
func (s *SSR) documentError(ctx context.Context, routeID string, fallback *HTTPError, raw error) *ssr.Error {
	return handleErrorAndJSONify(ctx, routeID, s.loads.cfg.HandleError, fallback, raw, s.report)
}

// dataError is the same handle_error_and_jsonify call on kit's data path.
// Keeping the implementation shared is load-bearing: kit's client consumes
// App.Error identically whether it came in a rendered document or a data node.
func (ls *Loads) dataError(ctx context.Context, routeID string, fallback *HTTPError, raw error) *ssr.Error {
	return handleErrorAndJSONify(ctx, routeID, ls.cfg.HandleError, fallback, raw, nil)
}

func handleErrorAndJSONify(ctx context.Context, routeID string, hook HandleError, fallback *HTTPError, raw error, report func(string, error)) *ssr.Error {
	var handled *ssr.HandledError
	if errors.As(raw, &handled) && handled.Body != nil {
		return handled.Body
	}
	if fallback == nil {
		fallback = &HTTPError{Status: http.StatusInternalServerError, Message: "Internal Error"}
	}

	if hook == nil {
		return &ssr.Error{Status: fallback.Status, Message: fallback.Message}
	}

	caught := CaughtError{Kind: "app", Status: fallback.Status, Message: fallback.Message}
	if validation, ok := validationCaught(raw); ok {
		return mergeCaughtError(fallback, runHandleError(ctx, routeID, hook, validation, report))
	}
	var httpErr *HTTPError
	if raw != nil && !errors.As(raw, &httpErr) {
		caught.Kind = "unknown"
		caught.Err = raw
	}

	return mergeCaughtError(fallback, runHandleError(ctx, routeID, hook, caught, report))
}

// runHandleError calls the hook and turns a panic into kit's own
// hook-failure fallback.
func runHandleError(ctx context.Context, routeID string, hook HandleError, caught CaughtError, report func(string, error)) (result map[string]any) {
	defer func() {
		if v := recover(); v != nil {
			err := fmt.Errorf("skgo: the handleError hook panicked: %v\n%s", v, debug.Stack())
			if report != nil {
				report(routeID, err)
			} else {
				log.Printf("skgo: route %s: %v", routeID, err)
			}
			// Kit's own catch: the status stays the fallback's, but the
			// message is forced back to "Internal Error" — a hook that just
			// panicked is not one the app can vouch for, app-kind fallback or
			// not.
			result = map[string]any{"message": "Internal Error"}
		}
	}()
	return hook(ctx, caught)
}

// mergeCaughtError applies kit's own override rule over fallback and returns
// everything the hook added beyond status and message as Extra.
func mergeCaughtError(fallback *HTTPError, hookReturn map[string]any) *ssr.Error {
	result := &ssr.Error{Status: fallback.Status, Message: fallback.Message}
	if len(hookReturn) == 0 {
		return result
	}
	for k, v := range hookReturn {
		switch k {
		case "status":
			if n, ok := v.(int); ok {
				result.Status = n
			}
		case "message":
			if m, ok := v.(string); ok {
				result.Message = m
			}
		default:
			if result.Extra == nil {
				result.Extra = map[string]any{}
			}
			result.Extra[k] = v
		}
	}
	return result
}

// hookContext derives the context a page's `handleError` hook runs with: a
// read-only event, on kit's own rule that the hook may inspect the request
// but not act on it — nothing it does is tracked either, because the hook is
// not a load.
func hookContext(r *http.Request, shared *loadRequest) context.Context {
	e := shared.event(0, nil)
	e.mutable = false
	e.load.uses.tracking = false
	return withEvent(r.Context(), e)
}
