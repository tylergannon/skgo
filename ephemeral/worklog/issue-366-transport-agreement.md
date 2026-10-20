# Issue 366 — Go/Kit transport agreement

- The root checkout's `example/web/node_modules` held Kit 3.0.0 on 2026-10-19
  while the pin is 3.1.0; `vp install` in the worktree before mapping Kit.
- Kit's `Transport`/`Transporter` types are exported from `@sveltejs/kit/hooks`,
  not `@sveltejs/kit`.
- Kit routes client snapshots and history state through `transport` too, so a
  TypeScript-only transport key is legitimate in Kit. An exact-keys contract
  would forbid something Kit permits.
- Measured with pinned tsc 6: `{...} satisfies { Money: Transporter<Money, {cents:number}> }`
  rejects a renamed, missing or extra key and payload drift (encode and decode
  both), with errors in the literal; Kit's widened `: Transport` annotation is
  rejected when assigned to the exact type ("Property 'Money' is missing").
- `skgo build` does not type-check; `skgo check` (svelte-check), the editor
  and CI's example job do.
- `KitTransport & { Money: Transporter<Money, P> }` still gives `encode`/`decode`
  contextual types from the Go entry (svelte-check 0 errors, mutations caught);
  the intersection does not widen `data` to `any`. A missing key is reported at
  the `satisfies` keyword, not at the misspelled key.
- polytype v1.10.0 follows encoding/json dominance (typegrammar.go
  `dominantFieldCandidates`) and drops untagged same-depth ties silently; it
  refuses bare-pointer embeds and `omitempty`/`omitzero` on non-Optional fields.
- The app formatter is `vp fmt` (oxfmt, width 100), not prettier; `prettier
  --check` disagreeing with generated files is not a defect.
- Release tooling maps `!` or a `BREAKING CHANGE:` footer line to a major bump,
  which is v1.0.0 from v0.41.x. Never put either in a squash message without the
  user's explicit decision; describe migrations in prose.
- The Mac data volume hit 100% (566 MB free) on 2026-10-19 mid-task; go
  generate failed with ENOSPC in TMPDIR. Orphaned go-build/skgo-gen-test temp
  dirs older than a day were ~600 MB.
