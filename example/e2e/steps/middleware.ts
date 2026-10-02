import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

Given('the browser holds the stale visit cookie {string}', async ({ context }, value: string) => {
	await context.addCookies([{ name: 'skgo_visit', value, url: process.env.BASE_URL! }]);
});

Then('the document was authenticated by middleware as {string}', async ({ page, documents }, token: string) => {
	expectMode(documents.last!);
	expect(documents.last!.status()).toBe(200);
	expect(documents.last!.headers()['x-skgo-middleware']).toBe('/middleware data=false');
	await expect(page.getByTestId('title')).toHaveText('Middleware');
	await expect(page.getByTestId('mw-token')).toHaveText(token);
	await expect(page.getByTestId('mw-route')).toHaveText('/middleware');
	await expect(page.getByTestId('mw-cookie')).toHaveText(token);
});

Then('the browser now holds the visit cookie {string}', async ({ context }, value: string) => {
	const cookies = (await context.cookies()).filter((cookie) => cookie.name === 'skgo_visit');
	expect(cookies.map((cookie) => cookie.value)).toEqual([value]);
});

When('I follow the middleware link to {string}', async ({ page }, slug: string) => {
	await hydrated(page);
	await page.getByTestId('mw-alpha').click();
	await expect(page).toHaveURL(new RegExp(`/middleware/${slug}$`));
});

Then(
	'the page shows slug {string} authenticated as {string} from a data request',
	async ({ page }, slug: string, token: string) => {
		await expect(page.getByTestId('title')).toHaveText(`Middleware ${slug}`);
		await expect(page.getByTestId('mw-slug-param')).toHaveText(slug);
		await expect(page.getByTestId('mw-slug-token')).toHaveText(token);
		await expect(page.getByTestId('mw-slug-route')).toHaveText('/middleware/[slug]');
		await expect(page.getByTestId('mw-slug-data')).toHaveText('data request');
	}
);

Then('the data response was marked {string}', async ({ data }, marker: string) => {
	expect(data.last).not.toBeNull();
	expect(data.last!.headers()['x-skgo-middleware']).toBe(marker);
});

type Submission = 'Kit enhancement' | 'native form';

When(/^I save the middleware note with (Kit enhancement|native form)$/, async ({ page, notes }, submission: Submission) => {
	const enhanced = submission === 'Kit enhancement';
	if (enhanced) await hydrated(page);
	const pending = page.waitForResponse(
		(response) => response.request().method() === 'POST' && new URL(response.url()).pathname === '/middleware'
	);
	await page.getByTestId(`${enhanced ? 'enhanced' : 'native'}-note-form`).getByRole('button').click();
	const response = await pending;
	expectMode(response);
	expect(response.status()).toBe(200);
	expect(response.request().headers()['x-sveltekit-action'] === 'true').toBe(enhanced);
	expect(response.request().resourceType() === 'document').toBe(!enhanced);
	expect(response.headers()['x-skgo-middleware']).toBe('/middleware data=false');
	notes.set('middleware-submission', 1);
});

Then(/^the middleware answered the (?:enhanced|native) submission$/, async ({ notes }) => {
	expect(notes.get('middleware-submission')).toBe(1);
});

Then('the page shows the receipt {string}', async ({ page }, receipt: string) => {
	await expect(page.getByTestId('mw-receipt')).toHaveText(receipt);
});

Then('the document was transformed by middleware and kit still hydrated it', async ({ page }) => {
	await hydrated(page);
	await expect(page.locator('html')).toHaveAttribute('data-middleware-transformed', 'yes');
});
