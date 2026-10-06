import { noargValue } from '../prerender-contract/fixture.remote';

export const prerender = false;

export async function load() {
	return { value: await noargValue() };
}
