import { expect, test } from '@playwright/test';

import { recordRequests, shot, waitForHydration } from './helpers';

// Junkyard: e2e/forms.spec.ts — classic `+page.server.ts` form actions with
// `use:enhance`. skgo has no form actions; the flow is hand-wired onto Go
// commands, so this measures whether the *capability* survives the port.
async function loginFlow(page: import('@playwright/test').Page, tag: string) {
	await page.goto('/login');
	await expect(page.getByTestId('login-state')).toHaveText('signed out');

	await page.getByTestId('login').click();
	await page.waitForTimeout(300);
	await shot(page, `login-validation-${tag}`);
	await expect(page.getByTestId('login-error')).toHaveText('name is required');
	await expect(page).toHaveURL(/\/login/);

	await page.getByTestId('name').fill('tyler');
	await page.getByTestId('login').click();
	await expect(page).toHaveURL(/\/$/);
	await shot(page, `login-signed-in-${tag}`);
	await expect(page.getByTestId('user')).toHaveText('tyler');

	await page.goto('/whoami');
	await shot(page, `login-whoami-${tag}`);
	await expect(page.getByTestId('whoami-user')).toHaveText('tyler');

	await page.goto('/login');
	await expect(page.getByTestId('login-state')).toHaveText('signed in as tyler');
	await page.getByTestId('logout').click();
	await expect(page.getByTestId('logged-out')).toBeVisible();
	await shot(page, `login-signed-out-${tag}`);
	await expect(page.getByTestId('user')).toHaveText('anonymous');
}

test('classic form actions: validation failure, login redirect, logout', async ({ page }) => {
	await page.goto('/login');
	await waitForHydration(page);
	const requests = recordRequests(page);
	await loginFlow(page, 'js');
	expect(requests.documents().map((r) => r.url)).toEqual(
		expect.arrayContaining([expect.stringMatching(/\/whoami$/), expect.stringMatching(/\/login$/)])
	);
});

test.describe('without JavaScript', () => {
	test.use({ javaScriptEnabled: false });

	test('classic form actions work as plain HTML forms', async ({ page }) => {
		await page.goto('/login');
		await shot(page, 'login-nojs');
		await loginFlow(page, 'nojs');
	});
});
