package templates

import (
	"context"
	"fmt"
	"github.com/tylergannon/skgo/templateapi"
	"io"
)

// CommandHelp enumerates contributions without executing them.
func (r *Registry) CommandHelp(out io.Writer) error {
	for _, e := range r.Entries {
		if e.Descriptor.Command != "" {
			fmt.Fprintf(out, "  %s: %s\n", e.Descriptor.ID, e.Descriptor.Command)
		}
	}
	return nil
}

// RunCommand forwards one namespace's arguments and process resources unchanged.
func (r *Registry) RunCommand(ctx context.Context, id string, req templateapi.CommandRequest) (err error) {
	var providers []Entry
	for _, e := range r.Entries {
		if e.Descriptor.ID == id && e.Descriptor.Command != "" {
			providers = append(providers, e)
		}
	}
	if err := selection("plugin command", id, providers); err != nil {
		return err
	}
	defer func() {
		if v := recover(); v != nil {
			err = fmt.Errorf("plugin command %s panicked: %v", id, v)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	return providers[0].Plugin.(templateapi.CommandPlugin).RunCommand(ctx, req)
}
