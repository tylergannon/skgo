import { createBdd } from 'playwright-bdd';
import type { Page } from '@playwright/test';
import { expect, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

When('the todo list has loaded', async ({ page }) => {
	// Both the list query and the live-count stream must have gone out before a
	// scenario starts counting remote requests. The stream is opened while the
	// page hydrates, so this has to outlast hydration: in dev the document is
	// rendered and on screen long before the browser has the modules that make
	// it live, and a count taken in that window catches the stream opening and
	// blames the scenario's own interaction for it.
	await hydrated(page);
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible();
	await expect(page.getByTestId('count')).toBeVisible();
});

// Kit evicts a query's cache entry when the last proxy for it is garbage
// collected, so how long a request is in flight decides whether a second
// proxy finds the entry. A held response is how a scenario makes that
// deterministic instead of waiting for a collection to land in the window.
Given('every remote response is held for {int} ms', async ({ page }, ms: number) => {
	await page.route('**/_app/remote/**', async (route) => {
		await new Promise((resolve) => setTimeout(resolve, ms));
		await route.continue();
	});
});

When('I note the remote request count', async ({ remotes }) => {
	remotes.mark();
});

When('I add the todo {string}', async ({ page }, text: string) => {
	await hydrated(page);
	await page.getByTestId('new-todo').fill(text);
	await page.getByTestId('add-todo').click();
});

When('the todo detail has loaded', async ({ page }) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).not.toBeEmpty();
});

When('I open the todo {string}', async ({ page }, text: string) => {
	await hydrated(page);
	await page.getByTestId('todos').getByRole('link', { name: text }).click();
});

When('I rename the open todo to {string}', async ({ page }, text: string) => {
	await hydrated(page);
	await page.getByTestId('rename-todo').fill(text);
	await page.getByTestId('save-todo').click();
});

Then('I do not see the todo {string}', async ({ page }, text: string) => {
	// An absence is only evidence when the thing that would have shown it is
	// working. A crashed query renders the boundary's `failed` snippet and no
	// todos at all, and a bare count-of-zero would call that a pass.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible({ timeout: 15_000 });
	await expect(page.getByTestId('todo').filter({ hasText: text })).toHaveCount(0, {
		timeout: 15_000
	});
});

Then('I see the todo {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('todo').filter({ hasText: text }).first()).toBeVisible({
		timeout: 15_000
	});
});

Then('the todo detail shows {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).toHaveText(text, {
		timeout: 15_000
	});
});

Then('the item load and component both show {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('load-item-name')).toHaveText(text);
	await expect(page.getByTestId('item-name')).toHaveText(text);
});

Then('no remote request was made during hydration', async ({ page, remotes }) => {
	await hydrated(page);
	await page.waitForTimeout(500);
	expect(remotes.count, `remote requests: ${remotes.urls.join(', ') || 'none'}`).toBe(0);
});

Then(
	'exactly {int} remote request was made since',
	async ({ page, remotes }, expected: number) => {
		// A second, non-single-flight round trip would already be in flight by
		// now; wait a beat so it lands and can be counted.
		await page.waitForTimeout(500);
		expect(remotes.since, `remote requests: ${remotes.urlsSince.join(', ') || 'none'}`).toBe(
			expected
		);
	}
);

// A number the scenario wrote down, not one read off the page earlier: the
// visitor's list starts from the store's fixtures, so what the count should be
// is known before the page is opened.
Then('the live count is {int}', async ({ page }, expected: number) => {
	await expectLiveCount(page, expected);
});

// The number on screen is the live query's answer; the list beside it is the
// `getTodos` query's answer. They are two different endpoints, so asserting one
// against the other is an assertion about two real numbers, and it is the thing
// a signed-out visitor can check with their own eyes.
Then('the todo count is the number of todos on the page', async ({ page }) => {
	await countMatchesList(page);
});

// The same check after a command has driven the live query.
Then('the todo count still matches the todos on the page', async ({ page }) => {
	await countMatchesList(page);
});

async function countMatchesList(page: Page): Promise<void> {
	// Zero against zero is not agreement, it is two broken components agreeing
	// about nothing. Both halves have to be on screen first.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible({ timeout: 15_000 });
	await expect(async () => {
		const listed = await page.getByTestId('todo').count();
		const counted = await liveCount(page);
		expect(counted, `the counter says ${counted}; the page lists ${listed} todos`).toBe(listed);
	}).toPass({ timeout: 15_000 });
}

async function expectLiveCount(page: Page, expected: number): Promise<void> {
	await expect(page.getByTestId('count')).toHaveText(String(expected), { timeout: 15_000 });
}

async function liveCount(page: Page): Promise<number> {
	const text = await page.getByTestId('count').innerText();
	const value = Number(text.trim());
	expect(Number.isFinite(value), `live count was ${JSON.stringify(text)}`).toBe(true);
	return value;
}
