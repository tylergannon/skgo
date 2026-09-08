import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

When('I sign in as {string}', async ({ page }, user: string) => {
	await hydrated(page);
	await page.getByTestId('user').fill(user);
	await page.getByTestId('sign-in').click();
});

When('I sign out', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('sign-out').click();
});

When('I reload the page', async ({ page }) => {
	await page.reload();
});

Then('I am signed in as {string}', async ({ page }, user: string) => {
	await expect(page.getByTestId('session')).toHaveText(`Signed in as ${user}`, {
		timeout: 15_000
	});
});

Then('I am signed out', async ({ page }) => {
	await expect(page.getByTestId('session')).toHaveText('Signed out', { timeout: 15_000 });
});
