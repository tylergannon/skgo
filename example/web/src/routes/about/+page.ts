/**
 * Not prerendered any more, and the reason is worth writing down.
 *
 * A page can only be prerendered if nothing in its branch has a `+*.server.ts`.
 * Kit runs the server loads of a branch while it prerenders, and skgo's are
 * generated stubs that throw — so a prerendered page with a Go load fails the
 * build rather than shipping silently wrong.
 *
 * The root layout is in every branch, and it now has a Go load
 * (src/routes/layout.server.go), which is the fixture for kit's static
 * error.html: a load that fails there has no `+error.svelte` above it. That
 * makes every page in this app un-prerenderable, this one included. The two
 * cannot both be demonstrated by one app until skgo can answer a server load
 * while the build prerenders, which is #81.
 */
export const prerender = false;
