package sharederror

import (
	"context"
	"errors"
	"github.com/tylergannon/skgo"
)

func failed(context.Context) (string, error) { return "", errors.New("literal-private-error-278") }

var _ = skgo.Prerender(failed)

func failedQuery(context.Context) (string, error) { return "", errors.New("literal-private-error-278") }

var _ = skgo.Query(failedQuery)
