import { createBdd } from 'playwright-bdd';
import { booted, expect, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

Given('I have signed in as {string}', async ({ page }, user: string) => {
	await page.goto('/todos');
	await hydrated(page);
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
	await hydrated(page);
	await page.getByRole('link', { name, exact: true }).click();
});

When('I refresh the account layout', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('refresh-account').click();
});

When('I note the data request count', async ({ data }) => {
	data.mark();
});

Then('the account layout greets {string}', async ({ page }, user: string) => {
	await expect(page.getByTestId('account-user')).toHaveText(`Account of ${user}`, {
		timeout: 15_000
	});
});

Then(
	'exactly {int} data requests were made since',
	async ({ page, data }, expected: number) => {
		// A count of zero is also what a page whose client never started would
		// show, so the client has to be running before the count means anything.
		await booted(page);
		// A second round trip would already be in flight; wait a beat so it
		// lands and can be counted.
		await page.waitForTimeout(500);
		expect(data.since, `data requests: ${data.urlsSince.join(', ') || 'none'}`).toBe(expected);
	}
);

Then("the browser never asked for the page's data", async ({ page, data }) => {
	// A count of zero is also what a page whose client never started would
	// show, so the client has to be running before the count means anything.
	await booted(page);
	// A refetch would already be in flight; wait a beat so it lands and can be
	// counted.
	await page.waitForTimeout(500);
	expect(data.count, `data requests: ${data.urls.join(', ') || 'none'}`).toBe(0);
});

When('I note the layout serial', async ({ page, notes }) => {
	notes.set('layout serial', await layoutSerial(page));
});

Then('the layout serial is unchanged', async ({ page, notes }) => {
	await page.waitForTimeout(300);
	expect(await layoutSerial(page)).toBe(noted(notes, 'layout serial'));
});

Then('the layout serial has changed', async ({ page, notes }) => {
	const before = noted(notes, 'layout serial');
	await expect
		.poll(async () => await layoutSerial(page), { timeout: 15_000 })
		.not.toBe(before);
});

Then('the error message is {string}', async ({ page }, message: string) => {
	await expect(page.getByTestId('error-message')).toHaveText(message, { timeout: 15_000 });
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
