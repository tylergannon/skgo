import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then(
	'the quote for {string} is {string}, answered in a call of {int}',
	async ({ page, shot }, symbol: string, price: string, size: number) => {
		const rows = page.locator('[data-testid="quote"]');
		const quote = page.locator(`[data-testid="quote"][data-symbol="${symbol}"]`);
		await expect(quote).toBeVisible({ timeout: 15_000 });
		await expect(quote.getByTestId('quote-price')).toHaveText(price);
		await expect(quote.getByTestId('quote-batch')).toHaveText(`batch of ${size}`);
		// The size on a row is only meaningful beside the rows it claims to have
		// been answered with: "batch of 4" on a page showing one row would be
		// the server reporting a call the page never made.
		expect(await rows.count()).toBe(size);
		await shot();
	}
);

Then(
	'the document already said the quote for {string} is {string}, answered in a call of {int}',
	async ({ documents, shot }, symbol: string, price: string, size: number) => {
		expect(documents.last, 'no document response was observed').not.toBeNull();
		const html = await documents.last!.text();
		const row = new RegExp(
			`<tr data-testid="quote" data-symbol="${symbol}">.*?` +
				`<td data-testid="quote-price">${price.replace('.', '\\.')}</td>.*?` +
				`<td data-testid="quote-batch">batch of ${size}</td>`,
			's'
		);
		expect(html).toMatch(row);
		await shot();
	}
);

Then('the quotes endpoint was asked exactly once since', async ({ page, remotes, shot }) => {
	// A second round trip would already be in flight; wait a beat so it lands
	// and can be counted.
	await page.waitForTimeout(500);
	const asked = remotes.urlsSince.filter((url) => url.endsWith('/getQuotes'));
	expect(asked.length, `remote requests: ${remotes.urlsSince.join(', ') || 'none'}`).toBe(1);
	expect(asked[0]).toMatch(/^POST \/_app\/remote\/[a-z0-9]+\/getQuotes$/);
	await shot();
});
