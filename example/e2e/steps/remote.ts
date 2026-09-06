import { createBdd } from 'playwright-bdd';
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

When('the todo list has loaded', async ({ page }) => {
	// Both the list query and the live-count stream must have gone out before a
	// scenario starts counting remote requests.
	await expect(page.getByTestId('todo').first()).toBeVisible();
	await expect(page.getByTestId('count')).toBeVisible();
});

When('I note the remote request count', async ({ remotes }) => {
	remotes.mark();
});

When('I note the live count', async ({ page, notes }) => {
	notes.set('live count', await liveCount(page));
});

When('I add the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('new-todo').fill(text);
	await page.getByTestId('add-todo').click();
});

When('the todo detail has loaded', async ({ page }) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).not.toBeEmpty();
});

When('I rename the open todo to {string}', async ({ page }, text: string) => {
	await page.getByTestId('rename-todo').fill(text);
	await page.getByTestId('save-todo').click();
});

Then('I do not see the todo {string}', async ({ page }, text: string) => {
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

Then('the live count increased by {int}', async ({ page, notes }, by: number) => {
	const before = notes.get('live count');
	expect(before, 'the live count was never noted').not.toBeUndefined();
	await expect(page.getByTestId('count')).toHaveText(String(before! + by), { timeout: 15_000 });
});

async function liveCount(page: Page): Promise<number> {
	const text = await page.getByTestId('count').innerText();
	const value = Number(text.trim());
	expect(Number.isFinite(value), `live count was ${JSON.stringify(text)}`).toBe(true);
	return value;
}
