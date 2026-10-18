# Documentation pipeline (skgo side of the docs product)

Plan: skgo-project `ephemeral/plans/2026-10-17-docs-product.md`, M0/M1.
Source tree `documentation/docs`, renderer and checker `documentation/render`,
committed rendered tree, `cmd/skgo-docs`, command table, exported pins.

decision: command flags are described as data in `internal/commands` and a
go/ast test in `cmd/skgo` keeps that data equal to the real flag sets, rather
than refactoring twelve inline flag sets.
decision: Node has no pin in code; `toolchain.NodeMinimum` is introduced with
the new-app README template as its witness.

friction: go/doc files every function returning a type under that type, so
Load, Query and GET all rendered under Marker -> functions returning an
empty struct are promoted to their own entries (render/godoc.go).
friction: go/doc/comment's Markdown printer escapes backquotes and emits
tab-indented code blocks -> tidyMarkdown restores inline code and fences
code as go.
decision: the reference frames link to Kit's guides where SKGo's own guide
page does not exist yet; the link checker refuses a link to a page that is
not in the tree, and that rule stays.
discovery: `native/` has no Go package (tests and Zig/Swift sources only), so
the plan's `skgo/native` reference page does not exist.
