//go:build cgo && (darwin || linux)

package templates

import (
	"fmt"
	"plugin"
	"strings"

	"github.com/tylergannon/skgo/templateapi"
)

func open(path string) (p templateapi.Plugin, d templateapi.Descriptor, err error) {
	defer func() {
		if v := recover(); v != nil {
			p = nil
			err = fmt.Errorf("plugin initialization/description panicked: %v", v)
		}
	}()
	loaded, err := plugin.Open(path)
	if err != nil {
		if strings.Contains(err.Error(), "plugin already loaded") {
			return nil, d, fmt.Errorf("duplicate-source load: %w", err)
		}
		return nil, d, err
	}
	symbol, err := loaded.Lookup("SKGoPluginV1")
	if err != nil {
		return nil, d, err
	}
	factory, ok := symbol.(func() templateapi.Plugin)
	if !ok {
		return nil, d, fmt.Errorf("SKGoPluginV1 must be func() templateapi.Plugin")
	}
	p = factory()
	if p == nil {
		return nil, d, fmt.Errorf("SKGoPluginV1 returned nil")
	}
	return p, p.Describe(), nil
}
