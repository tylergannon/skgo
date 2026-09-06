import { expect, test } from '@playwright/test';

import { recordRequests, shot, waitForHydration } from './helpers';

// Junkyard: e2e/routing.spec.ts
test('home page is server-rendered', async ({ request }) => {
	const response = await request.get('/');
	expect(response.status()).toBe(200);
	expect(await response.text()).toContain('hello from the server');
});

test('hydration does not refetch, client navigation does not reload', async ({ page }) => {
	const requests = recordRequests(page);
	await page.goto('/');
	await waitForHydration(page);
	await shot(page, 'routing-home-hydrated');

	expect(requests.data()).toHaveLength(0);
	expect(requests.documents()).toHaveLength(1);

	await page.getByRole('link', { name: 'Who am I' }).click();
	await expect(page).toHaveURL(/\/whoami$/);
	await shot(page, 'routing-whoami-after-nav');
	await expect(page.getByTestId('request-id')).not.toBeEmpty();

	expect(requests.documents()).toHaveLength(1);
	expect(requests.data().length).toBeGreaterThan(0);
});

test('deep link and client navigation render the same dynamic page', async ({ page }) => {
	await page.goto('/messages/1');
	await waitForHydration(page);
	await shot(page, 'message-deep-link');
	const deepLinked = await page.getByTestId('message-text').textContent();

	await page.goto('/messages');
	await waitForHydration(page);
	await page.getByRole('link', { name: '#1' }).click();
	await expect(page).toHaveURL(/\/messages\/1$/);
	await shot(page, 'message-client-nav');
	await expect(page.getByTestId('message-text')).toHaveText(deepLinked!);
});

test('unknown route renders the 404 error page', async ({ page }) => {
	const response = await page.goto('/does-not-exist');
	await page.waitForTimeout(300);
	await shot(page, 'unknown-route');
	expect(response?.status()).toBe(404);
	await expect(page.getByTestId('error-status')).toHaveText('404');
});

test('unknown message id is a 404 with the app message', async ({ page }) => {
	const response = await page.goto('/messages/999');
	await page.waitForTimeout(500);
	await shot(page, 'unknown-message-id');
	expect(response?.status()).toBe(404);
	await expect(page.getByTestId('error-message')).toHaveText('no message #999');
});

test('prerendered page is served and trailing slash redirects', async ({ page, request }) => {
	await page.goto('/about');
	await waitForHydration(page);
	await shot(page, 'about-prerendered');
	await expect(page.getByTestId('built-at')).toHaveText('This page was rendered at build time.');

	const slash = await request.get('/about/', { maxRedirects: 0 });
	expect(slash.status()).toBe(308);
	expect(slash.headers()['location']).toMatch(/about$/);
});

test('redirect thrown in a load lands on the destination', async ({ page }) => {
	await page.goto('/redirect');
	await page.waitForTimeout(1000);
	await shot(page, 'redirect-landing');
	await expect(page).toHaveURL(/\/messages$/);
	await expect(page.getByRole('heading', { name: 'Messages' })).toBeVisible();
});
