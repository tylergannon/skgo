import { expect, type Response } from '@playwright/test';
import { test as base } from 'playwright-bdd';

/** Records the document (top-level navigation) traffic of one scenario. */
export type Documents = {
	/** How many document requests the browser made. */
	count: number;
	/** The most recent document response. */
	last: Response | null;
};

/**
 * Records the programmatic remote-function traffic (`/_app/remote/...`) of one
 * scenario: queries, live-query streams and command invocations all go there.
 */
export type Remotes = {
	/** How many remote requests the browser made in the scenario so far. */
	count: number;
	/** Their URLs, in order — used to explain a failed count. */
	urls: string[];
	/** Freeze the current count so `since` can measure a single interaction. */
	mark(): void;
	/** How many remote requests were made since the last `mark()`. */
	readonly since: number;
	/** The URLs of those requests. */
	readonly urlsSince: string[];
};

/** Scratch values a scenario carries from one step to the next. */
export type Notes = Map<string, number>;

export const test = base.extend<{ documents: Documents; remotes: Remotes; notes: Notes }>({
	documents: async ({ page }, use) => {
		const documents: Documents = { count: 0, last: null };

		page.on('request', (request) => {
			if (request.resourceType() === 'document') documents.count++;
		});
		page.on('response', (response) => {
			if (response.request().resourceType() === 'document') documents.last = response;
		});

		await use(documents);
	},

	// `auto` so the listener is attached before the scenario's first navigation,
	// however late the first step that reads it runs.
	remotes: [
		async ({ page }, use) => {
			let marked = 0;
			const remotes: Remotes = {
				count: 0,
				urls: [],
				mark() {
					marked = remotes.count;
				},
				get since() {
					return remotes.count - marked;
				},
				get urlsSince() {
					return remotes.urls.slice(marked);
				}
			};

			page.on('request', (request) => {
				if (!request.url().includes('/_app/remote/')) return;
				remotes.count++;
				remotes.urls.push(`${request.method()} ${new URL(request.url()).pathname}`);
			});

			await use(remotes);
		},
		{ auto: true }
	],

	// eslint-disable-next-line no-empty-pattern -- Playwright infers fixture deps from this pattern.
	notes: async ({}, use) => {
		await use(new Map<string, number>());
	}
});

export { expect };
