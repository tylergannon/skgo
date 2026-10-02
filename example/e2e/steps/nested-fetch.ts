import { createBdd, type DataTable } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then(
	'the nested page shows {string} for {string} with status {int}',
	async ({ page }, fact: string, visitor: string, status: number) => {
		await expect(page.getByTestId('nested-fact')).toHaveText(fact, { timeout: 15_000 });
		await expect(page.getByTestId('nested-visitor')).toHaveText(visitor);
		await expect(page.getByTestId('nested-status')).toHaveText(String(status));
	}
);

Then(
	'the destinations page lists these answers:',
	async ({ page }, table: DataTable) => {
		for (const [path, answer] of table.rows()) {
			await expect(page.locator(`li[data-path="${path}"]`)).toContainText(answer, {
				timeout: 15_000
			});
		}
	}
);
