import { getMessage } from '../messages.remote';
import type { PageLoad } from './$types';

export const load: PageLoad = async ({ params }) => ({
	message: await getMessage(params.id)
});
