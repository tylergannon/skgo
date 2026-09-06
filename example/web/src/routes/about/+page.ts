// Prerendered: the build writes about.html and Go serves that file, so this
// page costs no route matching and no rendering at request time.
//
// A page can only be prerendered if nothing in its branch has a `+*.server.ts`.
// Kit runs the server loads of a branch while it prerenders, and skgo's are
// generated stubs that throw — so a prerendered page with a Go load would fail
// the build rather than ship silently wrong.
export const prerender = true;
