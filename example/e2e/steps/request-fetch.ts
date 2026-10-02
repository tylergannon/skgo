import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then(
	'the request-fetch page shows {string} fetched as {string}',
	async ({ page }, fact: string, visitor: string) => {
		await expect(page.getByTestId('request-fetch-fact')).toHaveText(fact, { timeout: 15_000 });
		await expect(page.getByTestId('request-fetch-visitor')).toHaveText(`Fetched as ${visitor}`);
	}
);
