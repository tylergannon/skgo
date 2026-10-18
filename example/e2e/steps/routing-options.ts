import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

Then('the routing receipt is {string}', async ({ page }, receipt: string) => {
  await expect(page.getByTestId('routing-receipt')).toHaveText(receipt);
});

Then('the routing URL is {string}', async ({ page }, path: string) => {
  await expect.poll(() => new URL(page.url()).pathname).toBe(path);
});
