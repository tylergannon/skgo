import type { PageLoad } from './$types';
import { getItem } from './item.remote';

export const load: PageLoad = async ({ params }) => ({
	loadedItem: await getItem(params.id)
});
