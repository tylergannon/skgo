import { createBdd } from 'playwright-bdd';
import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

When('the pair of todos has loaded', async ({ page }) => {
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('pair-panel')).toHaveCount(2, { timeout: 15_000 });
	for (const id of ['p1', 'p2']) {
		await expect(panelText(page, id)).not.toBeEmpty({ timeout: 15_000 });
	}
});

When(
	'I retitle {string} to {string} and refresh {string}',
	async ({ page }, id: string, text: string, refreshId: string) => {
		await page.getByTestId('retitle-id').fill(id);
		await page.getByTestId('retitle-text').fill(text);
		await page.getByTestId('retitle-refresh').fill(refreshId);

		// The command's response is the only thing that can move either panel,
		// so the step is not over until it has arrived — otherwise the next
		// step reads the page before the answer it is about.
		const landed = page.waitForResponse(
			(response) =>
				response.request().method() === 'POST' && response.url().includes('/retitleTodo')
		);
		await page.getByTestId('retitle-save').click();
		await landed;
		// And a beat for the client to apply what came back, because half of
		// what these scenarios assert is that a panel did *not* move.
		await page.waitForTimeout(300);
	}
);

Then('the pair panel {string} shows {string}', async ({ page }, id: string, text: string) => {
	await expect(panelText(page, id)).toHaveText(text, { timeout: 15_000 });
});

// The same assertion said the other way round, so the frame the suite leaves
// behind is named for what the sentence claims: this panel was not refreshed
// and still reads what an earlier step in this scenario put there.
Then('the pair panel {string} still shows {string}', async ({ page }, id: string, text: string) => {
	await expect(panelText(page, id)).toHaveText(text, { timeout: 15_000 });
});

function panelText(page: Page, id: string): Locator {
	return page.locator(`[data-testid="pair-panel"][data-id="${id}"]`).getByTestId('pair-text');
}
