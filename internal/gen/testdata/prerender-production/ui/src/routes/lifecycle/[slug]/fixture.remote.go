package lifecyclepage

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo"
	"github.com/tylergannon/skgo/example/internal/app"
)

func value(ctx context.Context, _ string) (string, error) {
	locals := app.LocalsFrom(ctx)
	locals.Calls++
	event := skgo.EventFrom(ctx)
	flavor, _ := event.Cookie("flavor")
	_, hidden := event.Cookie("hidden")
	_, foreign := event.Cookie("foreign")
	_, erase := event.Cookie("erase")
	if flavor != "scoped%20raw" || hidden || foreign || erase {
		return "", fmt.Errorf("remote cookie synchronization: flavor=%q hidden=%v foreign=%v erase=%v", flavor, hidden, foreign, erase)
	}
	return fmt.Sprintf("%s:remote-%d cookies:scoped%%20raw deleted", locals.Value, locals.Calls), nil
}

var _ = skgo.Prerender(value)
