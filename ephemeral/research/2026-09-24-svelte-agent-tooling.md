# Svelte component tooling research — 2026-09-24

Three independent agents researched component linting, formatting, and checking.
Version observations are from live npm metadata on this date. No general fastest
reliable tool was demonstrated; coverage and maturity qualify any speed claim.

## Component linting

- ESLint 10.11.0, eslint-plugin-svelte 3.23.0, parser 1.8.1: established full
  component/template/runes/TS-aware lint and rule-specific autofixes. Preferred
  default; qualify exact installation with Kit's pinned dependencies rather
  than upgrading unrelated app packages.
- Oxlint 1.85.0: script-block coverage; its JS plugin API cannot host custom
  parsers/file formats, so it cannot replace eslint-plugin-svelte. Its unused
  variable rule deliberately skips Svelte. Optional ordinary JS/TS acceleration
  is useful only without overlapping duplicate rules.
- Biome 2.5.14: real whole-file Svelte support, but experimental opt-in. Five
  Svelte-domain rules are nursery; the recommended domain enables none of those
  five. General JS/HTML rules provide additional coverage.
- @rsvelte/lint 0.12.4: strongest native challenger, standalone fixes and Svelte
  AST diagnostics; upstream oracle has exclusions despite the all-80-rules claim.
  @rsvelte/oxlint-plugin 0.2.5 skips scriptless components, has approximate markup
  positions, no fixes and limited rule enabling. Neither is the sprint default.
- Kit 3 Vite config support is documented through @sveltejs/load-config.

Sources:
- https://sveltejs.github.io/eslint-plugin-svelte/rules/
- https://sveltejs.github.io/eslint-plugin-svelte/user-guide/#svelte-config-inside-vite-config-js
- https://oxc.rs/docs/guide/usage/linter
- https://oxc.rs/docs/guide/usage/linter/js-plugins
- https://oxc.rs/docs/guide/usage/linter/rules/eslint/no-unused-vars
- https://biomejs.dev/internals/language-support/
- https://biomejs.dev/linter/domains/#svelte
- https://github.com/baseballyama/rsvelte/blob/main/apps/npm/lint/README.md
- https://github.com/baseballyama/rsvelte/blob/main/apps/npm/oxlint-plugin/README.md#limitations
- https://github.com/baseballyama/rsvelte/blob/bb3b94b2e6c23e3278282416a49d643b77a19e61/crates/rsvelte_lint/tests/eslint_plugin_oracle.rs

## Formatting

- Prettier 3.9.9 + prettier-plugin-svelte 4.1.1: established compatibility
  baseline; plugin v4 requires Svelte 5 and Node >=20.
- Oxfmt 0.70.0: Svelte uses bundled Prettier/plugin with native embedded JS/TS.
  Enable svelte:true and provide the app's Svelte compiler. The npm distribution
  is needed for Prettier-backed formats; standalone binary skips them. Good
  Vite+ integration, not demonstrated faster Svelte formatting. Qualify the
  actually installed Vite+ tool version, not these latest metadata versions.
- Biome 2.5.14: experimental full Svelte formatting, not the default.
- @rsvelte/fmt 0.7.24: early v0, promising native formatter; output/flags still
  stabilizing. @fuzdev/tsv 0.4.1: native/WASM, fixed non-configurable style,
  no JSX/SCSS/LESS/CSS Modules. Neither transparently preserves all project policy.
- dprint + markup_fmt 0.27.3 is another credible formatter composition, with
  additional TS and CSS/preprocessor plugin configuration to maintain.

TSV's September 23 vendor API benchmark on 945 shared real Svelte files reports
median sweeps: Prettier 5.47s, Oxfmt 5.54s, Biome WASM 1.16s, TSV native 0.07s.
It excludes six files rejected by Biome from all timing rows, produces different
styles, and does not measure independent CLI/editor latency. rsvelte was covered
for parsing but not timed. Do not treat this as equivalent-work performance proof.

Sources:
- https://github.com/sveltejs/prettier-plugin-svelte
- https://github.com/sveltejs/prettier-plugin-svelte/blob/main/CHANGELOG.md
- https://oxc.rs/docs/guide/usage/formatter/language-support
- https://oxc.rs/docs/guide/usage/formatter/config-file-reference
- https://github.com/baseballyama/rsvelte/blob/main/apps/npm/fmt/README.md
- https://raw.githubusercontent.com/fuzdev/tsv/main/README.md
- https://github.com/fuzdev/tsv/blob/main/docs/conformance_prettier.md
- https://github.com/fuzdev/tsv/blob/main/benches/js/results/report.node.md
- https://github.com/g-plane/markup_fmt

## Checker qualification

The checker agent made an isolated probe under
`ephemeral/research/svelte-check/fixture`, with tools installed separately under
`ephemeral/research/svelte-check/toolchain`. App dependencies were not changed.
Svelte 5.57.0, svelte-check 4.7.6, TypeScript 6.0.3 and 7.0.2 were used.

- Classic svelte-check detects the planted wrong component prop through a #lib
  import, accessibility warning and CSS warning with correct authored paths.
- --tsgo-experimental-api detects those three but reports bogus lowercased paths
  for some Svelte warnings on this Mac.
- --tsgo reports COMPLETED and exit 0 while missing the planted wrong prop type.
- @rsvelte/svelte-check 0.5.31 still fails #lib/Card.svelte resolution in this probe.
- Missing TSGo dependency can print an exception and 'svelte-check failed' but
  exit zero. Require successful completion evidence, not only process status;
  completion alone still does not establish diagnostic parity (plain TSGo above).
- Captured repeated-run tiny-fixture timings: classic 428–430ms,
  experimental 281–339ms, plain TSGo 281–339ms, rsvelte 87–92ms. Incomplete/incorrect modes
  do not constitute valid speed improvements.

Default to the classic official checker. Keep TS6 for Kit sync/build, which uses
its classic compiler APIs. Do not make TSGo or rsvelte default until the actual
project passes diagnostic, alias, and authored-path parity tests. Vite+ type
checking alone does not supply Svelte template diagnostics.

Sources:
- https://github.com/sveltejs/language-tools/blob/master/packages/svelte-check/README.md
- https://github.com/sveltejs/language-tools/blob/master/packages/svelte-check/src/index.ts
- https://viteplus.dev/guide/check
- https://github.com/baseballyama/rsvelte
- Pinned Kit: /Users/tyler/src/skgo/ephemeral/inspiration/reference/kit@3.0.0-next.27

## Independent fixtures worth retaining

Scriptless markup; runes; variable used only by template; accessibility; typed
component props through #lib; Kit 3 Vite config; whitespace-sensitive markup;
typed snippets/render tags; async expressions; attachments; configured style
languages; formatter second-pass stability; missing checker dependencies.

Probe source and captured output are retained locally at `ephemeral/research/svelte-check/probe.go`, `probe.log`, and `missing-native.log`. The alias fixture is under that directory's `fixture/`. Downloaded tooling is isolated under `toolchain/`; it is not an app dependency.
