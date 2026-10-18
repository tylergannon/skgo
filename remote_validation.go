package skgo

import (
	"context"
	"errors"

	"github.com/tylergannon/skgo/internal/ssr"
)

// ValidationIssue is a private argument-decoder diagnostic supplied to
// HandleError for kind "validation". It reaches the client only if the hook
// deliberately includes it in its public result.
type ValidationIssue struct {
	Message string `json:"message"`
}

type argumentValidationError struct{ detail error }

func (e argumentValidationError) Error() string {
	if e.detail == nil {
		return "skgo: invalid remote argument"
	}
	return "skgo: invalid remote argument: " + e.detail.Error()
}
func (e argumentValidationError) Unwrap() error {
	return &HTTPError{Status: 400, Message: "Bad Request"}
}

// RejectArgument marks a generated strict decoder's schema refusal. Ordinary
// application BadRequest errors, malformed transport and form issues do not
// use this boundary. Generated registrations call it before the remote body.
func (Call) RejectArgument(detail error) error { return argumentValidationError{detail: detail} }

func validationCaught(err error) (CaughtError, bool) {
	var rejected argumentValidationError
	if !errors.As(err, &rejected) {
		return CaughtError{}, false
	}
	message := "Invalid remote argument"
	if rejected.detail != nil {
		message = rejected.detail.Error()
	}
	return CaughtError{Kind: "validation", Status: 400, Message: "Bad Request", Issues: []ValidationIssue{{Message: message}}}, true
}

func (rs *Remotes) remoteError(ctx context.Context, err error) *ssr.Error {
	fallback := asHTTPError(err)
	caught, validation := validationCaught(err)
	if !validation || rs.cfg.HandleError == nil {
		return &ssr.Error{Status: fallback.Status, Message: fallback.Message}
	}
	return mergeCaughtError(fallback, runHandleError(ctx, "", rs.cfg.HandleError, caught, nil))
}
