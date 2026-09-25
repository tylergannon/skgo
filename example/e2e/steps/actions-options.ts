import { createBdd } from 'playwright-bdd';
import type { Page, Response } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { expect, expectMode, expectedMode, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);
const lastPost = new WeakMap<Page, Response>();
const paths = {
	client: '/actions/options/no-client',
	ssr: '/actions/options/no-ssr'
};
type Outcome = 'success' | 'validation' | 'forbidden' | 'unavailable';

async function post(page: Page, path: string, submit: () => Promise<void>, native = true, bootData = false): Promise<Response> {
	const response = page.waitForResponse((r) => r.request().method() === 'POST' && new URL(r.url()).pathname === path);
	const documentLoaded = native ? page.waitForEvent('domcontentloaded') : null;
	const dataLoaded = bootData ? page.waitForResponse((r) => r.url().includes('/__data.json')) : null;
	await submit();
	const answer = await response;
	if (documentLoaded) await documentLoaded;
	if (dataLoaded) await dataLoaded;
	expectMode(answer);
	lastPost.set(page, answer);
	return answer;
}

function posted(page: Page): Response {
	const response = lastPost.get(page);
	expect(response, 'a native or enhanced POST should have been observed').toBeDefined();
	return response!;
}

function actionStatus(outcome: Outcome): number {
	return outcome === 'success' ? 200 : outcome === 'validation' ? 422 : outcome === 'forbidden' ? 403 : 500;
}

function assertNative(response: Response) {
	expect(response.request().resourceType()).toBe('document');
	expect(response.request().headers()['x-sveltekit-action']).toBeUndefined();
}

async function assertShell(response: Response, outcome: Outcome) {
	assertNative(response);
	expect(response.status()).toBe(actionStatus(outcome));
	const body = await response.text();
	expect(body.toLowerCase()).toContain('<!doctype html>');
	expect(body.includes('rel="stylesheet"') || body.includes('<style data-sveltekit>')).toBe(true);
	expect(body).toContain('<script');
	expect(body).not.toContain('Saved Grace Hopper');
	expect(body).not.toContain('Enter a valid email address');
	expect(body).not.toContain('Action error 403');
	expect(body).not.toContain('Action error 500');
	expect(body).not.toContain('data-testid="no-ssr-title"');
	if (expectedMode() === 'dev') {
		const log = readFileSync(process.env.SKGO_LOG!, 'utf8');
		const warning = outcome === 'forbidden' || outcome === 'unavailable'
			? "The form action returned an error, but +error.svelte wasn't rendered because SSR is off."
			: "The form action returned a value, but it isn't available in `page.form`, because SSR is off.";
		expect(log).toContain(warning);
	}
}

Given('I open the Actions page without client JavaScript', async ({ page }) => {
	const response = await page.goto(paths.client);
	expectMode(response!);
	await expect(page.getByTestId('no-client-title')).toHaveText('Actions without client JavaScript');
	await expect(page.getByTestId('no-client-saved-name')).toHaveText('Ada Lovelace');
	await expect(page.getByTestId('no-client-saved-email')).toHaveText('ada@example.test');
	await expect(page.getByTestId('no-client-saved-biography')).toHaveText('First programmer');
});

When('I save Grace on the no-client page', async ({ page }) => {
	const form = page.getByTestId('no-client-form');
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill('grace@example.test');
	await form.getByRole('textbox', { name: 'Biography' }).fill('Compiler pioneer');
	await post(page, paths.client, () => form.getByRole('button', { name: 'Save profile' }).click());
});

