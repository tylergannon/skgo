import { createBdd } from 'playwright-bdd';
import type { Locator, Page } from '@playwright/test';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

When('the gate page has loaded', async ({ page }) => {
	await expect(page.getByTestId('gate-panel')).toHaveCount(3, { timeout: 15_000 });
	await expect(page.locator('[data-testid$="-pending"]')).toHaveCount(0, { timeout: 15_000 });
});

When(
	'I write left {string}, right {string} and banner {string}',
	async ({ page }, left: string, right: string, banner: string) => {
		await hydrated(page);
		await page.getByTestId('gate-left').fill(left);
		await page.getByTestId('gate-right').fill(right);
		await page.getByTestId('gate-banner').fill(banner);
		await page.getByTestId('gate-write').click();
		// The button is re-enabled in the handler's `finally`, which runs after
		// the command has resolved and its single-flight updates have been
		// applied — so this is the moment the page is showing the answer.
		await expect(page.getByTestId('gate-write')).toBeEnabled({ timeout: 15_000 });
	}
);

When('I reload all three panels', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('gate-reload').click();
	await expect(page.getByTestId('gate-reload')).toBeEnabled({ timeout: 15_000 });
});

Then('the panel {string} shows {string}', async ({ page }, id: string, text: string) => {
	await expect(panel(page, id).getByTestId(`note-${id}`)).toHaveText(text, { timeout: 15_000 });
});

// The same assertion said the other way round, so the frame this step leaves
// behind is named for what the sentence claims: this panel was not refreshed
// and still reads what an earlier step in this scenario put there.
Then('the panel {string} still shows {string}', async ({ page }, id: string, text: string) => {
	await expect(panel(page, id).getByTestId(`note-${id}`)).toHaveText(text, { timeout: 15_000 });
});

Then('the panel {string} is refused with {string}', async ({ page }, id: string, text: string) => {
	await expect(panel(page, id).getByTestId(`note-refused-${id}`)).toHaveText(text, {
		timeout: 15_000
	});
});

Then('the command reported writing {int} values', async ({ page }, count: number) => {
	await expect(page.getByTestId('gate-wrote')).toHaveText(String(count), { timeout: 15_000 });
});

function panel(page: Page, id: string): Locator {
	return page.locator(`[data-testid="gate-panel"][data-id="${id}"]`);
}
