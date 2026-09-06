import { expectedError } from '../error.remote';
import type { PageLoad } from './$types';

export const load: PageLoad = async () => ({ never: await expectedError() });
