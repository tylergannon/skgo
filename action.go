package skgo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Action publishes a named page form action beside a page load in page.server.go.
// The action reads its submission from EventFrom(ctx).Request(). Its result is
// a plain object, carried through the application's transport hook.
func Action[Out any](fn func(context.Context) (Out, error)) Marker { _ = fn; return Marker{} }

// DefaultAction publishes the unnamed action on a page with no named actions.
func DefaultAction[Out any](fn func(context.Context) (Out, error)) Marker { _ = fn; return Marker{} }

// ActionWithFailure declares the typed data returned by Fail. The handler is
// still implemented only in Go; the second argument supplies its failure type
// to the generated Kit action export.
func ActionWithFailure[Out, Failure any](fn func(context.Context) (Out, error), failure Failure) Marker {
	_, _ = fn, failure
	return Marker{}
}

// ActionNoData declares an action whose successful result has no form data.
func ActionNoData(fn func(context.Context) error) Marker { _ = fn; return Marker{} }

type actionFailure struct {
	status int
	data   any
}

func (f *actionFailure) Error() string { return fmt.Sprintf("action failed with status %d", f.status) }

// Fail returns a Kit validation failure rather than an error page. Its data
// is retained in the form channel for native and enhanced submissions.
func Fail[Data any](status int, data Data) error {
	if status < 400 || status > 599 {
		panic("skgo: action failure status must be between 400 and 599")
	}
	return &actionFailure{status: status, data: data}
}

// PageAction is a generated Go action registration.
type PageAction struct {
	module string
	name   string
	run    func(context.Context) (any, error)
}

type ActionSpec struct {
	Module string
	Name   string
	Run    func(context.Context) (any, error)
}

func NewPageAction(spec ActionSpec) *PageAction {
	if spec.Run == nil {
		panic("skgo: page action has no Go handler")
	}
	return &PageAction{module: spec.Module, name: spec.Name, run: spec.Run}
}

type Actions struct {
	byModule map[string]map[string]*PageAction
}

func NewActions(actions ...*PageAction) (*Actions, error) {
	out := &Actions{byModule: map[string]map[string]*PageAction{}}
	for _, action := range actions {
		if action == nil || action.module == "" || action.name == "" || action.run == nil {
			return nil, errors.New("skgo: incomplete page action")
		}
		if !strings.HasSuffix(action.module, "/+page.server.ts") {
			return nil, fmt.Errorf("skgo: action %s belongs in +page.server.ts", action.name)
		}
		if out.byModule[action.module] == nil {
			out.byModule[action.module] = map[string]*PageAction{}
		}
		if out.byModule[action.module][action.name] != nil {
			return nil, fmt.Errorf("skgo: duplicate action %s in %s", action.name, action.module)
		}
		if action.name == "default" && len(out.byModule[action.module]) != 0 ||
			action.name != "default" && out.byModule[action.module]["default"] != nil {
			return nil, fmt.Errorf("skgo: default action cannot be used with named actions in %s", action.module)
		}
		out.byModule[action.module][action.name] = action
	}
	return out, nil
}

func (a *Actions) lookup(module, name string) *PageAction {
	if a == nil {
		return nil
	}
	return a.byModule[module][name]
}

func isActionJSON(r *http.Request) bool {
	// Kit negotiates Accept before looking at x-sveltekit-action. A JSON POST
	// reaches the page action even without the action header.
	accept := r.Header.Get("Accept")
	if accept == "" {
		accept = "*/*"
	}
	return r.Method == http.MethodPost && negotiate(accept, "application/json", "text/html") == "application/json"
}
