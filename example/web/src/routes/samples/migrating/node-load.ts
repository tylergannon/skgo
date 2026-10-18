import type { ServerLoad } from "@sveltejs/kit";

// The server load of a Node SvelteKit app's
// src/routes/prerender/[slug]/+page.server.ts, before it moved to Go as
// src/routes/prerender/[slug]/page.server.go.
export const load: ServerLoad<{ slug: string }> = async ({ params }) => {
  return { slug: params.slug, receipt: `Go entry load: ${params.slug}` };
};
