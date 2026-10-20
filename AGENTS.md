# skgo

A Go server for SvelteKit. The frontend stays plain SvelteKit; Go owns the
process and answers every server endpoint: remote functions, server loads,
actions, hooks and `+server` routes. Kit's JavaScript for those is a generated
stub that throws. In production, kit's own renderer runs in-process on goja
(pure Go, no cgo) from a pool of runtimes, and no application I/O runs in
JavaScript. One binary; Node is build-time only. `skgo dev` uses native
Vite/Kit SSR and HMR, calling Go for application logic
(`ephemeral/projects/dev-server/`).

## Workflow

- Invoke the `agent-protocol` skill at session start and after compaction.
- Never write to `docs/` without permission.
- `vp` owns the frontend toolchain (`vp install`, `vp run`, `vp exec`,
  `vp dlx`). Never call pnpm or npm directly.
- After a release publishes, run `brew update` and
  `brew reinstall --cask tylergannon/skgo/skgo`, then check `skgo` reports it.
- Start a watcher on every dispatched run's pid so you wake when it exits.
- Fix problems real code or users hit, not hypotheticals.
- Guide pages and README: `ephemeral/guide-pages/SKILL.md`.

## Rules

- Kit is the spec. Read `example/web/node_modules/@sveltejs/kit` before any
  design question; a mirrored feature obeys kit's constraints. Kit 3 breaks
  model priors: read `ephemeral/sveltekit-current/SKILL.md` first.
- Write Go and test with `go test`. JavaScript only where kit requires it.
- Build the product, not ledgers, proof directories or status documents.
- Delegate missions, not methods: the capability, kit as spec, the proof
  that will be accepted, constraints, and what the agent owns.

## Testing

- Contracts are Go tests at the real handler with literal fixtures. A scenario
  in `example/e2e` exists only when kit's client must act (hydration, enhanced
  submit, client navigation, invalidation, streaming, transport, batching) or
  scripting is off.
- `just test` takes seconds. Run an expensive command once and assert many
  times on its output. Test skgo's behaviour, never a dependency's.
- Never derive an expectation from the thing under test. Anchor it in a
  fixture or a literal.
- Skips are failures, and no recipe means "done". `just test` green, `just e2e`
  green on main, and someone who looked at the running app.
- `skgo_qualification` tags compiler/process and built-example tests;
  `skgo_dev_integration` tags Vite/Go session checks.
