import { dev } from '$app/env';

/**
 * No client-side JavaScript at all. Kit leaves the boot script out of a
 * document whose branch turns `csr` off, so this page is HTML and nothing else
 * — and everything on it had to be rendered on the server to be there.
 *
 * Which is why it has to have CSR in dev. The root layout turns `ssr` off
 * there, and a branch with neither `ssr` nor `csr` has nobody left to render
 * it: kit answers with an empty shell that boots nothing, and the page is
 * blank. `csr` off is a claim about a rendered document, so it applies where
 * there is one.
 */
export const csr = dev;
