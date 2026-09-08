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

// The other half of the same claim. The markup says a method ran; this says the
// value it ran on was still cents on the wire, handed to the client under the
// transport key for `app.decode` to rebuild — so the server did not render the
// price by flattening the type, and the client is not hydrating a plain object
// over markup that claims otherwise.
Then(
	'the document carried the price as a Money of {int} cents',
	async ({ documents, shot }, cents: number) => {
		const html = await documentText(documents);
		expect(html).toContain(`price:app.decode("Money", {cents:${cents}})`);
		await shot();
	}
);

// A boundary with a `pending` snippet renders the snippet instead of its
// children while the document is built, so the query behind it is never called
// there. This says the loading state was in the bytes — not merely that the
// plans were absent, which a page that rendered nothing at all would satisfy.
Then('the document carried the plans as still loading', async ({ documents, shot }) => {
	const html = await documentText(documents);
	expect(html).toContain('data-testid="plans-pending"');
	expect(html).not.toContain('data-testid="plan"');
	await shot();
});

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
Then('the document carried no rendered page', async ({ documents, page, shot }) => {
	const html = await documentText(documents);
	expect(html).not.toContain('data-testid="title"');
	expect(html).not.toContain('data-testid="site-name"');
	expect(html, 'the shell still has to boot kit').toContain('kit.start(app, element');
	// The claim is about bytes that have already been read, so the frame left
	// behind can wait for the shell to have become a page. Without this it is a
	// photograph of an empty document — true, and no use to anyone looking at
	// it.
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
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
const concurrent: { second: Page | null; html: string[] } = { second: null, html: [] };

When(
	'{string} and {string} are asked for at the same moment',
	async ({ page, browser }, first: string, second: string) => {
		// The scenario's own page takes the first URL and a second browser
		// takes the other, so every frame this scenario leaves behind is a page
		// somebody actually looked at.
		const context = await browser.newContext();
		concurrent.second = await context.newPage();
		const baseURL = process.env.BASE_URL ?? '';

		// Both navigations are started before either is awaited, so the two
		// renders are in the server at the same time. On a single shared
		// runtime the second would overwrite the first's render context and one
		// of the documents would come back empty or carrying the other's page.
		const responses = await Promise.all([
			page.goto(baseURL + first),
			concurrent.second.goto(baseURL + second)
		]);

		concurrent.html = await Promise.all(responses.map((response) => response!.text()));
	}
);

Then('the first document says the item is named {string}', async ({ shot }, name: string) => {
	expect(concurrent.html[0]).toContain(`<p data-testid="item-name">${name}</p>`);
	await shot('first');
});

Then('the second document says the item is named {string}', async ({ page }, name: string) => {
	expect(concurrent.html[1]).toContain(`<p data-testid="item-name">${name}</p>`);
	await screenshot(concurrent.second!, 'two-pages-at-once-second');
	// And the first page again, so the pair can be read side by side.
	await screenshot(page, 'two-pages-at-once-first');
});

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
	await concurrent.second!.context().close();
	concurrent.second = null;
});

/** Photographs a page this scenario opened itself. */
async function screenshot(page: Page, name: string) {
	const mode = process.env.EXPECTED_MODE ?? 'unknown';
	await page.screenshot({
		path: `../../ephemeral/screenshots/ssr/${mode}/${name}.png`,
		fullPage: true
	});
}
