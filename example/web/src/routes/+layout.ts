import { dev } from '$app/env';

/**
 * Server rendering is Go's, and Go only has an engine in a built app.
 *
 * `vp dev` never runs the adapter, so there is no SSR bundle in dev; kit's own
 * dev server would render the branch itself, and the only implementation it can
 * reach for a load or a remote function is the generated stub, which throws by
 * design. Turning `ssr` off there is kit's own answer to "do not render this on
 * the server" (packages/kit/src/runtime/server/page/index.js: an `ssr === false`
 * branch returns the shell without running a single load), and it leaves every
 * value on every page coming from exactly where it comes from in production —
 * Go, over `__data.json` and `/_app/remote/...`.
 *
 * It is not a literal, so kit's static analyser cannot fold it
 * (packages/kit/src/exports/vite/static_analysis/index.js returns null for an
 * initialiser that is not a Literal). That is fine and deliberate: the analyser
 * only decides whether dev can skip *loading* the module, and the value kit and
 * skgo both act on is the one this module exports at runtime. In a build `dev`
 * is false, so the adapter reads `ssr: true` and the production document is
 * rendered by Go exactly as before.
 */
export const ssr = !dev;
