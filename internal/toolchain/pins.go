// Package toolchain defines the qualified toolchain for this skgo release:
// the Kit, Svelte and Go versions the module is built and tested against, the
// Node floor, and the frontend tool environment. VitePlus is the primary
// frontend pin; PNPM is its qualified companion here, since upstream vp also
// allows project/environment package-manager selection.
//
// Every other place in the module that needs one of these values reads it
// from here, and the tests in this package pin each value to the file that
// actually enforces it (go.mod, the example application's package.json, the
// new-application README).
package toolchain

import (
	"bytes"
	_ "embed"
)

//go:embed pins.go
var source []byte

// MatchesSource checks that a generator and application use the same qualified pins.
func MatchesSource(data []byte) bool { return bytes.Equal(data, source) }

// Kit is the @sveltejs/kit version skgo mirrors and patches.
const Kit = "3.1.0"

// Svelte is the svelte version the example application and new applications use.
const Svelte = "5.57.2"

// Go is the Go release the module declares and native development requires.
const Go = "1.27.2"

// NodeMinimum is Kit's Node floor; Node is the full qualified frontend range.
const NodeMinimum = "22.17"

// Node is the Node range required by the qualified VitePlus release.
const Node = "^22.18.0 || ^24.11.0 || >=26.0.0"

// VitePlus is the qualified vp release.
const VitePlus = "1.3.0"

// PNPM is the package manager vp resolves for this release.
const PNPM = "12.14.0"

// Pins is every pinned version in one value, in the shape the rendered
// documentation publishes as versions.json.
type Pins struct {
	Kit         string `json:"kit"`
	Svelte      string `json:"svelte"`
	Go          string `json:"go"`
	NodeMinimum string `json:"node"`
	VitePlus    string `json:"vitePlus"`
	PNPM        string `json:"pnpm"`
}

// Current returns the pins of this release.
func Current() Pins {
	return Pins{Kit: Kit, Svelte: Svelte, Go: Go, NodeMinimum: NodeMinimum, VitePlus: VitePlus, PNPM: PNPM}
}
