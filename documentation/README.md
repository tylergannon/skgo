# Documentation

The source of skgo's documentation, rendered to `rendered/` and embedded in the
module. The project site (tylergannon/skgo-project) copies `rendered/` at the
skgo tag it pins and serves it under `/docs`; it renders and never authors.
This README is the authoring contract. `go test ./documentation/...` enforces
it by rendering sources in memory. Automatic generation and checks against the
committed snapshot are disconnected; source changes do not require updating
`rendered/`.

## Tree

```
docs/<division>/index.md                   title only
docs/<division>/<NN-section>/index.md      title only
docs/<division>/<NN-section>/<NN-slug>.md  a page
```

Divisions are `skgo`, `cli` and `ai`. Order comes from the two-digit prefixes
and the slug from the file name, as on svelte.dev; nothing in the frontmatter
repeats the path. A page's URL on the site is `/docs/<division>/<slug>`.

## Frontmatter

```yaml
---
title: Loading data
summary: Server loads in Go, their inputs and what reaches the page.
proof:
  - example/e2e/features/loads.feature
  - TestLoadParent
---
```

`title` and `summary` (under twenty words) are required on a page and only
`title` on an index. `proof` is optional and names feature files under
`example/e2e/features` or Go test function names anywhere in the module; a
name that resolves to nothing fails the check. Nothing else is allowed.

## Prose

Write the way svelte.dev writes: second person, present tense, one idea per
paragraph, the code before the explanation of the code. The product is
"SKGo" in prose and `skgo` in code and commands. The web framework is "Kit" or
"SvelteKit"; never "kit". Callouts are GitHub's:

```markdown
> [!NOTE] Kit's version of this page is [Loading data](https://svelte.dev/docs/kit/load).
```

Never write a pinned version as a literal. Write `%kit%`, `%svelte%`, `%go%`,
`%node%`, `%vp%`, `%pnpm%` or `%skgo%`; the renderer substitutes all but
`%skgo%`, which the site fills from the version it pins.

## Links

- Another page in the same division: `[Loading data](load)`, with an optional
  `#anchor`. The checker requires the page to exist and the anchor to be a
  heading on the rendered page.
- A page in another division, or anything the site serves: an absolute path,
  `[skgo generate](/docs/cli/generate)`.
- Kit's documentation: the full `https://svelte.dev/docs/kit/...` URL.

Anchors follow svelte.dev's rule: the heading text with spaces as `-`, case
kept, nested under its parent heading (`Universal-vs-server-Input`).

## Code

Go code the reader is told to write is always real code from `example/`,
pulled in by an include. The fence keeps its label and loses the include line:

````markdown
```go
/// file: src/routes/account/page.server.go
/// include: example/web/src/routes/account/page.server.go#parent
```
````

`#parent` names a region delimited in the source by comment lines
`// docs: parent` and `// docs: end parent`; omit it for the whole file. The
region's common indentation is removed. A `go` fence with a `/// file:` label
and no include fails the check. Any fence may instead be typed and literal:
shell sessions, Svelte fragments, JSON shapes, and "do not do this"
counterexamples.

The `/// file:` label is the path the reader sees, relative to the application
root the way Kit's docs write it (`src/routes/...`), not the path in this
repository.

## Reference pages

A reference page is a short frame followed by a placeholder:

```markdown
> PACKAGE: github.com/tylergannon/skgo
> SYMBOLS: Load Parent Deferred Async Resolved
```

The renderer replaces the two lines with, for each symbol in order, an `##`
heading, the declaration in a `go` fence, the doc comment as markdown, and the
type's constructors, methods and associated constants under `###` headings.
Omit `SYMBOLS:` to take the whole package. Every exported symbol of every
package named anywhere in the tree must appear on exactly one page, so a new
exported function fails the check until a page claims it.

A CLI reference page uses `> COMMAND: skgo generate` for the synopsis,
description and flag table of one command from `internal/commands`, and
`> COMMANDS:` for the table of all of them.

## Commands

```sh
just docs                           # validate sources without writing output
go run ./cmd/skgo-docs check         # the same checks, with file:line output
go test ./documentation/...          # contract, directives, ownership, renderer
```

`go run ./cmd/skgo-docs render` remains an explicit manual operation. Normal
generation and documentation checks leave the existing `rendered/` snapshot
unchanged.
