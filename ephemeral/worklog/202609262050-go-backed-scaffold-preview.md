# Go-backed scaffold preview

decision: Kit's pinned Vite preview imports Kit's Node server and therefore invokes skgo's intentionally throwing JavaScript route stubs; the generated `pnpm preview` must delegate to the existing root `just serve` recipe so it builds and starts the Go binary.
friction: `skgo new` in CI reaches VitePlus install with sv's pre-add-on lockfile after sv and Storybook mutate package.json, and VitePlus freezes that install by default -> pass `--no-frozen-lockfile` specifically on the generator's post-add-on install.
friction: an isolated skgo worktree lacks example frontend dependencies and built assets, so `just test` fails example contracts before exercising product behavior -> run the repository's `just install` and `just build` setup first.
