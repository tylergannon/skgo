Failed prerenders now preserve cleanup ownership and distinguish application transform errors from broken bridge replies, so Kit can apply its page-error policy. Hash-router document requests receive the client shell, and `skgo add tailwindcss` composes reusable setup that excludes Go source without changing Vite's watcher behavior.

Generation failure and transitive-remote folding are qualified against the current implementation. The rebase preserves main's relative `_links` route tree, colocated documentation, and smaller test boundaries; it does not restore the removed build fixtures or rendered documentation.

Validation:

- The rebased production example builds and was inspected in Chrome: Home, Go-backed prerendered About, and filtered invoices rendered their supplied values.
- `just test -v`: 835 top-level passes, zero failures or skips across 34 tested packages.
- Focused Go tests exercise the real hash document handler, failed-generation publication/link preservation and recovery, actual emitted-chunk folding into goja, callback error classification, and native process draining. CI explicitly selects the tagged process/fold witnesses, including Darwin EPERM.
- Fresh Claude Opus integration review: PASS for all five goals after the rebase; no verified reproducible regression. The actual report is `ephemeral/issue-resolutions/rebase-opus-review.json`.
- Original real Kit build/browser captures, full issue bodies/comments, cache annotations, and actual Claude Opus reviews are retained under `ephemeral/issue-resolutions/`. The native EPERM reproduction demonstrates the historical owner failing and current owner draining; the lost original PID's exact state is not claimed as recovered.

Closes #250
Closes #276
Closes #253
Closes #350
Closes #116
Closes #245