Then('the no-client response is a script-free 200 receipt with the exact saved Grace profile', async ({ page }) => {
	const response = posted(page);
	assertNative(response);
	expect(response.status()).toBe(200);
	const body = await response.text();
	expect(body).toContain('Saved Grace Hopper');
	expect(body).not.toContain('<script');
	await expect(page.getByTestId('no-client-status')).toHaveText('Page status 200');
	await expect(page.getByTestId('no-client-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('no-client-saved-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('no-client-saved-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('no-client-saved-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('no-client-saved-state')).toHaveText('Active');
});

When('I submit the invalid Grace edit on the no-client page', async ({ page }) => {
	const form = page.getByTestId('no-client-form');
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill('grace-at-example');
	await form.getByRole('textbox', { name: 'Biography' }).fill('Keep this biography');
	await post(page, paths.client, () => form.getByRole('button', { name: 'Save profile' }).click());
});

Then('the no-client response is script-free 422 with the exact edit and unchanged Ada profile', async ({ page }) => {
	const response = posted(page);
	assertNative(response);
	expect(response.status()).toBe(422);
	const body = await response.text();
	expect(body).toContain('Enter a valid email address');
	expect(body).not.toContain('<script');
	await expect(page.getByTestId('no-client-status')).toHaveText('Page status 422');
	await expect(page.getByTestId('no-client-email-error')).toHaveText('Enter a valid email address');
	const form = page.getByTestId('no-client-form');
	await expect(form.getByRole('textbox', { name: 'Name' })).toHaveValue('Grace Hopper');
	await expect(form.getByRole('textbox', { name: 'Email' })).toHaveValue('grace-at-example');
	await expect(form.getByRole('textbox', { name: 'Biography' })).toHaveValue('Keep this biography');
	await expect(page.getByTestId('no-client-saved-name')).toHaveText('Ada Lovelace');
	await expect(page.getByTestId('no-client-saved-email')).toHaveText('ada@example.test');
	await expect(page.getByTestId('no-client-saved-biography')).toHaveText('First programmer');
	await expect(page.getByTestId('no-client-saved-state')).toHaveText('Active');
});

When('I submit the forbidden no-client action', async ({ page }) => {
	await post(page, paths.client, () => page.getByRole('button', { name: 'Try forbidden action' }).click());
});

Then('the no-client error branch renders the 403 section error with its own client boot', async ({ page }) => {
	const response = posted(page);
	assertNative(response);
	expect(response.status()).toBe(403);
	const body = await response.text();
	expect(body).toContain('Action error 403');
	expect(body).toContain('<script');
	await expect(page.getByTestId('action-error-title')).toHaveText('Action error 403');
	await expect(page.getByTestId('action-error-message')).toHaveText('You cannot edit this profile');
	await expect(page.getByTestId('action-error-return')).toBeVisible();
});

When('I submit the unavailable no-client action', async ({ page }) => {
	await post(page, paths.client, () => page.getByRole('button', { name: 'Try unavailable action' }).click());
});

Then('the no-client error branch renders the safe 500 section error with its own client boot', async ({ page }) => {
	const response = posted(page);
	assertNative(response);
	expect(response.status()).toBe(500);
	const body = await response.text();
	expect(body).toContain('Action error 500');
	expect(body).toContain('Something went wrong on our end.');
	expect(body).toContain('case-1121');
	expect(body).not.toContain('private-actions-database-token-4731');
	expect(body).toContain('<script');
	await expect(page.getByTestId('action-error-title')).toHaveText('Action error 500');
	await expect(page.getByTestId('action-error-message')).toHaveText('Something went wrong on our end.');
	await expect(page.getByTestId('action-error-support-id')).toHaveText('case-1121');
});

Given('I open the client-rendered Actions page after boot', async ({ page }) => {
	const response = await page.goto(paths.ssr);
	expectMode(response!);
	expect(response!.status()).toBe(200);
	const body = await response!.text();
	expect(body).not.toContain('data-testid="no-ssr-title"');
	expect(body.includes('rel="stylesheet"') || body.includes('<style data-sveltekit>')).toBe(true);
	await hydrated(page);
	await expect(page.getByTestId('no-ssr-title')).toHaveText('Client-rendered actions');
	await expect(page.getByTestId('no-ssr-saved-name')).toHaveText('Ada Lovelace');
});

When('I save Grace with Kit enhancement on the client-rendered page', async ({ page }) => {
	const form = page.getByTestId('enhanced-no-ssr-form');
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill('grace@example.test');
	await form.getByRole('textbox', { name: 'Biography' }).fill('Compiler pioneer');
	const response = await post(page, paths.ssr, () => form.getByRole('button', { name: 'Save profile' }).click(), false);
	expect(response.request().headers()['x-sveltekit-action']).toBe('true');
	expect(response.request().resourceType()).not.toBe('document');
	expect(response.status()).toBe(200);
	expect(await response.text()).toContain('Saved Grace Hopper');
});

Then('the client-rendered page shows the exact Go receipt and saved Grace profile', async ({ page }) => {
	await expect(page.getByTestId('no-ssr-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('no-ssr-saved-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('no-ssr-saved-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('no-ssr-saved-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('no-ssr-saved-state')).toHaveText('Active');
});

When(/^I submit (success|validation|forbidden|unavailable) natively on the client-rendered page$/, async ({ page }, outcome: Outcome) => {
	if (outcome === 'forbidden' || outcome === 'unavailable') {
		await post(page, paths.ssr, () => page.getByRole('button', { name: outcome === 'forbidden' ? 'Try forbidden action' : 'Try unavailable action' }).click(), true, true);
		return;
	}
	const form = page.getByTestId('native-no-ssr-form');
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill(outcome === 'success' ? 'grace@example.test' : 'grace-at-example');
	await form.getByRole('textbox', { name: 'Biography' }).fill(outcome === 'success' ? 'Compiler pioneer' : 'Keep this biography');
	await post(page, paths.ssr, () => form.getByRole('button', { name: 'Save profile' }).click(), true, true);
});

Then(/^the (success|validation|forbidden|unavailable) response is a styled shell with the action status and no action outcome$/, async ({ page }, outcome: Outcome) => {
	await assertShell(posted(page), outcome);
});

Then(/^after boot the ordinary client-rendered page shows (Grace|Ada) without an action outcome$/, async ({ page }, profile: 'Grace' | 'Ada') => {
	await hydrated(page);
	await expect(page.getByTestId('no-ssr-title')).toHaveText('Client-rendered actions');
	await expect(page.getByTestId('no-ssr-title')).toBeVisible();
	await expect(page.getByTestId('no-ssr-saved-name')).toHaveText(profile === 'Grace' ? 'Grace Hopper' : 'Ada Lovelace');
	await expect(page.getByTestId('no-ssr-saved-name')).toBeVisible();
	await expect(page.getByTestId('no-ssr-saved-email')).toHaveText(profile === 'Grace' ? 'grace@example.test' : 'ada@example.test');
	await expect(page.getByTestId('no-ssr-receipt')).toHaveCount(0);
	await expect(page.getByTestId('no-ssr-email-error')).toHaveCount(0);
});

When('I submit native sign-in on the client-rendered page', async ({ page }) => {
	await post(page, paths.ssr, () => page.getByRole('button', { name: 'Native sign in' }).click());
});

Then('the native action redirects to the signed-in destination with its Go cookie', async ({ page }) => {
	const response = posted(page);
	assertNative(response);
	expect(response.status()).toBe(303);
	expect(response.headers()['location']).toBe('/actions/signed-in');
	await expect(page).toHaveURL(/\/actions\/signed-in$/);
	await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
});

Then('the browser has scripting disabled and the no-client form is visible', async ({ page, $testInfo }) => {
	expect($testInfo.project.name).toBe('noscript');
	await expect(page.getByTestId('no-client-noscript')).toBeVisible();
	await expect(page.getByTestId('no-client-form').getByRole('button', { name: 'Save profile' })).toBeVisible();
});

Given('I request the client-rendered Actions page without scripting', async ({ page, $testInfo }) => {
	expect($testInfo.project.name).toBe('noscript');
	const response = await page.goto(paths.ssr);
	expectMode(response!);
	expect(response!.status()).toBe(200);
	await assertShell(response!, 'success');
	await expect(page.getByTestId('no-ssr-title')).toHaveCount(0);
	await page.goto(paths.client);
	await expect(page.getByTestId('no-client-title')).toHaveText('Actions without client JavaScript');
});

When(/^I post (success|validation|forbidden|unavailable) to the client-rendered page without scripting$/, async ({ page }, outcome: Outcome) => {
	const label = outcome === 'success' ? 'Post valid client-rendered edit' : outcome === 'validation' ? 'Post invalid client-rendered edit' : outcome === 'forbidden' ? 'Post forbidden client-rendered edit' : 'Post unavailable client-rendered edit';
	await post(page, paths.ssr, () => page.getByRole('button', { name: label }).click());
});

Then('the page still has no rendered action or profile without scripting', async ({ page }) => {
	await expect(page.getByTestId('no-ssr-title')).toHaveCount(0);
	await expect(page.getByTestId('no-ssr-receipt')).toHaveCount(0);
	await expect(page.getByTestId('no-ssr-saved-name')).toHaveCount(0);
	await expect(page.getByTestId('action-error-title')).toHaveCount(0);
});
