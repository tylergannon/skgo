import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * The three strings src/routes/stream/page.server.go promises. Naming them here
 * is what lets a step say which value a chunk carried; which order they come in
 * is the scenario's business, and only the scenario's.
 */
const settledValues = [
	'the first thing to arrive',
	'the second thing to arrive',
	'the last thing to arrive'
];

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

/**
 * The page that promises three values, mid-flight.
 *
 * The headline is a value the load had in hand, and it is asserted first
 * because a page that rendered nothing has no loading state either — "the
 * values are absent" alone would be satisfied by a blank document.
 */
Then(
	'the page says {string} and is waiting for all three values',
	async ({ page, shot }, headline: string) => {
		await expect(page.getByTestId('headline')).toHaveText(headline, { timeout: 15_000 });
		await expect(page.getByTestId('ticker-pending')).toBeVisible();
		await expect(page.getByTestId('digest-pending')).toBeVisible();
		await expect(page.getByTestId('forecast-pending')).toBeVisible();
		await expect(page.getByTestId('ticker')).toHaveCount(0);
		await expect(page.getByTestId('digest-line')).toHaveCount(0);
		await expect(page.getByTestId('forecast')).toHaveCount(0);
		await shot('all-three-pending');
	}
);

When('the ticker arrives', async ({ page }) => {
	await expect(page.getByTestId('ticker-pending')).toHaveCount(0, { timeout: 15_000 });
});

When('every promised value has arrived', async ({ page }) => {
	await expect(page.getByTestId('forecast-pending')).toHaveCount(0, { timeout: 20_000 });
});

Then('the ticker says {string}', async ({ page, shot }, value: string) => {
	await expect(page.getByTestId('ticker')).toHaveText(value);
	await shot('ticker');
});

/**
 * The half of the ordering claim a visitor can see: the value that was ready
 * first is on the page while the two that were not are still loading. A page
 * that waited for all three, or that filled them in in the order it numbered
 * them, cannot be in this state.
 */
Then('the digest and the forecast are still pending', async ({ page, shot }) => {
	await expect(page.getByTestId('digest-pending')).toBeVisible();
	await expect(page.getByTestId('forecast-pending')).toBeVisible();
	await shot('ticker-in-digest-and-forecast-out');
});

Then(
	'the digest reads {string} and {string}',
	async ({ page, shot }, first: string, second: string) => {
		await expect(page.getByTestId('digest-line')).toHaveText([first, second]);
		await shot('digest');
	}
);

Then('the forecast says {string}', async ({ page, shot }, value: string) => {
	await expect(page.getByTestId('forecast')).toHaveText(value);
	await shot('all-three-settled');
});

/**
 * The document itself: three loading states inside it and not one of the three
 * values, which is what says the page was sent before Go had them.
 */
Then(
	'the document held all three loading states and none of the three values',
	async ({ documents }) => {
		const html = await documentBody(documents, '/stream');
		const end = html.indexOf('</html>');
		expect(end, 'the response carried no document at all').toBeGreaterThan(0);
		const document = html.slice(0, end);

		for (const state of ['ticker-pending', 'digest-pending', 'forecast-pending']) {
			expect(document, `the document had no ${state}`).toContain(`data-testid="${state}"`);
		}
		for (const value of settledValues) {
			expect(
				document,
				`"${value}" was in the document, so the page never showed a loading state for it`
			).not.toContain(value);
		}
	}
);

/**
 * And the other half, in the bytes: what Go appended after `</html>`, in the
 * order it appended it.
 *
 * Kit writes each settled value as a script of its own —
 * `<script>__sveltekit_xxx.resolve(<id>, () => [<value>])</script>` — so the
 * ids and the order are both readable straight off the response. The table in
 * the scenario says which number goes with which value and what order the three
 * lines come in; nothing here is read off one to check the other.
 */
Then(
	'the document was followed by these values, in this order',
	async ({ documents, shot }, table: { hashes(): Array<Record<string, string>> }) => {
		const html = await documentBody(documents, '/stream');
		const appended = html.slice(html.indexOf('</html>'));
		expect(appended, 'nothing was appended to the document').toContain('.resolve(');
		expect(chunkOrder(appended, /\.resolve\((\d+),[^]*?\)<\/script>/g, appended)).toEqual(
			expected(table)
		);
		await shot('streamed');
	}
);

/**
 * The bytes of the response the navigation just read — asked for a second time,
 * over HTTP, rather than taken out of the browser.
 *
 * The browser will not give it up: Chrome does not keep the body of a streamed
 * fetch the page has already consumed, and `response.text()` on it fails with
 * `No data found for resource` however early it is called. So the claim about
 * what the browser did with the response is the one above, in the DOM, and the
 * claim about what was in it is this one.
 *
 * The URL is the one kit's client asks for, invalidation parameter and all: the
 * client sends one digit per node in the branch, and a `__data.json` asked for
 * without it is a different request.
 */
Then(
	'that data response, asked for again, named three promises and carried none of their values',
	async ({ request, data, notes, shot }) => {
		expect(data.last, 'the browser made no data request').not.toBeNull();
		const asked = new URL(data.last!.url());
		expect(asked.pathname).toBe('/stream/__data.json');

		const res = await request.get(asked.pathname + asked.search);
		expect(res.status()).toBe(200);
		expect(res.headers()['content-type']).toBe('text/sveltekit-data');
		const body = await res.text();
		dataResponses.set(notes, body);

		const head = body.split('\n')[0];
		expect(head.match(/\["Promise"/g) ?? [], `the head named no promises:\n${head}`).toHaveLength(
			3
		);
		for (const value of settledValues) {
			expect(head, `"${value}" was in the head, so it was not promised at all`).not.toContain(value);
		}
		await shot('data-stream');
	}
);

Then(
	'it carried these values, in this order',
	async ({ notes }, table: { hashes(): Array<Record<string, string>> }) => {
		const body = dataResponse(notes);
		const chunks = body.split('\n').slice(1).join('\n');
		expect(chunkOrder(chunks, /"type":"chunk","id":(\d+)/g, body)).toEqual(expected(table));
	}
);

// The body the step above read. Notes hold numbers, so it lives beside them
// rather than in them.
const dataResponses = new WeakMap<object, string>();

function dataResponse(notes: object): string {
	const body = dataResponses.get(notes);
	expect(body, 'the scenario asked for no data response').toBeTruthy();
	return body!;
}

/** The table as `<id> <value>` pairs, in the order the scenario wrote them. */
function expected(table: { hashes(): Array<Record<string, string>> }): string[] {
	return table.hashes().map((row) => `${row.promise} ${row.value}`);
}

/**
 * The same pairs, read off the wire. `haystack` is the whole response, so a
 * mismatch says what actually arrived instead of just that something did.
 */
function chunkOrder(text: string, ids: RegExp, haystack: string): string[] {
	const found = [...text.matchAll(ids)].map((match) => {
		const from = match.index ?? 0;
		const upTo = text.indexOf('\n', from + 1);
		const chunk = text.slice(from, upTo === -1 ? undefined : upTo);
		const value = settledValues.find((v) => chunk.includes(v));
		return `${match[1]} ${value ?? `<no known value in ${chunk}>`}`;
	});
	expect(found.length, `expected three settled values in:\n${haystack}`).toBe(3);
	return found;
}

/** The document response for `path`, waited out to its last byte. */
async function documentBody(
	documents: { last: import('@playwright/test').Response | null },
	path: string
): Promise<string> {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(new URL(documents.last!.url()).pathname).toBe(path);
	return documents.last!.text();
}
