# @skgo/sv

The native Svelte CLI add-on used by `skgo new`.

VitePlus delegates SvelteKit project creation to `sv`, which runs this add-on.
The add-on is the sole owner of skgo's frontend integration: it installs the
exact `@skgo/sveltekit-adapter` version selected by the Go command, configures
SvelteKit remote functions, and applies the chosen `minimal` or `examples`
starting point. Vitest and Storybook still come through their own upstream
setup paths.

The published add-on bundles its build-only `@sveltejs/sv-utils` code. Normal
users of `@skgo/sveltekit-adapter` therefore download only the runtime adapter,
not this installer bundle.

This package is invoked by `skgo new`; it is not a runtime dependency of a
generated application.

## License

[MIT](LICENSE)
