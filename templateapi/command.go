package templateapi

import (
	"context"
	"io"
)

// CommandPlugin contributes commands under skgo plugin <descriptor ID>.
// The host selects the provider and passes all remaining arguments unchanged.
// Return an error to fail the process; honor context cancellation and write to
// the supplied streams, never call os.Exit. Parse flags and print help here.
type CommandPlugin interface {
	Plugin
	RunCommand(context.Context, CommandRequest) error
}

// CommandRequest contains the invoking application and process resources.
// Host is the absolute executing CLI, suitable for explicit child operations.
// It is not a promise that another executable with the same version can load
// the same Go plugin. Project paths are absolute. Args excludes the plugin ID.
type CommandRequest struct {
	Project        Project
	Args           []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Host           string
}
