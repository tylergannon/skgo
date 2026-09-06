import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

When('the todo list has loaded', async ({ page }) => {
	// Both the list query and the live-count stream must have gone out before a
	// scenario starts counting remote requests.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible();
	await expect(page.getByTestId('count')).toBeVisible();
});

When('I note the remote request count', async ({ remotes }) => {
	remotes.mark();
});

When('I add the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('new-todo').fill(text);
	await page.getByTestId('add-todo').click();
});

When('the todo detail has loaded', async ({ page }) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).not.toBeEmpty();
});

When('I open the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('todos').getByRole('link', { name: text }).click();
});

When('I rename the open todo to {string}', async ({ page }, text: string) => {
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

Then('the live count is the number of todos I can see', async ({ page }) => {
	// The count is the one number on this page a visitor cannot check against
	// anything else, which is exactly why it was the one that leaked: it used
	// to report the total, telling a signed-out visitor a todo they may not
	// read exists. Held to the list, it cannot say that again.
	const listed = await page.getByTestId('todo').count();
	await expect(page.getByTestId('count')).toHaveText(String(listed), { timeout: 15_000 });
});
