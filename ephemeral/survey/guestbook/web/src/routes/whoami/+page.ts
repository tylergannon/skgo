import { whoami } from '../session.remote';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => await whoami();
