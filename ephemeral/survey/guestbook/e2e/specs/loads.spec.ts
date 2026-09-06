import { expect, test } from '@playwright/test';

import { shot, waitForHydration } from './helpers';

// Junkyard: e2e/loads.spec.ts — a `+page.server.ts` load returning an
// unawaited promise. Ported to a universal load awaiting a Go query.
test('streamed load shows the fast part before the slow part arrives', async ({ page }) => {
	await page.goto('/slow', { waitUntil: 'commit' });
	await expect(page.getByTestId('fast')).toHaveText('fast part');
	await expect(page.getByTestId('slow')).toHaveText('loading…');
	await shot(page, 'slow-pending');
	await expect(page.getByTestId('slow')).toHaveText('slow part', { timeout: 5000 });
	await shot(page, 'slow-resolved');
});

// Junkyard: `+layout.server.ts` set a `visits` cookie on every request and
// composed with the page load.
test('layout load composes with page loads and persists a cookie', async ({ page }) => {
	await page.goto('/');
	await waitForHydration(page);
	await shot(page, 'home-first-visit');
	await expect(page.getByTestId('visits')).toHaveText('1');
	await expect(page.getByTestId('greeting')).toHaveText('hello from the server');

	await page.reload();
	await waitForHydration(page);
	await shot(page, 'home-second-visit');
	await expect(page.getByTestId('visits')).toHaveText('2');
});

// Junkyard: a `transport` codec carried a `Money` instance through a load and
// a query.
test('custom transport codec round-trips through a load and a query', async ({ page }) => {
	await page.goto('/prices');
	await waitForHydration(page);
	await shot(page, 'prices');
	await expect(page.getByTestId('price')).toHaveText('USD 19.99');
	await expect(page.getByTestId('price-is-money')).toHaveText('Money');
	await expect(page.getByTestId('sale-price')).toHaveText('USD 14.99');
	await expect(page.getByTestId('sale-price-is-money')).toHaveText('Money');
});
