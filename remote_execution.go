package skgo

import (
	"context"
	"fmt"

	"github.com/tylergannon/polytype/devalue"
	"github.com/tylergannon/skgo/internal/remotearg"
)

// remoteOutcome retains diagnostic detail for the private build boundary.
// Public handlers expose only the ordinary sanitized HTTP error.
type remoteOutcome struct {
	value         any
	err           error
	panicked      bool
	diagnostic    string
	argumentError bool
}

func (rs *Remotes) invoke(ctx context.Context, fn *Remote, call Call) (result remoteOutcome) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result.panicked = true
			result.diagnostic = fmt.Sprint(recovered)
			result.err = rs.recovered(fn, recovered, nil)
		}
	}()
	result.value, result.err = fn.call(ctx, call)
	if result.err != nil {
		result.diagnostic = result.err.Error()
	}
	return
}

func (rs *Remotes) invokePayload(ctx context.Context, fn *Remote, payload string) remoteOutcome {
	arg, present, err := remotearg.ParsePayloadWith(payload, rs.codecs())
	if err != nil {
		return remoteOutcome{err: Errorf(400, "Bad Request"), diagnostic: err.Error(), argumentError: true}
	}
	return rs.invoke(ctx, fn, rs.newCall(arg, present))
}

func encodeRemoteResult(transport Transport, data map[string]any) (string, error) {
	return devalue.StringifyWith(data, transport.reducers())
}
