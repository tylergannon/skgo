import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import type { Page, Response } from '@playwright/test';

const { When, Then } = createBdd(test);
type Submission = 'Kit enhancement' | 'native form';
const modeOf = (submission: Submission) => submission === 'Kit enhancement' ? 'enhanced' : 'native';

async function submit(page: Page, selector: string, submission: Submission): Promise<Response> {
	if (submission === 'Kit enhancement') await hydrated(page);
	const pending = page.waitForResponse((candidate) => candidate.request().method() === 'POST' && new URL(candidate.url()).pathname === '/actions');
	await page.getByTestId(`${modeOf(submission)}-${selector}-form`).getByRole('button').click();
	const response = await pending;
	expectMode(response);
	expect(response.request().headers()['x-sveltekit-action'] === 'true').toBe(submission === 'Kit enhancement');
	expect(response.request().resourceType() === 'document').toBe(submission === 'native form');
	return response;
}

async function ada(page: Page) {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('saved-name')).toHaveText('Ada Lovelace');
	await expect(page.getByTestId('saved-email')).toHaveText('ada@example.test');
	await expect(page.getByTestId('saved-biography')).toHaveText('First programmer');
	await expect(page.getByTestId('saved-state')).toHaveText('Active');
}

When(/^I remember the action cookie with (Kit enhancement|native form)$/, async ({ page, notes }, submission: Submission) => {
	const response = await submit(page, 'cookie', submission);
	notes.set('context-cookie-status', response.status());
	expect(response.status()).toBe(200);
	expect((await response.headerValue('set-cookie')) ?? '').toContain('skgo_actions_feedback=violet-42');
	const body = await response.text();
	if (submission === 'Kit enhancement') {
		const result = JSON.parse(body);
		expect(result.type).toBe('success');
		expect(result.status).toBe(200);
		expect(result.data).toContain('Remembered violet-42');
	} else {
		expect(body).toMatch(/data-testid="action-cookie-value"[^>]*>violet-42</);
		expect(body).toContain('Remembered violet-42');
	}
});

Then('the submitting response and page show the immediate cookie', async ({ page, notes }) => {
	expect(notes.get('context-cookie-status')).toBe(200);
	await ada(page);
	await expect(page.getByTestId('action-cookie-value')).toHaveText('violet-42');
	await expect(page.getByTestId('action-receipt')).toHaveText('Remembered violet-42');
});

Then('a later editor GET still shows the literal cookie and Ada fixture', async ({ page, $testInfo }) => {
	const response = await page.goto('/actions');
	expectMode(response!);
	expect(response?.status()).toBe(200);
	await ada(page);
	await expect(page.getByTestId('action-cookie-value')).toHaveText('violet-42');
	await expect(page.getByTestId('action-receipt')).toHaveCount(0);
	if ($testInfo.project.name === 'noscript') await expect(page.getByTestId('actions-noscript')).toBeVisible();
});

When(/^I save Grace and inspect the action header with (Kit enhancement|native form)$/, async ({ page, notes }, submission: Submission) => {
	if (submission === 'Kit enhancement') await hydrated(page);
	const form = page.getByTestId(`${modeOf(submission)}-save-form`);
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill('grace@example.test');
	await form.getByRole('textbox', { name: 'Biography' }).fill('Compiler pioneer');
	const response = await submit(page, 'save', submission);
	notes.set('context-header-status', response.status());
	expect(response.status()).toBe(200);
	expect(response.headers()['x-skgo-action-demo']).toBe('profile-saved');
	const body = await response.text();
	expect(body).toContain('Saved Grace Hopper');
});

Then("the response has the fixed header and the page has Grace's saved profile", async ({ page, notes }) => {
	expect(notes.get('context-header-status')).toBe(200);
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('action-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('saved-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('saved-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('saved-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('saved-state')).toHaveText('Active');
});

When(/^I attempt the guarded sign-in with (Kit enhancement|native form)$/, async ({ page, notes }, submission: Submission) => {
	const response = await submit(page, 'hook-redirect', submission);
	expect(new URL(response.url()).searchParams.get('hook')).toBe('sign-in');
	notes.set('hook-redirect-enhanced', submission === 'Kit enhancement' ? 1 : 0);
	expect(response.status()).toBe(submission === 'Kit enhancement' ? 200 : 303);
	if (submission === 'Kit enhancement') {
		expect(await response.json()).toEqual({ type: 'redirect', status: 303, location: '/actions/signed-in?required=1' });
	} else {
		expect(response.headers()['location']).toBe('/actions/signed-in?required=1');
	}
});

Then("the hook redirect has Kit's response and the sign-in destination", async ({ page, notes }) => {
	expect(notes.has('hook-redirect-enhanced')).toBe(true);
	await expect(page).toHaveURL(/\/actions\/signed-in\?required=1$/);
	await expect(page.getByTestId('hook-sign-in-title')).toHaveText('Sign in required');
	await expect(page.getByTestId('hook-sign-in-message')).toHaveText('The request was intercepted before the profile action ran.');
});

When(/^I attempt the guarded forbidden edit with (Kit enhancement|native form)$/, async ({ page, notes }, submission: Submission) => {
	const response = await submit(page, 'hook-error', submission);
	expect(new URL(response.url()).searchParams.get('hook')).toBe('forbidden');
	notes.set('hook-error-enhanced', submission === 'Kit enhancement' ? 1 : 0);
	expect(response.status()).toBe(403);
	const body = await response.text();
	if (submission === 'Kit enhancement') {
		expect(response.headers()['content-type']).toContain('application/json');
		expect(JSON.parse(body)).toEqual({ status: 403, message: 'Hook denied this edit', supportId: 'case-1121' });
		expect(body).not.toContain('"type"');
	} else {
		expect(response.headers()['content-type']).toContain('text/html');
		expect(body).toContain('Hook denied this edit');
		expect(body).toContain('403');
		expect(body).not.toContain('data-testid="action-error-title"');
	}
});

Then("the hook refusal has Kit's response and visible page behavior", async ({ page, notes }) => {
	if (notes.get('hook-error-enhanced') === 1) {
		await expect(page.getByTestId('action-error-title')).toHaveText('Action error 403');
		await expect(page.getByTestId('action-error-message')).toHaveText('Hook denied this edit');
		await expect(page.getByTestId('action-error-support-id')).toHaveText('case-1121');
	} else {
		await expect(page.locator('body')).toContainText('403');
		await expect(page.locator('body')).toContainText('Hook denied this edit');
		await expect(page.getByTestId('action-error-title')).toHaveCount(0);
	}
});

Then('a later editor GET still has every Ada fixture field', async ({ page, $testInfo }) => {
	const response = await page.goto('/actions');
	expectMode(response!);
	expect(response?.status()).toBe(200);
	await ada(page);
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	if ($testInfo.project.name === 'noscript') await expect(page.getByTestId('actions-noscript')).toBeVisible();
});
