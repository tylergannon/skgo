import { noargValue } from '../prerender-contract/fixture.remote';

export const prerender = true;

export async function load() {
	return { value: await noargValue() };
}
