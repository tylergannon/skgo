//go:build !cgo || (!darwin && !linux)

package templates

import (
	"fmt"
	"github.com/tylergannon/skgo/templateapi"
)

func open(path string) (templateapi.Plugin, templateapi.Descriptor, error) {
	return nil, templateapi.Descriptor{}, fmt.Errorf("native plugins require a cgo-enabled skgo on macOS or Linux")
}
