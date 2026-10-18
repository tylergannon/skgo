package traced

import (
	"context"

	"github.com/tylergannon/skgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

type Receipt struct {
	Kind string `json:"kind"`
}

func receipt(ctx context.Context) (Receipt, error) {
	skgo.EventFrom(ctx).Tracing.Current.SetAttributes(attribute.String("receipt", "remote"))
	_, span := otel.Tracer("myapp").Start(ctx, "receipt.build")
	defer span.End()
	return Receipt{Kind: "remote"}, nil
}

var _ = skgo.Query(receipt)
