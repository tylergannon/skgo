# Native dev validation

- `kit-patch` defaulted to ambient pnpm, and its Go config fixtures did the same; this broke outside `vp exec` when ambient pnpm differed from the declared supported pin. Route the command and fixtures through VitePlus's managed package-manager environment; focused CLI and all kitpatch tests now pass.
- While adapter source is being edited in a shared worktree, `go run skgo dev` can embed one fingerprint then see a newer live linked adapter at startup. Start actual-app qualification after the adapter owner announces a stable checkpoint; do not suppress the identity check.
