import { expect, type Response } from '@playwright/test';
import { test as base } from 'playwright-bdd';

/** Records the document (top-level navigation) traffic of one scenario. */
export type Documents = {
	/** How many document requests the browser made. */
	count: number;
	/** The most recent document response. */
	last: Response | null;
};

export const test = base.extend<{ documents: Documents }>({
	documents: async ({ page }, use) => {
		const documents: Documents = { count: 0, last: null };

		page.on('request', (request) => {
			if (request.resourceType() === 'document') documents.count++;
		});
		page.on('response', (response) => {
			if (response.request().resourceType() === 'document') documents.last = response;
		});

		await use(documents);
	}
});

export { expect };
