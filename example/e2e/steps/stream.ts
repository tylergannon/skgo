import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * Opens a page and comes back as soon as the response has started, rather than
 * when it has finished.
 *
 * That distinction is the whole scenario. A streamed document's `load` event
 * does not fire until the last value has arrived, so `page.goto` with its
 * default wait would return a second and a half later — with the orders already
 * on screen and the loading state gone unseen.
 */
When('I start loading {string}', async ({ page }, path: string) => {
	await page.goto(path, { waitUntil: 'commit' });
});

// The total is a value the load had in hand and the orders are the value it
// promised, so this is the page mid-flight: rendered, showing what Go knew, and
// still waiting for what Go did not know yet. The total is asserted first
// because a page that rendered nothing has no loading state either — "the rows
// are absent" alone would be satisfied by a blank document.
Then(
	'the page says there are {int} orders and is still fetching them',
	async ({ page, shot }, total: number) => {
		await expect(page.getByTestId('order-total')).toHaveText(`${total} orders`);
		await expect(page.getByTestId('orders-pending')).toBeVisible();
		await expect(page.getByTestId('order')).toHaveCount(0);
		await shot('pending');
	}
);

When('the orders arrive', async ({ page }) => {
	await expect(page.getByTestId('orders-pending')).toHaveCount(0, { timeout: 15_000 });
});

Then('the orders are {string} and {string}', async ({ page, shot }, first: string, second: string) => {
	await expect(page.getByTestId('order')).toHaveText([first, second]);
	await shot('resolved');
});

/**
 * The other half of the claim, in the bytes rather than on the screen.
 *
 * The document ends at `</html>`, and everything after it is what kit appends
 * to the same response as each promise settles: one `<script>` per value,
 * calling the `resolve` the boot script declared. So the loading state has to
 * be inside the document and the orders outside it — if the orders were in the
 * markup the page never had a loading state to show, and if they were in a
 * second response the browser went back for them.
 */
Then(
	'the document carried the loading state, and the orders after it ended',
	async ({ documents, shot }) => {
		expect(documents.last, 'no document response was observed').not.toBeNull();
		// The document the browser was given for this page, not the one it was
		// given for the page it signed in on.
		expect(new URL(documents.last!.url()).pathname).toBe('/account/orders');
		const html = await documents.last!.text();

		const end = html.indexOf('</html>');
		expect(end, 'the response carried no document at all').toBeGreaterThan(0);
		const document = html.slice(0, end);
		const appended = html.slice(end);

		expect(document).toContain('data-testid="orders-pending"');
		expect(document).toContain('<p data-testid="order-total">2 orders</p>');
		expect(document, 'the orders were in the document, so the page never had a loading state').not.toContain(
			'a slow parcel'
		);

		expect(appended, 'nothing was appended to the document').toContain('.resolve(');
		expect(appended).toContain('a slow parcel');
		expect(appended).toContain('a slower parcel');

		await shot('streamed');
	}
);
