// Package web embeds the build written by the skgo SvelteKit adapter.
package web

import "embed"

//go:embed all:build
var Build embed.FS
