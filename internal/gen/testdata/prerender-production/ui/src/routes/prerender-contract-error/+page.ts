import { noargValue } from '../prerender-contract/fixture.remote';

export async function load() {
	return { value: await noargValue() };
}
