import { createBdd } from 'playwright-bdd';
import { booted, expect, test } from './fixtures';

const { When, Then } = createBdd(test);

When('I open the library dialog', async ({ page }) => {
	await booted(page);
	await page.getByRole('button', { name: 'Open library dialog', exact: true }).click();
});

Then('the library dialog is open', async ({ page }) => {
	await expect(page.getByRole('dialog', { name: 'Library dialog', exact: true })).toBeVisible();
	await expect(page.getByTestId('dialog-state')).toHaveText('open');
	await expect(page.getByText('Hydrated Bits UI dialog content.', { exact: true })).toBeVisible();
});

When('I close the library dialog', async ({ page }) => {
	await page.getByRole('button', { name: 'Close library dialog', exact: true }).click();
});

Then('the library dialog is closed', async ({ page }) => {
	await expect(page.getByTestId('dialog-state')).toHaveText('closed');
	await expect(page.getByRole('button', { name: 'Open library dialog', exact: true })).toBeVisible();
	await expect(page.getByRole('dialog', { name: 'Library dialog', exact: true })).toHaveCount(0);
});

Then('the library scroll area can reach its last row', async ({ page }) => {
	const viewport = page.getByTestId('library-scroll-viewport');
	const last = page.getByText('Library row 20', { exact: true });
	await expect(viewport.locator('p')).toHaveCount(20);
	await viewport.hover();
	await page.mouse.wheel(0, 1200);
	await expect.poll(() => viewport.evaluate((element) => element.scrollTop)).toBeGreaterThan(0);
	await expect(last).toBeInViewport();
});
