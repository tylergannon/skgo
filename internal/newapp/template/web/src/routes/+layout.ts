import { dev } from '$app/env';

// Pages render in the Go process. Under `vite dev` kit's own dev server would
// render them instead and call the remote-function stubs, which throw, so dev
// is client-rendered for now. This line is interim and goes away with
// https://github.com/tylergannon/skgo/issues/40.
export const ssr = !dev;
