import type { FullConfig } from '@playwright/test';

export default async function verifyServerMode(config: FullConfig): Promise<void> {
	const expected = process.env.SKGO_EXPECTED_MODE;
	if (expected !== 'dev' && expected !== 'prod') {
		throw new Error(`SKGO_EXPECTED_MODE must be dev or prod, got ${JSON.stringify(expected)}`);
	}

	const baseURL = config.projects[0]?.use.baseURL;
	if (typeof baseURL !== 'string') {
		throw new Error(`BASE_URL must name the running skgo server, got ${JSON.stringify(baseURL)}`);
	}

	const response = await fetch(baseURL, { method: 'HEAD' });
	const observed = response.headers.get('x-skgo-mode');
	if (observed !== expected) {
		throw new Error(
			`expected ${expected} mode at ${baseURL}, X-Skgo-Mode was ${JSON.stringify(observed)}`
		);
	}
}
