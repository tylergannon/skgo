import { goSomewhereElse } from './redirect.remote';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => ({ never: await goSomewhereElse() });
