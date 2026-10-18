import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';
const { Then } = createBdd(test);

Then('the reroute receipt is {string}', async ({ page }, receipt: string) => {
 await expect(page.getByTestId('reroute-receipt')).toHaveText(receipt);
});
Then('the reroute URL is {string}', async ({ page }, value: string) => {
 await expect.poll(() => {const url=new URL(page.url());return url.pathname+url.search;}).toBe(value);
});
