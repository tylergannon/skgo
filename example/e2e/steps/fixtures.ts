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

/**
 * Records the data traffic (`/<route>/__data.json`) of one scenario. It is the
 * only endpoint kit's client calls on its own initiative, once per navigation,
 * so counting it is how a scenario says "and nothing went back for more".
 */
export type Data = {
	/** How many data requests the browser made in the scenario so far. */
	count: number;
	/** Their URLs, in order — used to explain a failed count. */
	urls: string[];
	/** Freeze the current count so `since` can measure a single interaction. */
	mark(): void;
	/** How many data requests were made since the last `mark()`. */
	readonly since: number;
	/** The URLs of those requests. */
	readonly urlsSince: string[];
};

/** Scratch values a scenario carries from one step to the next. */
export type Notes = Map<string, number>;

/**
 * Screenshots the page in whatever state the scenario left it, named after the
 * scenario. Every loads scenario leaves one behind, because the sprint's
 * acceptance is somebody looking at all of them.
 */
export type Shot = (name?: string) => Promise<void>;

export const test = base.extend<{
	documents: Documents;
	remotes: Remotes;
	data: Data;
	notes: Notes;
	shot: Shot;
}>({
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

	// `auto` so the listener is attached before the scenario's first navigation.
	data: [
		async ({ page }, use) => {
			let marked = 0;
			const data: Data = {
				count: 0,
				urls: [],
				mark() {
					marked = data.count;
				},
				get since() {
					return data.count - marked;
				},
				get urlsSince() {
					return data.urls.slice(marked);
				}
			};

			page.on('request', (request) => {
				if (!request.url().includes('/__data.json')) return;
				data.count++;
				data.urls.push(`${request.method()} ${new URL(request.url()).pathname}`);
			});

			await use(data);
		},
		{ auto: true }
	],

	// eslint-disable-next-line no-empty-pattern -- Playwright infers fixture deps from this pattern.
	notes: async ({}, use) => {
		await use(new Map<string, number>());
	},

	shot: async ({ page }, use, testInfo) => {
		// The title of a Scenario Outline's example is just "Example #1", so the
		// scenario's own title has to come along or three redirects overwrite
		// each other.
		const slug = testInfo.titlePath
			.slice(-2)
			.join('-')
			.replace(/[^a-z0-9]+/gi, '-')
			.replace(/^-|-$/g, '')
			.toLowerCase();
		let taken = 0;
		const shot: Shot = async (name) => {
			const suffix = name ? `-${name}` : taken > 0 ? `-${taken}` : '';
			taken += 1;
			await page.screenshot({
				path: `../../ephemeral/screenshots/loads/${slug}${suffix}.png`,
				fullPage: true
			});
		};
		await use(shot);
		// A scenario that took no shot of its own still leaves the state it
		// finished in.
		if (taken === 0) await shot('final');
	}
});

export { expect };
