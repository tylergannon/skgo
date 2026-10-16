# Kit 3.1 dependency refresh

- VitePlus 1.3 executes Vite through `dist/vite/bin/vite.js`; the former
  `node/cli.js` process identity check silently disabled the prerender parent
  observer. The existing outer-CLI signal contract caught a blocked Go producer
  left alive. Keep the check aligned with the qualified launcher entry.
- The npm `vite-plus` package supplies the project CLI, which rejects `env` and
  `upgrade`. `skgo new` needs the standalone global CLI to establish its managed
  environment. Installing npm's package globally does not provide that CLI;
  README installation now uses the official installer and `VP_VERSION`.
- pnpm 12.14 stores package-manager dependencies in a leading YAML lock
  document. In the adapter-only controlled-registry fixture, a package-manager
  pin makes pnpm fetch its own distribution from the fake registry before the
  actual policy assertion. Qualify the executable explicitly and omit that
  unrelated fixture pin.
- Adapter source edits invalidate the example build fingerprint. Run production
  handler qualification after the final adapter build, not alongside edits to
  its embedded JavaScript.
