import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * Every step here asserts a count the page computed with `.length` on a value
 * Go sent, beside the empty state the same value produced. The pair matters:
 * the count alone would pass over a list that rendered nothing, and the empty
 * state alone would pass over a page that never got a value at all.
 */

Then(
	'the report shows {int} diagnostics and the empty state {string}',
	async ({ page }, count: number, empty: string) => {
		await expect(page.getByTestId('report-count')).toHaveText(`${count} diagnostics`);
		await expect(page.getByTestId('report-empty')).toHaveText(empty);
		await expect(page.getByTestId('report-diagnostic')).toHaveCount(count);
	}
);

Then(
	'the models list shows {int} models and the empty state {string}',
	async ({ page }, count: number, empty: string) => {
		await expect(page.getByTestId('models-count')).toHaveText(`${count} models`);
		await expect(page.getByTestId('models-empty')).toHaveText(empty);
		await expect(page.getByTestId('model')).toHaveCount(count);
	}
);

When('I press reparse', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('reparse').click();
});

Then('the reparse result reads {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('reparse-count')).toHaveText(text);
});
