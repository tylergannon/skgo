# docs: Core concepts section (M2 of the documentation product plan)

Branch `claude/docs-core-concepts`, stacked on `claude/documentation-pipeline`
(skgo PR #368). Plan: skgo-project `ephemeral/plans/2026-10-17-docs-product.md`.

decision: every Go sample on the eight pages is an include from `example/`;
  31 `// docs:` regions were added to existing example files and no example
  code was changed. Regions sit around whole functions or declarations so a
  later edit to the example cannot silently truncate a sample.
decision: the environment-variables page is the one page whose Go fences are
  literal (no `/// file:` label), because the example app declares no
  `skgo.Environment`; adding `example/web/src/env.go` changes the example's
  build inputs and e2e, which is not a docs change -> follow-up: add an
  environment fixture to the example and turn those two fences into includes.
decision: page options have no Go spelling; the page says so and documents
  what each Kit option means when the server is Go, with the prerender,
  ssr=false and csr=false example routes as its includes.
friction: `go test ./...` in `example/` fails in `example/web`
  (TestBuildEmbedsImmutableAssets) and `example/native/host` without a
  `web/build` from `vp build`; this machine has vp 1.0.0 and the pins want
  %vp%, so the Go-only packages were run instead and CI is the gate for the
  rest -> local only.
friction: gofmt moved two region markers that the insertion script wrote at
  column 0 inside a function body; markers inside a body must be indented
  with the code, and the include strips the common indent anyway.
