package skgo

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/tylergannon/skgo/internal/ssr"
)

// boundaryError receives Kit's classified error from its renderer. Only an
// unknown error's diagnostic crosses as Cause; it is never a public fallback.
// HandledHttpError and redirects are resolved in Kit before this callback.
func (s *SSR) boundaryError(ctx context.Context, routeID string, payload []byte) ([]byte, error) {
	var input struct {
		Kind  string    `json:"kind"`
		Body  ssr.Error `json:"body"`
		Cause string    `json:"cause"`
	}
	if err := json.Unmarshal(payload, &input); err != nil {
		return nil, err
	}
	fallback := &HTTPError{Status: input.Body.Status, Message: input.Body.Message}
	var raw error = fallback
	if input.Kind == "unknown" {
		fallback = &HTTPError{Status: 500, Message: "Internal Error"}
		raw = errors.New(input.Cause)
		input.Body.Extra = nil
	}
	hook := s.loads.cfg.HandleError
	preserveBody := hook == nil
	if hook != nil {
		original := hook
		hook = func(ctx context.Context, caught CaughtError) map[string]any {
			body := original(ctx, caught)
			preserveBody = true
			return body
		}
	}
	handled := handleErrorAndJSONify(ctx, routeID, hook, fallback, raw, s.report)
	// HttpError.body can contain app fields before the hook runs. Kit merges
	// partial overrides over that entire body, not just its status/message.
	// A failed hook uses Kit's status/Internal Error fallback, which discards
	// the original body fields as well as any partial hook result.
	if preserveBody && input.Body.Extra != nil {
		for key, value := range handled.Extra {
			input.Body.Extra[key] = value
		}
		handled.Extra = input.Body.Extra
	}
	return json.Marshal(handled)
}
