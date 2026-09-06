// Package web embeds the SvelteKit build produced by the skgo adapter.
//
// A fresh checkout does not compile until `vp build` has run in this
// directory: a missing frontend should fail loudly at build time rather than
// produce a binary that serves nothing.
package web

import "embed"

// Build holds the adapter output rooted at build/. The `all:` prefix is
// mandatory — without it Go silently omits `_app/`, because its default
// patterns skip names starting with `_` or `.`.
//
//go:embed all:build
var Build embed.FS
