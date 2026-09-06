import type { PageServerLoad } from './$types';

// This is the junkyard app's `+page.server.ts` shape, reduced to the smallest
// thing that answers one question: does kit's client, with `ssr = false`, ask
// the Go server for `__data.json`? Kit decides at build time whether the data
// path exists in the client bundle by looking at whether any built
// `+*.server.js` *exports* a `load` — it never calls it.
export const load: PageServerLoad = () => {
	throw new Error('skgo: server loads are implemented in Go');
};
