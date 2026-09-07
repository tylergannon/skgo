import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * The `/api` page prints one line per call it made: the method, the status, the
 * content type, and the `Allow` header when there is one. Reading it is how a
 * scenario about HTTP sees HTTP, and it is what the screenshot shows.
 *
 * Every assertion below retries. A step that read the line once passed on the
 * *previous* call's result, because a click returns before the fetch it started
 * has answered.
 */
function responseLine(page: import('@playwright/test').Page) {
	return page.getByTestId('api-response');
}

Then(
	'the endpoint answered {word} with {int} and {string}',
	async ({ page }, method: string, status: number, type: string) => {
		await expect(responseLine(page)).toContainText(`${method} → ${status}`, { timeout: 15_000 });
		await expect(responseLine(page)).toContainText(type);
	}
);

Then(
	'the endpoint said the methods it allows are {string}',
	async ({ page, shot }, allow: string) => {
		await expect(responseLine(page)).toContainText(`Allow: ${allow}`, { timeout: 15_000 });
		await shot();
	}
);

// One of the endpoint's seeded records, in the list it answered with. Nothing
// creates a second copy of a seeded todo, so the count is exact and stays that
// way.
Then('the endpoint returned a todo saying {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByTestId('api-todo').filter({ hasText: text })).toHaveCount(1);
	await shot();
});

/**
 * The record the endpoint said it created, found in the list the endpoint
 * answers with — by its own id, not by its text.
 *
 * By id because the endpoint has no uniqueness rule and the store outlives a
 * run: posting the same sentence twice creates two records, correctly, and a
 * count of the rows saying it goes red on the second run against one server
 * while nothing at all is wrong.
 */
Then('the todo the endpoint created is in its list, saying {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByTestId('api-created')).toBeVisible({ timeout: 15_000 });
	const id = (await page.getByTestId('api-created').textContent())?.replace('created ', '').trim();
	expect(id, 'the endpoint named no record it had created').toBeTruthy();
	await expect(page.locator(`[data-testid="api-todo"][data-id="${id}"]`)).toHaveText(text);
	await shot();
});

When('I POST the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('api-draft').fill(text);
	await page.getByTestId('api-post').click();
});

When('I ask the endpoint to DELETE', async ({ page }) => {
	await page.getByTestId('api-delete').click();
});

/**
 * The header-only half of `the document response came from skgo in the expected
 * mode`. That step also waits for the app's nav, which is right for a page and
 * wrong for a route that answers JSON: there is no Svelte app on the other end
 * of an endpoint.
 */
Then('the document came from skgo in the expected mode', async ({ documents }) => {
	const expected = process.env.EXPECTED_MODE;
	expect(expected, 'EXPECTED_MODE must be set to dev or prod').toBeTruthy();
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.headers()['x-skgo-mode']).toBe(expected);
});

Then('the document response status was {int}', async ({ documents }, status: number) => {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.status()).toBe(status);
});

Then('the document content type was {string}', async ({ documents }, type: string) => {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.headers()['content-type'] ?? '').toContain(type);
});

Then(
	'the document is JSON listing a todo saying {string}',
	async ({ documents, shot }, text: string) => {
		expect(documents.last, 'no document response was observed').not.toBeNull();
		const body = await documents.last!.text();
		const todos = JSON.parse(body) as Array<{ text: string }>;
		expect(todos.map((todo) => todo.text)).toContain(text);
		await shot();
	}
);

When(
	'I request {string} without following redirects',
	async ({ page, notes }, path: string) => {
		const response = await page.request.get(path, { maxRedirects: 0 });
		notes.set('status', response.status());
		// `notes` carries numbers; the location has to go somewhere the next
		// step can read it, and the page is where a screenshot can show it.
		await page.setContent(
			`<pre data-testid="raw-response">${response.status()} ${response.headers()['location'] ?? '(no location)'}</pre>`
		);
	}
);

Then('the response status was {int}', async ({ notes }, status: number) => {
	expect(notes.get('status')).toBe(status);
});

Then('the redirect location was {string}', async ({ page, shot }, location: string) => {
	await expect(page.getByTestId('raw-response')).toContainText(location);
	await shot();
});
