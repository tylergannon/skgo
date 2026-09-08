import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

Given('I open {string}', async ({ page, documents }, path: string) => {
	// Touching `documents` here guarantees the listeners are attached before
	// the first navigation.
	expect(documents.count).toBe(0);
	await page.goto(path);
});

// The root layout's nav, by test id. The front page is an index of every
// capability this app has, so a link named for a page exists twice on it — once
// in the nav and once in the index — and an unscoped `getByRole` matches both.
When('I click the link to {string}', async ({ page }, path: string) => {
	// Kit's router has to be the thing that answers the click. Go renders the
	// document in dev too, so the nav is on screen and clickable before the
	// browser has the modules that make it live, and a click in that window is
	// an ordinary anchor: a second document, which is the opposite of what
	// every scenario using this step goes on to claim.
	await hydrated(page);
	await page.getByTestId('app-nav').getByRole('link', { name: linkName(path), exact: true }).click();
});

Then('the document response came from skgo in the expected mode', async ({ page, documents }) => {
	const expected = process.env.EXPECTED_MODE;
	expect(expected, 'EXPECTED_MODE must be set to dev or prod').toBeTruthy();
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.headers()['x-skgo-mode']).toBe(expected);
	// A document that boots nothing is not evidence skgo served the app, and
	// the frame this step leaves behind would be a blank page — which is what
	// it was in dev, where the first load waits on vite's module graph.
	//
	// The root layout's own nav, by test id rather than by role: a section
	// layout may have a nav of its own, and `getByRole('navigation')` then
	// matches two elements and fails on a page that rendered perfectly.
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
});

Then('every part of the page loaded', async ({ page }) => {
	// Every remote call on a page sits inside a `<svelte:boundary>` that renders
	// a `*-pending` test id while it is in flight and a `*-failed` one when it
	// throws. Both have to be gone.
	//
	// Failed, because a scenario that only looks for what it expects to see will
	// happily pass beside a broken component: the todo list renders while the
	// count above it reads "Error: 404". Pending, because this step ran first in
	// one scenario and passed against a page still showing "loading…" — the
	// frame it left behind was a picture of nothing having happened yet.
	// The app itself has to be on screen first. Both halves below are
	// count-is-zero assertions, and an empty page satisfies them perfectly: in
	// dev, where the first paint waits on vite's module graph, this step passed
	// twice against a page that had rendered nothing at all and left two blank
	// frames behind.
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
	await expect(page.locator('[data-testid$="-pending"]')).toHaveCount(0, { timeout: 15_000 });
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
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
	if (path === '/empty') return 'Empty';
	if (path === '/docs/guide/getting-started') return 'Docs';
	if (path === '/live') return 'Live';
	if (path === '/batch') return 'Batch';
	throw new Error(`no nav link for ${path}`);
}
