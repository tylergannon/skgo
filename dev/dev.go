// Package dev owns a development session: Vite, generation, Go compilation and
// application replacement behind one stable listener. Generate runs serially
// in the supervisor process, so its closure may retain generator sessions.
package dev

import (
	"context"
	internal "github.com/tylergannon/skgo/internal/dev"
)

// Config selects the application's commands, generation callback and watch roots.
type Config = internal.Config

// WatchInputs is a retained generator's current source selection.
type WatchInputs = internal.WatchInputs

// Server owns the public listener and all application and Vite processes it starts.
type Server = internal.Server

// New prepares a session without starting its processes. Use Listen to obtain a
// dynamically assigned public address before Run starts generation.
func New(cfg Config) (*Server, error) { return internal.New(cfg) }

// Run starts a session and stops its children when ctx is canceled. Unexpected
// Vite termination ends the session; generation/build errors remain visible at
// the public address until a subsequent source edit fixes them.
func Run(ctx context.Context, cfg Config) error {
	server, err := New(cfg)
	if err != nil {
		return err
	}
	return server.Run(ctx)
}
