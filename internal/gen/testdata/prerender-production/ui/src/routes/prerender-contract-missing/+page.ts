import { noargValue } from '../prerender-contract/fixture.remote';

export async function load() {
	// @ts-expect-error exercise the production null-key artifact miss
	return { value: await noargValue(null) };
}
