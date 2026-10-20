//go:build skgo_development

package ssr

import (
	"context"
	"errors"
)

// Engine has no runtime in a development host. skgo dev builds its host with
// the skgo_development tag because Vite renders development pages, which keeps
// goja out of the host binary and its link.
type Engine struct{}

var errDevelopment = errors.New("skgo: skgo dev renders pages with Vite; create the SSR with development.NewSSR from NewDevelopmentHandler")

// New refuses: a development host never renders.
func New(name string, source []byte, size int, console Console, environment ...[]byte) (*Engine, error) {
	return nil, errDevelopment
}

// Render is unreachable, because New never returns an Engine.
func (e *Engine) Render(ctx context.Context, routeID string, request []byte, hosts Hosts) (Result, []Call, error) {
	return Result{}, nil, errDevelopment
}
