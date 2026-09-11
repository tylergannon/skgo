decision: a release publishes @skgo/sveltekit-adapter only when `go run ./cmd/skgo-adapter-changed` finds the files the tree would publish differ from the registry's newest tarball (fetched over HTTP, integrity-checked; package.json compared as JSON minus `version`). The Go tag is still cut every release. skgo vX pairs with `@skgo/sveltekit-adapter@<=X` (adapter.RegistrySpec); the fingerprint gate at startup is unchanged and still decides.

correction: the first cut shelled out to `npm view`/`npm pack`/`npm pkg set`, which breaks the zero-npm rule. `pnpm pack` is no substitute — it writes package.json differently from the `npm publish` that made the published tarball, so every comparison would read "changed". The command now reads the registry itself and stamps the version with `-stamp`; `npm publish` is the one npm call left in the release, because trusted publishing is bound to it.

invariant: "newest at or below X carries X's adapter" holds only because every package change is published at the release that includes it. Never skip a publish for any reason other than identical files, and never publish a version out of order.

trap: pnpm's minimumReleaseAge turns a range into a silent fallback — `<=0.3.3` installed 0.3.2 an hour after 0.3.3 shipped (an exact spec refuses instead). The scaffold's pnpm-workspace.yaml therefore excludes '@skgo/sveltekit-adapter'; apps not made by `skgo new` need the same line.

trap: the README used to be stamped with the version at publish (`<matching-skgo-version>`), which would have made every comparison "changed". It no longer carries a version.
