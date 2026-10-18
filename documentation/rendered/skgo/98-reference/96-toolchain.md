---
title: "Toolchain"
summary: "The Kit, Svelte, Go, Node, VitePlus and pnpm versions this release of SKGo is built and tested with."
---

Each release of SKGo is built, tested and measured against one version of
everything it depends on. `skgo new` scaffolds these versions, `skgo update`
brings an application to them, and `skgo kit-patch --check` verifies the Kit
pin. The values below are rendered from the release's own pins, so this page
and the binary agree.

| Dependency | Version | Where it is enforced |
|---|---|---|
| `@sveltejs/kit` | 3.1.0 | the application's `package.json`; `skgo kit-patch` refuses any other |
| `svelte` | 5.57.2 | the application's `package.json` |
| Go | 1.27.2 | `go.mod`; `skgo dev` requires exactly this release for native development |
| Node | 22.17 or newer | the frontend tool environment |
| VitePlus (`vp`) | 1.3.0 | the application's `package.json`, as the `vite` dependency |
| pnpm | 12.14.0 | resolved by `vp`; never installed or selected by hand |

VitePlus owns the frontend tool environment. Use `vp install`, `vp build`,
`vp run`, `vp exec` and `vp dlx` from the application's `web` directory. Do
not invoke `pnpm` directly or select its version with another tool; a recipe
that does is a defect.

The same values are published next to the rendered documentation as
`versions.json`, which is how the project site labels the release it
documents.
