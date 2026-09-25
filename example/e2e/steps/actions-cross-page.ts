import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import type { Page, Response } from '@playwright/test';

const { Given, When, Then } = createBdd(test);
type Outcome = 'success' | 'validation' | 'redirect' | 'forbidden' | 'unavailable';
type Submission = 'Kit enhancement' | 'native form';

const fixture = {
	name: 'Ada Lovelace', email: 'ada@example.test', biography: 'First programmer', state: 'Active'
};
const grace = {
	name: 'Grace Hopper', email: 'grace@example.test', biography: 'Compiler pioneer', state: 'Active'
};
const controls: Record<Outcome, string> = {
	success: 'save', validation: 'invalid', redirect: 'signin', forbidden: 'forbidden', unavailable: 'unavailable'
};
const actions: Record<Outcome, string> = {
	success: 'save', validation: 'save', redirect: 'signIn', forbidden: 'forbidden', unavailable: 'unavailable'
};

async function assertSaved(page: Page, values: typeof fixture) {
	await expect(page.getByTestId('cross-saved-name')).toHaveText(values.name);
	await expect(page.getByTestId('cross-saved-email')).toHaveText(values.email);
	await expect(page.getByTestId('cross-saved-biography')).toHaveText(values.biography);
	await expect(page.getByTestId('cross-saved-state')).toHaveText(values.state);
}

Given('I open the cross-page action sender', async ({ page, $testInfo }) => {
	const response = await page.goto('/actions');
	expectMode(response!);
	await page.getByRole('link', { name: 'Submit to another page' }).click();
	await expect(page).toHaveURL(/\/actions\/cross\/send$/);
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
});

Then('the sender offers the destination actions', async ({ page, $testInfo }) => {
	await expect(page.getByTestId('cross-source-title')).toHaveText('Submit to another page');
	for (const action of Object.values(controls)) {
		await expect(page.getByTestId(`native-cross-${action}`)).toBeVisible();
		await expect(page.getByTestId(`enhanced-cross-${action}`)).toBeVisible();
	}
	if ($testInfo.project.name === 'noscript') {
		expect($testInfo.project.use.javaScriptEnabled).toBe(false);
		await expect(page.getByTestId('cross-noscript')).toBeVisible();
	}
});

When(/^I submit cross-page (success|validation|redirect|forbidden|unavailable) with (Kit enhancement|native form)$/, async ({ page, documents, notes, $testInfo }, outcome: Outcome, submission: Submission) => {
	if ($testInfo.project.name === 'noscript') expect(submission).toBe('native form');
	notes.set('cross-enhanced', submission === 'Kit enhancement' ? 1 : 0);
	notes.set('cross-documents-before', documents.count);
	const mode = submission === 'Kit enhancement' ? 'enhanced' : 'native';
	const action = actions[outcome];
	const form = page.getByTestId(`${mode}-cross-${controls[outcome]}`);
	const before = page.url();
	const responsePromise = page.waitForResponse((candidate) => {
		const url = new URL(candidate.url());
		return candidate.request().method() === 'POST' && url.pathname === '/actions/cross/receive' && url.searchParams.has(`/${action}`);
	});
	await form.getByRole('button').click();
	const response: Response = await responsePromise;
	expectMode(response);
	expect(before).toContain('/actions/cross/send');
	const request = response.request();
	if (submission === 'Kit enhancement') {
		expect(request.headers()['x-sveltekit-action']).toBe('true');
		expect(request.resourceType()).not.toBe('document');
	} else {
		expect(request.headers()['x-sveltekit-action']).toBeUndefined();
		expect(request.resourceType()).toBe('document');
	}
	const status = outcome === 'validation' ? 422 : outcome === 'forbidden' ? 403 : outcome === 'unavailable' ? 500 : submission === 'native form' && outcome === 'redirect' ? 303 : 200;
	expect(response.status()).toBe(status);
	const body = submission === 'native form' && outcome === 'redirect' ? '' : await response.text();
	expect(body).not.toContain('private-actions-database-token-4731');
	if (submission === 'Kit enhancement') {
		const result = JSON.parse(body);
		expect(result.type).toBe(outcome === 'validation' ? 'failure' : outcome === 'redirect' ? 'redirect' : outcome === 'forbidden' || outcome === 'unavailable' ? 'error' : 'success');
		if (outcome === 'redirect') {
			expect(result.status).toBe(303);
			expect(result.location).toBe('/actions/signed-in');
		} else {
			expect(result.location).toBe('/actions/cross/receive?source=invite');
			if (outcome === 'validation') expect(result.status).toBe(422);
			if (outcome === 'success') {
				expect(result.status).toBe(200);
				expect(result.data).toContain('Saved Grace Hopper');
				expect(result.data).toContain('Money');
			}
			if (outcome === 'forbidden' || outcome === 'unavailable') {
				expect(result.error.status).toBe(status);
				expect(result.error.message).toBe(outcome === 'forbidden' ? 'You cannot edit this profile' : 'Something went wrong on our end.');
				expect(result.error.supportId).toBe('case-1121');
			}
		}
	} else if (outcome === 'redirect') {
		expect(response.headers()['location']).toBe('/actions/signed-in');
	} else {
		expect(body).toContain(outcome === 'success' ? 'Saved Grace Hopper' : outcome === 'validation' ? 'Enter a valid email address' : outcome === 'forbidden' ? 'Cross-page action error 403' : 'Cross-page action error 500');
		if (outcome === 'success') expect(body).toContain('$7.50');
		if (outcome === 'validation') {
			expect(body).toContain('grace-at-example');
			expect(body).toContain('Keep this biography');
		}
	}
});

