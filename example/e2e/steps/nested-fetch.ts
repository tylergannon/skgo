import { createBdd, type DataTable } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

// Kit's native dev renderer declares text/html; the embedded production
// handler includes its charset. Keep both expected wire values explicit.
function destinationAnswer(answer: string): string {
	return process.env.SKGO_E2E_MODE === 'dev' && answer === '200 text/html; charset=utf-8'
		? '200 text/html'
		: answer;
}

Then('the document already lists destination {string} as {string}', async ({ documents }, path: string, answer: string) => {
	const html = await documents.last!.text();
	expect(html).toContain(`data-path="${path}">${destinationAnswer(answer)}</li>`);
});

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
			await expect(page.locator(`li[data-path="${path}"]`)).toContainText(destinationAnswer(answer), {
				timeout: 15_000
			});
		}
	}
);
