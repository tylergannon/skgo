import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * Every step here asserts a count the page computed with `.length` on a value
 * Go sent, beside the empty state the same value produced. The pair matters:
 * the count alone would pass over a list that rendered nothing, and the empty
 * state alone would pass over a page that never got a value at all.
 */

Then('the report is titled {string}', async ({ page, shot }, title: string) => {
	await expect(page.getByTestId('report-title')).toHaveText(title);
	await shot();
});

Then(
	'the report shows {int} diagnostics and the empty state {string}',
	async ({ page, shot }, count: number, empty: string) => {
		await expect(page.getByTestId('report-count')).toHaveText(`${count} diagnostics`);
		await expect(page.getByTestId('report-empty')).toHaveText(empty);
		await expect(page.getByTestId('report-diagnostic')).toHaveCount(count);
		await shot();
	}
);

Then(
	'the models list shows {int} models and the empty state {string}',
	async ({ page, shot }, count: number, empty: string) => {
		await expect(page.getByTestId('models-count')).toHaveText(`${count} models`);
		await expect(page.getByTestId('models-empty')).toHaveText(empty);
		await expect(page.getByTestId('model')).toHaveCount(count);
		await shot();
	}
);

Then(
	"the load's notes show {int} notes and the empty state {string}",
	async ({ page, shot }, count: number, empty: string) => {
		await expect(page.getByTestId('notes-count')).toHaveText(`${count} notes`);
		await expect(page.getByTestId('notes-empty')).toHaveText(empty);
		await expect(page.getByTestId('note')).toHaveCount(count);
		await shot();
	}
);

// The control. The rows are named in the feature file, so this cannot be
// satisfied by whatever the page happened to render.
Then(
	'the report titled {string} lists {string} and {string}',
	async ({ page, shot }, title: string, first: string, second: string) => {
		await expect(page.getByTestId('known-title')).toHaveText(title);
		await expect(page.getByTestId('known-count')).toHaveText('2 diagnostics');
		await expect(page.getByTestId('known-diagnostic')).toHaveText([first, second]);
		await shot();
	}
);

When('I press reparse', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('reparse').click();
});

Then('the reparse result reads {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByTestId('reparse-count')).toHaveText(text);
	await shot();
});
