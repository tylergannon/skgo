import { createBdd } from 'playwright-bdd';
import { readFileSync } from 'node:fs';
import { expect, hydrated, test } from './fixtures';

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
	await hydrated(page);
	await page.getByTestId('user').fill(user);
	await page.getByTestId('sign-in').click();
	await expect(page.getByTestId('session')).toHaveText(`Signed in as ${user}`, {
		timeout: 15_000
	});
});

Given('the no-script browser has a session for {string}', async ({ page }, user: string) => {
	const manifest = JSON.parse(
		readFileSync(new URL('../../web/skgo.remotes.json', import.meta.url), 'utf8')
	) as { remotes: string[] };
	const id = manifest.remotes.find((candidate) => candidate.endsWith('/signIn'));
	expect(id, 'the generated remote list names no signIn command').toBeTruthy();
	const payload = Buffer.from(JSON.stringify([user]), 'utf8').toString('base64url');
	const response = await page.request.post(`/_app/remote/${id}`, {
		headers: { origin: process.env.BASE_URL ?? '' },
		data: { payload, refreshes: [] }
	});
	expect(response.status(), await response.text()).toBe(200);
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
