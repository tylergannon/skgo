package validation

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/tylergannon/skgo"
)

var bodyCalls atomic.Int64

type CatalogInput struct {
	Name string `json:"name"`
}

func validateCatalog(ctx context.Context, input CatalogInput) (string, error) {
	calls := bodyCalls.Add(1)
	return fmt.Sprintf("Go accepted %s|body-calls=%d", input.Name, calls), nil
}

var _ = skgo.Query(validateCatalog)

func validationBodyCalls(ctx context.Context) (int64, error) { return bodyCalls.Load(), nil }

var _ = skgo.Query(validationBodyCalls)
