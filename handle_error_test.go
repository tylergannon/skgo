package skgo

import (
	"context"
	"errors"
	"testing"

	"github.com/tylergannon/skgo/internal/ssr"
)

// hookSSR is a renderer with nothing but a handleError hook wired in — enough
// to exercise documentError, which reads only s.handleError and s.onError
// (through report, on a hook panic).
func hookSSR(hook HandleError) *SSR {
	return &SSR{handleError: hook}
}

func TestDocumentErrorWithNoHookKeepsTheFallbackExactly(t *testing.T) {
	s := hookSSR(nil)
	got := s.documentError(context.Background(), "/x", &HTTPError{Status: 418, Message: "This page is a teapot"}, nil)
	want := &ssr.Error{Status: 418, Message: "This page is a teapot"}
	if got.Status != want.Status || got.Message != want.Message || len(got.Extra) != 0 {
		t.Errorf("documentError with no hook = %+v, want %+v", got, want)
	}
}

// This is the mapped fact the issue's own premise got wrong: kit's
// `handle_error_and_jsonify` runs the hook for an error the app raised on
// purpose too, not only for one nobody expected
// (exports/hooks/public.d.ts: "runs for every error thrown ... except
// redirects" — no exception carved out for an `error()` kind). Only an
// already-handled error skips it, and nothing on skgo's render path produces
// one of those.
func TestDocumentErrorConsultsTheHookForAnAppRaisedErrorToo(t *testing.T) {
	var sawKind string
	hook := func(ctx context.Context, caught CaughtError) map[string]any {
		sawKind = caught.Kind
		return map[string]any{"supportId": "case-1121"}
	}
	s := hookSSR(hook)
	got := s.documentError(context.Background(), "/x", &HTTPError{Status: 418, Message: "This page is a teapot"}, nil)

	if sawKind != "app" {
		t.Errorf("hook saw kind %q, want %q", sawKind, "app")
	}
	// The app's own message survives untouched: the hook returned nothing for
	// "message", and kit's own rule is that an omitted property is inherited
	// from the caught error, not replaced by any default.
	if got.Message != "This page is a teapot" {
		t.Errorf("Message = %q, want the app's own message unchanged", got.Message)
	}
	if got.Status != 418 {
		t.Errorf("Status = %d, want 418", got.Status)
	}
	if got.Extra["supportId"] != "case-1121" {
		t.Errorf("Extra[supportId] = %v, want case-1121", got.Extra["supportId"])
	}
}

func TestDocumentErrorClassifiesAnOrdinaryGoErrorAsUnknown(t *testing.T) {
	var sawKind string
	var sawErr error
	raw := errors.New("the connection string is postgres://ada:hunter2@db")
	hook := func(ctx context.Context, caught CaughtError) map[string]any {
		sawKind = caught.Kind
		sawErr = caught.Err
		return map[string]any{"message": "Something went wrong on our end.", "supportId": "case-1121"}
	}
	s := hookSSR(hook)
	got := s.documentError(context.Background(), "/x", &HTTPError{Status: 500, Message: "Internal Error"}, raw)

	if sawKind != "unknown" {
		t.Errorf("hook saw kind %q, want %q", sawKind, "unknown")
	}
	if !errors.Is(sawErr, raw) {
		t.Errorf("hook saw Err %v, want the original error", sawErr)
	}
	if got.Message != "Something went wrong on our end." {
		t.Errorf("Message = %q, want the hook's own words", got.Message)
	}
	if got.Extra["supportId"] != "case-1121" {
		t.Errorf("Extra[supportId] = %v, want case-1121", got.Extra["supportId"])
	}
}

func TestDocumentErrorOverridesStatusToo(t *testing.T) {
	hook := func(ctx context.Context, caught CaughtError) map[string]any {
		return map[string]any{"status": 503}
	}
	s := hookSSR(hook)
	got := s.documentError(context.Background(), "/x", &HTTPError{Status: 500, Message: "Internal Error"}, errors.New("boom"))
	if got.Status != 503 {
		t.Errorf("Status = %d, want 503", got.Status)
	}
}

// Kit's own rule: a hook that panics is logged and treated as a fallback that
// forces the message back to "Internal Error" — even for an app-kind error,
// because a hook that just failed to run is not one whose omissions can be
// trusted. The status is left alone.
func TestDocumentErrorRecoversAPanickingHook(t *testing.T) {
	hook := func(ctx context.Context, caught CaughtError) map[string]any {
		panic("the hook itself has a bug")
	}
	s := hookSSR(hook)
	got := s.documentError(context.Background(), "/x", &HTTPError{Status: 418, Message: "This page is a teapot"}, nil)
	if got.Status != 418 {
		t.Errorf("Status = %d, want the fallback's own 418", got.Status)
	}
	if got.Message != "Internal Error" {
		t.Errorf("Message = %q, want \"Internal Error\" once the hook has panicked", got.Message)
	}
	if len(got.Extra) != 0 {
		t.Errorf("Extra = %v, want none: a panicking hook's return value is discarded", got.Extra)
	}
}
