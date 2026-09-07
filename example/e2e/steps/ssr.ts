import type { Page } from '@playwright/test';
import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * The bytes the server sent, before a single line of JavaScript ran.
 *
 * A page's markup is only proof of server-side rendering if it was in the
 * response: the same content appears in the DOM a moment later either way, so
 * asserting on the DOM alone cannot tell a rendered page from a hydrated one.
 */
async function documentText(documents: { last: import('@playwright/test').Response | null }) {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	return await documents.last!.text();
}

Then('the document already said {string}', async ({ documents, shot }, text: string) => {
	expect(await documentText(documents)).toContain(text);
	await shot();
});

Then(
	'the document already said the site is named {string}',
	async ({ documents, shot }, name: string) => {
		expect(await documentText(documents)).toContain(`<p data-testid="site-name">${name}</p>`);
		await shot();
	}
);

Then(
	'the document already said the item is named {string}',
	async ({ documents, shot }, name: string) => {
		expect(await documentText(documents)).toContain(`<p data-testid="item-name">${name}</p>`);
		await shot();
	}
);

Then('the document never mentions {string}', async ({ documents, shot }, text: string) => {
	expect(await documentText(documents)).not.toContain(text);
	await shot();
});

// A document with no script is the whole claim of `csr = false`: kit leaves the
// boot script out, so what arrived is all there will ever be.
Then('the document carries no script', async ({ documents, page, shot }) => {
	expect(await documentText(documents)).not.toContain('<script');
	await expect(page.locator('script')).toHaveCount(0);
	await shot();
});

// The other side of the same coin. `ssr = false` means the document is kit's
// shell: none of the page is in it, and all of it is on screen once the client
// has booted — which the step after this one asserts, so this is not a claim
// about a page that simply never rendered.
Then('the document carried no rendered page', async ({ documents, shot }) => {
	const html = await documentText(documents);
	expect(html).not.toContain('data-testid="title"');
	expect(html).not.toContain('data-testid="site-name"');
	expect(html, 'the shell still has to boot kit').toContain('kit.start(app, element');
	await shot();
});

Then(
	'exactly {int} remote requests were made since',
	async ({ page, remotes, shot }, expected: number) => {
		// A refetch would already be in flight; wait a beat so it lands and can
		// be counted.
		await page.waitForTimeout(500);
		expect(remotes.since, `remote requests: ${remotes.urlsSince.join(', ') || 'none'}`).toBe(
			expected
		);
		await shot('hydrated');
	}
);

Then('nothing on the page failed to load', async ({ page, shot }) => {
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
	await shot();
});

/** The two documents the concurrency scenario fetched, and their HTML. */
const concurrent: { pages: Page[]; html: string[] } = { pages: [], html: [] };

When(
	'{string} and {string} are asked for at the same moment',
	async ({ browser }, first: string, second: string) => {
		const contexts = await Promise.all([browser.newContext(), browser.newContext()]);
		const pages = await Promise.all(contexts.map((context) => context.newPage()));
		const baseURL = process.env.BASE_URL ?? '';

		// Both navigations are started before either is awaited, so the two
		// renders are in the server at the same time. On a single shared
		// runtime the second would overwrite the first's render context and one
		// of the documents would come back empty or carrying the other's page.
		const responses = await Promise.all([
			pages[0].goto(baseURL + first),
			pages[1].goto(baseURL + second)
		]);

		concurrent.pages = pages;
		concurrent.html = await Promise.all(responses.map((response) => response!.text()));
	}
);

Then(
	'the first document says the item is named {string}',
	async ({ shot }, name: string) => {
		expect(concurrent.html[0]).toContain(`<p data-testid="item-name">${name}</p>`);
		await shotOf(concurrent.pages[0], 'first');
		await shot('first-live');
	}
);

Then(
	'the second document says the item is named {string}',
	async ({ shot }, name: string) => {
		expect(concurrent.html[1]).toContain(`<p data-testid="item-name">${name}</p>`);
		await shotOf(concurrent.pages[1], 'second');
		await shot('second-live');
	}
);

Then("neither document carries the other's item", async ({ shot }) => {
	const names = concurrent.html.map((html) => {
		const match = /<p data-testid="item-name">([^<]*)<\/p>/.exec(html);
		expect(match, 'a document carried no item at all').not.toBeNull();
		return match![1];
	});
	expect(names[0]).not.toEqual(names[1]);
	for (const [i, html] of concurrent.html.entries()) {
		expect(html).not.toContain(`>${names[1 - i]}</p>`);
	}
	await shot('distinct');
	for (const page of concurrent.pages) await page.context().close();
	concurrent.pages = [];
});

/** Photographs one of the concurrency scenario's own pages. */
async function shotOf(page: Page, name: string) {
	const mode = process.env.EXPECTED_MODE ?? 'unknown';
	await page.screenshot({
		path: `../../ephemeral/screenshots/ssr/${mode}/two-pages-at-once-${name}.png`,
		fullPage: true
	});
}
