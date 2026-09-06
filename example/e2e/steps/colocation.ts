import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then('the item is named {string}', async ({ page }, name: string) => {
	await expect(page.getByTestId('item-name')).toHaveText(name);
});

// Every colocated page renders the path of the Go file that answered it, so a
// scenario can say which directory the answer came out of rather than trusting
// that it came out of the right one.
Then('the answer came from {string}', async ({ page }, source: string) => {
	await expect(page.getByTestId('colocated')).toHaveText(source);
});

Then('the plans are {string}', async ({ page }, names: string) => {
	const plans = await page.getByTestId('plan').allTextContents();
	expect(plans.map((plan) => plan.split(' — ')[0])).toEqual(names.split(', '));
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
