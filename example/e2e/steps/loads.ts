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

// The serial counts this visitor's runs of the layout load, so the number is
// the scenario's to state, not one it read off the page a step earlier.
Then('the layout serial is {int}', async ({ page }, expected: number) => {
	await expect(page.getByTestId('account-serial')).toHaveText(String(expected), {
		timeout: 15_000
	});
});

// The same claim after a navigation that must not have re-run the load. A
// re-run would already be on its way; give it a beat to land.
Then('the layout serial is still {int}', async ({ page }, expected: number) => {
	await page.waitForTimeout(300);
	await expect(page.getByTestId('account-serial')).toHaveText(String(expected));
});

Then('the error message is {string}', async ({ page }, message: string) => {
	await expect(page.getByTestId('error-message')).toHaveText(message, { timeout: 15_000 });
});
