import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then('the plans are {string}', async ({ page }, names: string) => {
	const expected = names.split(', ');
	await expect(page.getByTestId('plan')).toHaveCount(expected.length, { timeout: 15_000 });
	const plans = await page.getByTestId('plan').allTextContents();
	expect(plans.map((plan) => plan.split(' — ')[0])).toEqual(expected);
});

Then('the doc is titled {string}', async ({ page }, title: string) => {
	await expect(page.getByTestId('doc-title')).toHaveText(title);
});

Then('the doc is {int} segments deep', async ({ page }, depth: number) => {
	await expect(page.getByTestId('doc-depth')).toHaveText(String(depth));
});

Then('the site is named {string}', async ({ page }, name: string) => {
	await expect(page.getByTestId('site-name')).toHaveText(name);
});
