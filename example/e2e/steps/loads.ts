import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

Given('nobody has signed in', async ({ page, shot }) => {
	// A scenario gets its own browser context, so nobody is signed in yet. The
	// step exists so the feature file says which visitor it is talking about.
	await page.goto('/');
	await expect(page.getByTestId('session')).toHaveText('Signed out', { timeout: 15_000 });
	await shot('signed-out');
});

Given('I have signed in as {string}', async ({ page }, user: string) => {
	await page.goto('/todos');
	await page.getByTestId('user').fill(user);
	await page.getByTestId('sign-in').click();
	await expect(page.getByTestId('session')).toHaveText(`Signed in as ${user}`, {
		timeout: 15_000
	});
});

When('I visit {string}', async ({ page }, path: string) => {
	await page.goto(path);
});

When('I follow the {string} link', async ({ page }, name: string) => {
	await page.getByRole('link', { name, exact: true }).click();
});

When('I refresh the account layout', async ({ page }) => {
	await page.getByTestId('refresh-account').click();
});

When('I note the data request count', async ({ data }) => {
	data.mark();
});

Then('I land on {string}', async ({ page, shot }, path: string) => {
	await expect(page).toHaveURL(new RegExp(`${path.replace(/\//g, '\\/')}$`), { timeout: 15_000 });
	await shot('landed');
});

Then('the account layout greets {string}', async ({ page, shot }, user: string) => {
	await expect(page.getByTestId('account-user')).toHaveText(`Account of ${user}`, {
		timeout: 15_000
	});
	await shot();
});

Then('the account layout is not on the page', async ({ page, shot }) => {
	await expect(page.getByTestId('account-user')).toHaveCount(0);
	await shot();
});

Then('the account page says its parent loaded {string}', async ({ page }, user: string) => {
	await expect(page.getByTestId('parent-user')).toHaveText(`The layout loaded ${user}`, {
		timeout: 15_000
	});
});

Then('exactly {int} data request was made', async ({ data, shot }, expected: number) => {
	expect(data.count, `data requests: ${data.urls.join(', ') || 'none'}`).toBe(expected);
	await shot();
});

Then(
	'exactly {int} data requests were made since',
	async ({ page, data, shot }, expected: number) => {
		// A second round trip would already be in flight; wait a beat so it
		// lands and can be counted.
		await page.waitForTimeout(500);
		expect(data.since, `data requests: ${data.urlsSince.join(', ') || 'none'}`).toBe(expected);
		await shot('resolved');
	}
);

// The total is a field the load had in hand; the orders are a field it promised
// for later. Both are in the document, because Go settles what a load promised
// before it renders — and the two are compared against each other rather than
// against a number written here, so a page that rendered nothing fails.
Then(
	'the document already carried as many orders as the total said',
	async ({ documents, shot }) => {
		expect(documents.last, 'no document response was observed').not.toBeNull();
		const html = await documents.last!.text();

		const total = Number(
			/data-testid="order-total">(\d+)/.exec(html)?.[1] ?? NaN
		);
		expect(Number.isFinite(total), 'the document carried no order total').toBe(true);
		expect(total, 'the order total was zero, so counting rows proves nothing').toBeGreaterThan(0);

		const rows = html.match(/data-testid="order"/g)?.length ?? 0;
		expect(rows).toBe(total);
		await shot('in-the-document');
	}
);

Then("the browser never asked for the page's data", async ({ page, data, shot }) => {
	// A refetch would already be in flight; wait a beat so it lands and can be
	// counted.
	await page.waitForTimeout(500);
	expect(data.count, `data requests: ${data.urls.join(', ') || 'none'}`).toBe(0);
	await shot('no-refetch');
});

// Cross-endpoint in spirit: the number was in the first line of the response and
// the list arrived in a later one, so asserting one against the other is an
// assertion about two things the visitor can see, not about the implementation.
Then('there are as many orders as the total said', async ({ page }) => {
	const total = Number((await page.getByTestId('order-total').innerText()).split(' ')[0]);
	expect(Number.isFinite(total), 'the order total was not a number').toBe(true);
	await expect(page.getByTestId('order')).toHaveCount(total);
});

When('I note the layout serial', async ({ page, notes }) => {
	notes.set('layout serial', await layoutSerial(page));
});

Then('the layout serial is unchanged', async ({ page, notes, shot }) => {
	await page.waitForTimeout(300);
	expect(await layoutSerial(page)).toBe(noted(notes, 'layout serial'));
	await shot();
});

Then('the layout serial has changed', async ({ page, notes, shot }) => {
	const before = noted(notes, 'layout serial');
	await expect
		.poll(async () => await layoutSerial(page), { timeout: 15_000 })
		.not.toBe(before);
	await shot('after-refresh');
});

Then('the error message is {string}', async ({ page, shot }, message: string) => {
	await expect(page.getByTestId('error-message')).toHaveText(message, { timeout: 15_000 });
	await shot();
});

async function layoutSerial(page: import('@playwright/test').Page): Promise<number> {
	const text = await page.getByTestId('account-serial').innerText();
	const value = Number(text.trim());
	expect(Number.isFinite(value), `the layout serial was ${JSON.stringify(text)}`).toBe(true);
	return value;
}

function noted(notes: Map<string, number>, label: string): number {
	const value = notes.get(label);
	expect(value, `${JSON.stringify(label)} was never noted`).not.toBeUndefined();
	return value!;
}
