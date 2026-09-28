import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { Then } = createBdd(test);

Then('the prerendered Go values are visible', async ({ page }) => {
	await hydrated(page);
	await expect(page.getByTestId('prerender-parent')).toHaveText('skgo example');
	await expect(page.getByTestId('prerender-price')).toHaveText('$7.50');
	await expect(page.getByTestId('prerender-deferred')).toHaveText('GO_PRERENDER_DEFERRED');
});
