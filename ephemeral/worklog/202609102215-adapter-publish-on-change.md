decision: a release publishes @skgo/sveltekit-adapter only when `go run ./cmd/skgo-adapter-changed` finds the packed tree (stamped with the registry's version) differs from the registry's newest tarball. The Go tag is still cut every release. skgo vX pairs with `@skgo/sveltekit-adapter@<=X` (adapter.RegistrySpec); the fingerprint gate at startup is unchanged and still decides.

invariant: "newest at or below X carries X's adapter" holds only because every package change is published at the release that includes it. Never skip a publish for any reason other than byte-identical tarballs, and never publish a version out of order.

trap: pnpm's minimumReleaseAge turns a range into a silent fallback — `<=0.3.3` installed 0.3.2 an hour after 0.3.3 shipped (an exact spec refuses instead). The scaffold's pnpm-workspace.yaml therefore excludes '@skgo/sveltekit-adapter'; apps not made by `skgo new` need the same line.

trap: the README used to be stamped with the version at publish (`<matching-skgo-version>`), which would have made every comparison "changed". It no longer carries a version.
