import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

Given('I open {string}', async ({ page, documents }, path: string) => {
	// Touching `documents` here guarantees the listeners are attached before
	// the first navigation.
	expect(documents.count).toBe(0);
	await page.goto(path);
});

When('I click the link to {string}', async ({ page }, path: string) => {
	await page.getByRole('link', { name: linkName(path) }).click();
});

Then('the document response came from skgo in the expected mode', async ({ documents }) => {
	const expected = process.env.EXPECTED_MODE;
	expect(expected, 'EXPECTED_MODE must be set to dev or prod').toBeTruthy();
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.headers()['x-skgo-mode']).toBe(expected);
});

Then('I see the greeting component', async ({ page }) => {
	await expect(page.getByTestId('greeting')).toHaveText(
		'Hello, skgo! This is Svelte, served by Go.'
	);
});

Then('I see {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('title')).toHaveText(text);
});

Then('exactly {int} document request was made', async ({ documents }, expected: number) => {
	expect(documents.count).toBe(expected);
});

function linkName(path: string): string {
	if (path === '/') return 'Home';
	if (path === '/about') return 'About';
	if (path === '/items/42') return 'Item 42';
	if (path === '/pricing') return 'Pricing';
	if (path === '/docs/guide/getting-started') return 'Docs';
	throw new Error(`no nav link for ${path}`);
}
