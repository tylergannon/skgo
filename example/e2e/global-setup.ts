import type { FullConfig } from '@playwright/test';

export default async function verifyServerMode(config: FullConfig): Promise<void> {
	const baseURL = config.projects[0]?.use.baseURL;
	if (typeof baseURL !== 'string') {
		throw new Error(`BASE_URL must name the running skgo server, got ${JSON.stringify(baseURL)}`);
	}

	const mode = process.env.SKGO_E2E_MODE ?? 'prod';
	if (mode !== 'prod' && mode !== 'dev') throw new Error(`unsupported SKGO_E2E_MODE ${JSON.stringify(mode)}`);
	const response = await fetch(baseURL, { method: 'HEAD' });
	const observed = response.headers.get('x-skgo-mode');
	if (observed !== mode) {
		throw new Error(
			`expected ${mode} handler at ${baseURL}, X-Skgo-Mode was ${JSON.stringify(observed)}`
		);
	}
}
