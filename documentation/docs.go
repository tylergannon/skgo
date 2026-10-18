// Package skgodocs embeds skgo's rendered documentation: the tree under
// rendered/, produced from the sources under docs/ by cmd/skgo-docs. The
// project site copies the same tree at the tag it pins; `skgo` itself can
// print a page offline from this copy.
package skgodocs

import (
	"embed"
	"encoding/json"
	"io/fs"

	"github.com/tylergannon/skgo/internal/toolchain"
)

//go:embed rendered
var embedded embed.FS

// Rendered is the rendered tree, rooted at the directory that holds
// versions.json and the division directories.
func Rendered() fs.FS {
	sub, err := fs.Sub(embedded, "rendered")
	if err != nil {
		panic(err)
	}
	return sub
}

// Versions reads the pins the tree was rendered with.
func Versions() (toolchain.Pins, error) {
	var pins toolchain.Pins
	data, err := fs.ReadFile(Rendered(), "versions.json")
	if err != nil {
		return pins, err
	}
	err = json.Unmarshal(data, &pins)
	return pins, err
}
