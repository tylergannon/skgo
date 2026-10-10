//go:build !darwin && !linux

package plugininstall

import (
	"context"
	"fmt"
)

func lock(context.Context, string) (func(), error) {
	return nil, fmt.Errorf("source plugins require cgo-enabled macOS or Linux")
}
