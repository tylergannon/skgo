import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then(
	'the document displays the reserved query parameter error for {string}',
	async ({ page, shot }, name: string) => {
		await expect(page.locator('body')).toHaveText(`Cannot use reserved query parameter "${name}"`);
		await shot();
	}
);
