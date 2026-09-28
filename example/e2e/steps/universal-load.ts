import { createBdd } from 'playwright-bdd';
import { booted, expect, test } from './fixtures';

const { When, Then } = createBdd(test);

Then('the invoice view shows the overdue fixture', async ({ page }) => {
	await booted(page);
	await expect(page.getByTestId('invoice-filter')).toHaveText('Overdue');
	await expect(page.getByTestId('invoice')).toHaveCount(1);
	await expect(page.getByTestId('invoice').first()).toContainText('INV-100');
	await expect(page.getByTestId('invoice-total')).toHaveText('Total: 12500 cents');
});

When('I switch to all invoices through Kit navigation', async ({ page }) => {
	await page.getByRole('link', { name: 'All invoices' }).click();
});

Then('the invoice view shows the full fixture without another document or data request', async ({ page, documents, data }) => {
	await expect(page).toHaveURL(/\/invoices$/);
	await expect(page.getByTestId('invoice-filter')).toHaveText('All invoices');
	await expect(page.getByTestId('invoice')).toHaveCount(3);
	await expect(page.getByTestId('invoice').nth(0)).toContainText('INV-200');
	await expect(page.getByTestId('invoice').nth(1)).toContainText('INV-100');
	await expect(page.getByTestId('invoice').nth(2)).toContainText('INV-300');
	await expect(page.getByTestId('invoice-total')).toHaveText('Total: 19900 cents');
	expect(documents.count, documents.log.join(', ')).toBe(1);
	expect(data.count, data.urls.join(', ')).toBe(0);
});
