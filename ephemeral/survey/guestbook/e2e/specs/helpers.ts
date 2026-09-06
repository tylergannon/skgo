import { expect, type Page } from '@playwright/test';

/** Records every request the page makes from now on. */
export function recordRequests(page: Page) {
	const requests: { url: string; type: string }[] = [];
	page.on('request', (request) => {
		requests.push({ url: request.url(), type: request.resourceType() });
	});
	return {
		all: () => requests,
		matching: (pattern: RegExp) => requests.filter((r) => pattern.test(r.url)),
		documents: () => requests.filter((r) => r.type === 'document'),
		data: () => requests.filter((r) => r.type === 'fetch' || r.type === 'xhr')
	};
}

export async function waitForHydration(page: Page) {
	await expect(page.getByTestId('hydrated')).toHaveText('yes');
}

/** Screenshots the page at the moment a scenario asserts. */
export async function shot(page: Page, name: string) {
	await page.screenshot({ path: `shots/${name}.png`, fullPage: true });
}
