import { createBdd } from 'playwright-bdd';
import { booted, expect, test } from './fixtures';

const { Then } = createBdd(test);

Then('the entry receipt is {string}', async ({ page }, receipt: string) => {
	await expect(page.getByTestId('entry-receipt')).toHaveText(receipt);
});

Then('the prerendered Go values are visible', async ({ page, remotes }) => {
	await booted(page);
	await expect(page.getByTestId('prerender-parent')).toHaveText('skgo example');
	await expect(page.getByTestId('prerender-price')).toHaveText('$7.50');
	await expect(page.getByTestId('prerender-deferred')).toHaveText('GO_PRERENDER_DEFERRED');
	await expect(page.getByTestId('prerender-remote')).toHaveText('Go prerender remote: atlas');
	expect(remotes.urls.filter((url) => url.includes('/buildReceipt'))).toEqual([]);
});
