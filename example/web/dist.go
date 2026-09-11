// Package web embeds the SvelteKit build produced by the skgo adapter.
//
// A fresh checkout embeds only build/.gitkeep so Go can compile before `vp
// build` has built the frontend. Starting that binary still fails loudly when
// it cannot read the adapter manifest; the placeholder is not a runnable
// frontend.
package web

import "embed"

// Build holds the adapter output rooted at build/. The `all:` prefix is
// mandatory — without it Go silently omits `_app/`, because its default
// patterns skip names starting with `_` or `.`.
//
//go:embed all:build
var Build embed.FS
