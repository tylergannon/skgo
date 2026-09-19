# @skgo/sv

The native Svelte CLI add-on used by `skgo new`.

VitePlus delegates SvelteKit project creation to `sv`, where the developer
makes `sv`'s own choices; `skgo new` then has `sv add` run this add-on on the
result. The add-on is the sole owner of skgo's frontend integration: it installs the
exact `@skgo/sveltekit-adapter` version selected by the Go command, configures
SvelteKit remote functions, and applies the `minimal` or `examples` starting
point. `examples` replaces `sv`'s demo template, a JavaScript server
application, and keeps what other add-ons put in its layout. Vitest and Storybook still come through their own upstream
setup paths.

The published add-on bundles its build-only `@sveltejs/sv-utils` code. Normal
users of `@skgo/sveltekit-adapter` therefore download only the runtime adapter,
not this installer bundle.

This package is invoked by `skgo new`; it is not a runtime dependency of a
generated application.

## First publication

The first `@skgo/sv` release is a manual bootstrap. Do not publish the checked-in
`0.0.0-dev` manifest and do not let the release workflow attempt to create the
package for the first time.

Publish before merging, from the candidate branch that introduces this
package. The release workflow runs on every push to `main`, refuses to create
`@skgo/sv` for the first time, and cuts no Go tag until the package exists, so
merging first leaves `main` with a failed release and a `skgo new` that cannot
select its add-on.

The version to publish is the exact one the pending merge will release, not
the latest existing tag: an add-on stamped with an older skgo version claims
compatibility with a release that predates it. The workflow derives that
version from the squash commit's Conventional Commit subject — the PR title —
applied to the latest `v*` tag (`fix` is a patch, `feat` a minor), so settle the
PR title first and do not change it, or let another release land, between
publishing and merging. If either happens, publish again at the new version
before merging.

From the root of the candidate branch's checkout, with that version as
`vX.Y.Z`:

```sh
go run ./cmd/skgo-adapter-changed -dir internal/sv -package @skgo/sv -stamp vX.Y.Z
pnpm --dir internal/sv install --frozen-lockfile
pnpm --dir internal/sv run build
mkdir -p /tmp/skgo-sv-first-release
pnpm --dir internal/sv pack --pack-destination /tmp/skgo-sv-first-release
```

Inspect the resulting tarball before publishing: its `package/package.json`
must name the real `X.Y.Z` version, and its file list must contain only the
package contract tested by `internal/sv/package_test.go`. Then publish that
exact tarball manually with `npm publish <tarball> --access public`.

Only after the manual publication succeeds, configure and verify npm trusted
publishing for `.github/workflows/release.yml`, and only then merge the
candidate branch. That merge and subsequent releases use the workflow. This
ordering makes the first published version participate in
`skgo new`'s exact, not-newer-than-skgo package pairing instead of leaving
`0.0.0-dev` as the selectable package.

## License

[MIT](LICENSE)
