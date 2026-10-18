package skgo

import (
	"context"
	"errors"
	"net/http"

	"github.com/tylergannon/skgo/internal/ssr"
)

// validationError is the schema-refusal branch of Kit's handle_error_and_jsonify.
// Malformed wire payloads and no-argument refusals are ordinary HTTP errors,
// not schema issues (runtime/app/server/remote/shared.js, create_validator).
func (rs *Remotes) validationError(ctx context.Context, err error) *ssr.Error {
	var rejected badRequest
	if !errors.As(err, &rejected) {
		return nil
	}
	if event := EventFrom(ctx); event != nil {
		readonly := *event
		readonly.mutable = false
		ctx = withEvent(ctx, &readonly)
	}
	return handleErrorAndJSONify(ctx, "", rs.cfg.HandleError, asHTTPError(err), err, nil)
}

func (rs *Remotes) remoteError(ctx context.Context, err error) *ssr.Error {
	if body := rs.validationError(ctx, err); body != nil {
		return body
	}
	fallback := asHTTPError(err)
	return &ssr.Error{Status: fallback.Status, Message: fallback.Message}
}

func (rs *Remotes) writeCallError(w http.ResponseWriter, ctx context.Context, err error) {
	h := rs.header(w)
	h.Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	writeJSON(w, remoteResponse{Type: "error", Error: rs.remoteError(ctx, err)})
}
