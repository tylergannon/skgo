import type { FullConfig } from '@playwright/test';

export default async function verifyProductionServer(config: FullConfig): Promise<void> {
	const baseURL = config.projects[0]?.use.baseURL;
	if (typeof baseURL !== 'string') {
		throw new Error(`BASE_URL must name the running skgo server, got ${JSON.stringify(baseURL)}`);
	}

	const response = await fetch(baseURL, { method: 'HEAD' });
	const observed = response.headers.get('x-skgo-mode');
	if (observed !== 'prod') {
		throw new Error(
			`expected production handler at ${baseURL}, X-Skgo-Mode was ${JSON.stringify(observed)}`
		);
	}
}
