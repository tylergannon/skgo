// Package toolchain defines the qualified toolchain for this skgo release:
// the Kit, Svelte and Go versions the module is built and tested against, the
// Node range, and the frontend tool environment. VitePlus is the primary
// frontend pin; PNPM is its qualified companion here, since upstream vp also
// allows project/environment package-manager selection.
//
// Every other place in the module that needs one of these values reads it
// from here, and the tests in this package pin each value to the file that
// actually enforces it (go.mod, the example application's package.json and
// lockfile), and the new-application README quotes them. The project site
// (tylergannon/skgo-project, cmd/docgen) reads these constants by name from
// this file, so renaming or removing one breaks its build unless the old name
// is accepted there first.
package toolchain

// Kit is the @sveltejs/kit version skgo mirrors and patches.
const Kit = "3.1.0"

// Svelte is the svelte version the example application and new applications use.
const Svelte = "5.57.2"

// Go is the Go release the module declares and native development requires.
const Go = "1.27.2"

// Node is the range of Node releases the frontend tool environment accepts:
// the engines range the VitePlus release declares. It is narrower than Kit's
// own floor, so it is the range a developer must satisfy.
const Node = "^22.18.0 || ^24.11.0 || >=26.0.0"

// VitePlus is the qualified vp release.
const VitePlus = "1.3.0"

// PNPM is the package manager vp resolves for this release.
const PNPM = "12.14.0"
