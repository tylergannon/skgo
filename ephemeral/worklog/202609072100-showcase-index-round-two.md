# Showcase index, round two — three traps

## `pnpm test -- <name>` does not filter. It runs everything.

The index in `ephemeral/index/README.md` says one feature only, while
iterating, is `mise x -- pnpm test -- live`. It is not: `test` is
`bddgen && playwright test`, and pnpm's `--` forwarding does not reach the
second command, so the whole suite runs. Two and a half minutes a go, times
however many iterations. What works:

```
cd example/e2e && mise x -- pnpm exec bddgen
BASE_URL=$ORIGIN EXPECTED_MODE=prod SKGO_LOG=$LOG \
  mise x -- pnpm exec playwright test --project=chromium --grep "<scenario or feature title>"
```

`--grep` matches the title path, so a feature's name selects its whole file.
`bddgen` has to run separately or playwright reuses the last compiled
`.features-gen/`.

## AfterStep screenshots are never cleaned, and stale ones read as fresh

`example/e2e/screenshots/<mode>/<feature>/<Scenario or Example-N>/NN-<step>.png`
is written per step, and nothing removes what a previous run left. Insert a row
into a Scenario Outline and `Example-6/` then holds both
`04-Then-I-see-Overview.png` from the run before and
`04-Then-I-see-A-pair-of-todos.png` from this one — the same step index, two
different pages, no way to tell which run each came from. `rm -rf
example/e2e/screenshots ephemeral/screenshots` before any run whose frames you
intend to look at.

## An index entry is a stronger claim than a scenario, and it found a bug

Writing "what to look for" beside a link forces the promise to hold on the path
the link takes — a client-side navigation — and not merely on a document. The
entry for `/error/unexpected` was drafted as "the app's own handleError hook's
words and a support id". A document has those. The `__data.json` a click fetches
does not: it carries kit's default `{"status":500,"message":"Internal Error"}`,
because skgo runs the hook only on the render path. Same page, two different
answers depending on how the visitor arrived. That is #82, and kit does run
`handle_error_and_jsonify` on both wires (`runtime/server/data/index.js`:101
and :136).

Two comments on main already record the deferral (`data.go`'s `dataNode.raw`,
`document.go`'s `SSR.handleError`), so this was known and scoped out — but
nothing had ever *shown* the divergence, because every scenario about the hook
opens the page as a document. The index entry was what made it visible.
