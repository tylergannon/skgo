import { expect, test } from '@playwright/test';

import { shot } from './helpers';

// Junkyard: e2e/errors.spec.ts
test('expected error renders its status and message', async ({ page }) => {
	const response = await page.goto('/error/expected');
	await page.waitForTimeout(500);
	await shot(page, 'error-expected');
	expect(response?.status()).toBe(418);
	await expect(page.getByTestId('error-status')).toHaveText('418');
	await expect(page.getByTestId('error-message')).toHaveText('expected teapot');
});

test('unexpected error renders a 500 and leaks nothing', async ({ page }) => {
	const response = await page.goto('/error/unexpected');
	await page.waitForTimeout(500);
	await shot(page, 'error-unexpected');
	expect(response?.status()).toBe(500);
	await expect(page.getByTestId('error-status')).toHaveText('500');
	await expect(page.getByTestId('error-message')).toHaveText('Something went wrong on our side.');
});
