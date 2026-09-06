import { getSlowPart } from './slow.remote';
import type { PageLoad } from './$types';

// Kit streams a promise returned from a load: the page renders `fast`
// immediately and fills `slow` in when it resolves.
export const load: PageLoad = () => ({
	fast: 'fast part',
	slow: getSlowPart()
});
