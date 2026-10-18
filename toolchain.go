package skgo

import (
	"github.com/tylergannon/skgo/internal/adapter"
	"github.com/tylergannon/skgo/internal/toolchain"
)

// ToolchainVersions describes the toolchain qualified by the adopted SKGo
// module. Go is the qualified pin, rather than the running binary's compiler;
// Node is the supported range, rather than the installed Node version.
type ToolchainVersions struct {
	SKGo     string
	Go       string
	Kit      string
	VitePlus string
	PNPM     string
	Node     string
}

// Toolchain returns the versions qualified by this application's SKGo module.
// SKGo comes from Go build information: it is the module version for a released
// dependency, or "devel" for a checkout, replaced module or unavailable build
// information. It never reports the application's own version as SKGo.
//
// skgo generate exports the same values as versions in src/skgo.versions.ts
// (or .js in a JavaScript application). Check that generated build input into
// source control and regenerate it when adopting a new SKGo release.
func Toolchain() ToolchainVersions {
	return ToolchainVersions{SKGo: adapter.Version(), Go: toolchain.Go, Kit: toolchain.Kit,
		VitePlus: toolchain.VitePlus, PNPM: toolchain.PNPM, Node: toolchain.Node}
}
