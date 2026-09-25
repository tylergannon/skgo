import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import type { Page, Response } from '@playwright/test';

const { When, Then } = createBdd(test);

type Submission = 'Kit enhancement' | 'native form';

async function postResponse(page: Page, predicate: (response: Response) => boolean, submit: () => Promise<void>): Promise<Response> {
	const waiting = page.waitForResponse((response) => response.request().method() === 'POST' && predicate(response));
	await submit();
	const response = await waiting;
	expectMode(response);
	return response;
}

Then('the shared route offers classic and remote forms', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('native-save-form')).toBeVisible();
	await expect(page.getByTestId('native-remote-note')).toBeVisible();
	await expect(page.getByTestId('enhanced-remote-note')).toBeVisible();
	await expect(page.getByTestId('endpoint-post')).toBeVisible();
});

Then('the shared route offers native forms with scripting disabled', async ({ page, $testInfo }) => {
	expect($testInfo.project.use.javaScriptEnabled).toBe(false);
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('coexist-noscript')).toBeVisible();
	await expect(page.getByTestId('native-save-form')).toBeVisible();
	await expect(page.getByTestId('native-remote-note')).toBeVisible();
});

When(/^I save Grace through the classic form with (Kit enhancement|native form)$/, async ({ page, $testInfo }, submission: Submission) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const form = page.getByTestId(submission === 'Kit enhancement' ? 'enhanced-save-form' : 'native-save-form');
	await form.getByRole('textbox', { name: 'Name' }).fill('Grace Hopper');
	await form.getByRole('textbox', { name: 'Email' }).fill('grace@example.test');
	await form.getByRole('textbox', { name: 'Biography' }).fill('Compiler pioneer');
	const response = await postResponse(page, (answer) => new URL(answer.url()).pathname === '/actions', () => form.getByRole('button', { name: 'Save profile' }).click());
	const request = response.request();
	expect(new URL(response.url()).searchParams.has('/save')).toBe(true);
	expect(response.status()).toBe(200);
	expect(await response.text()).toContain('Saved Grace Hopper');
	if (submission === 'Kit enhancement') {
		expect(request.headers()['x-sveltekit-action']).toBe('true');
		expect(request.headers()['accept']).toContain('application/json');
		expect(request.resourceType()).not.toBe('document');
		expect((await response.json()).type).toBe('success');
	} else {
		expect(request.headers()['x-sveltekit-action']).toBeUndefined();
		expect(request.resourceType()).toBe('document');
		expect(response.headers()['content-type']).toContain('text/html');
	}
});

Then('the page shows the classic Grace receipt and saved profile', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('action-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
	await expect(page.getByTestId('saved-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('saved-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('saved-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('saved-state')).toHaveText('Active');
});

When('I ask the sibling endpoint at that route', async ({ page }) => {
	await hydrated(page);
	const response = await postResponse(page, (answer) => new URL(answer.url()).pathname === '/actions', () => page.getByTestId('endpoint-post').click());
	expect(response.request().headers()['x-sveltekit-action']).toBeUndefined();
	expect(response.request().headers()['accept']).toBe('application/json');
	expect(response.headers()['content-type']).toContain('application/json');
	expect(await response.json()).toEqual({ answer: 'Endpoint POST answered by Go' });
});

Then('the endpoint answer is distinct and the classic receipt remains', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('endpoint-answer')).toHaveText('Endpoint POST answered by Go');
	await expect(page.getByTestId('action-receipt')).toHaveText('Saved Grace Hopper');
});

When('I submit the native remote form', async ({ page, $testInfo }) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const form = page.getByTestId('native-remote-note');
	const action = await form.getAttribute('action');
	expect(action).toMatch(/\/remote=.+/);
	const response = await postResponse(page, (answer) => new URL(answer.url()).pathname === '/actions', () => form.getByRole('button', { name: 'Send native remote note' }).click());
	expect(response.request().resourceType()).toBe('document');
	expect(response.request().headers()['x-sveltekit-action']).toBeUndefined();
	expect(new URL(response.url()).searchParams.get('/remote')).toBeTruthy();
	expect(response.headers()['content-type']).toContain('text/html');
	expect(await response.text()).toContain('Remote Go form received Grace Hopper');
});

Then('the remote Go receipt appears without a classic receipt', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('remote-note-receipt')).toHaveText('Remote Go form received Grace Hopper');
	await expect(page.getByTestId('no-receipt')).toBeVisible();
});

When('I submit the enhanced remote form', async ({ page }) => {
	await hydrated(page);
	const response = await postResponse(page, (answer) => new URL(answer.url()).pathname.includes('/_app/remote/'), () => page.getByTestId('enhanced-remote-note').getByRole('button', { name: 'Send enhanced remote note' }).click());
	expect(response.request().resourceType()).not.toBe('document');
	expect(response.request().headers()['x-sveltekit-action']).toBeUndefined();
	expect(response.headers()['content-type']).toContain('application/json');
	expect(await response.text()).toContain('Remote Go form received Grace Hopper');
});

Then('its remote Go receipt appears from the remote protocol', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('remote-note-receipt')).toHaveText('Remote Go form received Grace Hopper');
	await expect(page.getByTestId('no-receipt')).toBeVisible();
});