Then(/^the cross-page (success|validation|redirect|forbidden|unavailable) destination shows its exact outcome$/, async ({ page, documents, notes, $testInfo }, outcome: Outcome) => {
	if (outcome === 'redirect') {
		await expect(page).toHaveURL(/\/actions\/signed-in$/);
		await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
		await expect(page.getByTestId('signed-in-cookie')).toBeVisible();
	} else if (outcome === 'forbidden' || outcome === 'unavailable') {
		const status = outcome === 'forbidden' ? 403 : 500;
		await expect(page.getByTestId('cross-error-title')).toHaveText(`Cross-page action error ${status}`);
		await expect(page.getByTestId('cross-error-message')).toHaveText(outcome === 'forbidden' ? 'You cannot edit this profile' : 'Something went wrong on our end.');
		await expect(page.getByTestId('cross-error-support-id')).toHaveText('case-1121');
		await expect(page.getByTestId('cross-error-return')).toBeVisible();
		await expect(page.locator('body')).not.toContainText('private-actions-database-token-4731');
	} else {
		if (notes.get('cross-enhanced') === 1) await expect(page).toHaveURL(/\/actions\/cross\/receive\?source=invite$/);
		await expect(page.getByTestId('cross-destination-title')).toHaveText('Cross-page destination');
		await expect(page.getByTestId('cross-source-query')).toHaveText('Source invite');
		await expect(page.getByTestId('cross-status')).toHaveText(`Page status ${outcome === 'success' ? 200 : 422}`);
		if (outcome === 'success') {
			await expect(page.getByTestId('cross-receipt')).toHaveText('Saved Grace Hopper');
			await expect(page.getByTestId('cross-money')).toHaveText('$7.50');
			await assertSaved(page, grace);
		} else {
			await expect(page.getByTestId('cross-validation')).toBeVisible();
			await expect(page.getByTestId('cross-email-error')).toHaveText('Enter a valid email address');
			await expect(page.getByRole('textbox', { name: 'Name', exact: true })).toHaveValue('Grace Hopper');
			await expect(page.getByRole('textbox', { name: 'Email', exact: true })).toHaveValue('grace-at-example');
			await expect(page.getByRole('textbox', { name: 'Biography', exact: true })).toHaveValue('Keep this biography');
			await assertSaved(page, fixture);
		}
	}
	if ($testInfo.project.name === 'noscript') {
		await expect(page.getByTestId(outcome === 'redirect' ? 'signed-in-noscript' : outcome === 'forbidden' || outcome === 'unavailable' ? 'cross-error-noscript' : 'cross-noscript')).toBeVisible();
	}
	if ($testInfo.project.name !== 'noscript') {
		await hydrated(page);
		await page.getByTestId('client-interaction').click();
		await expect(page.getByTestId('client-interaction-done')).toHaveText('Client interaction complete');
		if (outcome === 'success') await expect(page.getByTestId('cross-money')).toHaveText('$7.50');
	}
	if (notes.get('cross-enhanced') === 1) expect(documents.count).toBe(notes.get('cross-documents-before'));
});

Then(/^the cross-page (success|validation|redirect|forbidden|unavailable) fixture remains correct on a later GET$/, async ({ page }, outcome: Outcome) => {
	if (outcome === 'forbidden' || outcome === 'unavailable') {
		await page.getByTestId('cross-error-return').click();
		await expect(page.getByTestId('cross-source-title')).toHaveText('Submit to another page');
	}
	const response = await page.goto('/actions/cross/receive');
	expectMode(response!);
	expect(response?.status()).toBe(200);
	await expect(page.getByTestId('cross-destination-title')).toHaveText('Cross-page destination');
	await assertSaved(page, outcome === 'success' ? grace : fixture);
	await expect(page.getByTestId('cross-receipt')).toHaveCount(0);
});
