import { json } from '@sveltejs/kit';
import type { RequestHandler } from './$types';

// Ported verbatim from the junkyard app. skgo builds no server bundle, so
// whether this is reachable at all is one of the things the survey measures.
export const POST: RequestHandler = async ({ request }) => {
	const body = await request.arrayBuffer();
	return json({ bytes: body.byteLength });
};
