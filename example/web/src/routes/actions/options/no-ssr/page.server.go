package nossr

import (
	"context"

	"github.com/tylergannon/skgo"
	actions "github.com/tylergannon/skgo/example/internal/skgo/links/onzggl3sn52xizltf5qwg5djn5xhg"
)

func pageLoad(ctx context.Context) (actions.PageData, error) { return actions.OptionLoad(ctx) }
func save(ctx context.Context) (actions.SaveResult, error) { return actions.OptionSave(ctx) }
func signIn(ctx context.Context) error { return actions.OptionSignIn(ctx) }
func forbidden(ctx context.Context) error { return actions.OptionForbidden(ctx) }
func unavailable(ctx context.Context) error { return actions.OptionUnavailable(ctx) }

var _ = skgo.Load(pageLoad)
var _ = skgo.ActionWithFailure(save, actions.ValidationFailure{})
var _ = skgo.ActionNoData(signIn)
var _ = skgo.ActionNoData(forbidden)
var _ = skgo.ActionNoData(unavailable)
