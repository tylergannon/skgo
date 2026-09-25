import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import type { Page, Response } from '@playwright/test';

const { When, Then } = createBdd(test);

const grace = {
	name: 'Grace Hopper',
	email: 'grace@example.test',
	biography: 'Compiler pioneer'
};

const invalidGrace = {
	name: 'Grace Hopper',
	email: 'grace-at-example',
	biography: 'Keep this biography'
};

const ada = {
	name: 'Ada Lovelace',
	email: 'ada@example.test',
	biography: 'First programmer',
	state: 'Active'
};

async function profile(page: Page, values: typeof grace & { state: string }) {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('saved-name')).toHaveText(values.name);
	await expect(page.getByTestId('saved-email')).toHaveText(values.email);
	await expect(page.getByTestId('saved-biography')).toHaveText(values.biography);
	await expect(page.getByTestId('saved-state')).toHaveText(values.state);
}

async function fillGrace(page: Page, mode: 'native' | 'enhanced') {
	const form = page.getByTestId(`${mode}-save-form`);
	await form.getByRole('textbox', { name: 'Name' }).fill(grace.name);
	await form.getByRole('textbox', { name: 'Email' }).fill(grace.email);
	await form.getByRole('textbox', { name: 'Biography' }).fill(grace.biography);
	return form;
}

async function fillInvalidGrace(page: Page, mode: 'native' | 'enhanced') {
	const form = page.getByTestId(`${mode}-save-form`);
	await form.getByRole('textbox', { name: 'Name' }).fill(invalidGrace.name);
	await form.getByRole('textbox', { name: 'Email' }).fill(invalidGrace.email);
	await form.getByRole('textbox', { name: 'Biography' }).fill(invalidGrace.biography);
	return form;
}

async function rejected(page: Page) {
	await profile(page, ada);
	await expect(page.getByTestId('page-status')).toHaveText('Page status 422');
	await expect(page.getByTestId('action-status')).toHaveText('Status 422');
	await expect(page.getByTestId('validation-summary')).toBeVisible();
	for (const mode of ['native', 'enhanced']) {
		const form = page.getByTestId(`${mode}-save-form`);
		await expect(form.getByRole('textbox', { name: 'Name' })).toHaveValue(invalidGrace.name);
		await expect(form.getByRole('textbox', { name: 'Email' })).toHaveValue(invalidGrace.email);
		await expect(form.getByRole('textbox', { name: 'Biography' })).toHaveValue(invalidGrace.biography);
		await expect(page.getByTestId(`${mode}-email-error`)).toHaveText('Enter a valid email address');
	}
	await expect(page.getByTestId('action-receipt')).toHaveCount(0);
}

async function archived(page: Page, status = 200) {
	await profile(page, { ...ada, state: 'Archived' });
	await expect(page.getByTestId('page-status')).toHaveText(`Page status ${status}`);
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	await expect(page.getByTestId('action-receipt')).toHaveCount(0);
	await expect(page.getByTestId('action-money')).toHaveCount(0);
}

async function inspectNativeHTML(page: Page, html: string, check: (rendered: Page) => Promise<void>) {
	const browser = page.context().browser();
	if (!browser) throw new Error('native SSR check requires a Playwright browser');
	const noScript = await browser.newContext({ javaScriptEnabled: false });
	try {
		const rendered = await noScript.newPage();
		await rendered.setContent(html, { waitUntil: 'domcontentloaded' });
		await check(rendered);
	} finally {
		await noScript.close();
	}
}

async function receipt(page: Page) {
	await profile(page, { ...grace, state: 'Active' });
	await expect(page.getByTestId('action-status')).toHaveText('Status 200');
	await expect(page.getByTestId('action-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
	await page.evaluate(() => window.scrollTo(0, 0));
}

function saveResponse(response: Response) {
	return response.request().method() === 'POST' && new URL(response.url()).searchParams.has('/save');
}

Then('the Actions editor shows the Ada fixture', async ({ page, shot }) => {
	await profile(page, {
		name: 'Ada Lovelace',
		email: 'ada@example.test',
		biography: 'First programmer',
		state: 'Active'
	});
	await expect(page.getByTestId('native-save-form')).toBeVisible();
	await expect(page.getByTestId('enhanced-save-form')).toBeVisible();
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	await shot('ada-fixture');
});

When('I save Grace with Kit enhancement', async ({ page, documents, notes }) => {
	await hydrated(page);
	const form = await fillGrace(page, 'enhanced');
	notes.set('before-save-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		saveResponse(candidate) && candidate.request().headers()['x-sveltekit-action'] === 'true'
	);
	await form.getByRole('button', { name: 'Save profile' }).click();
	const saved = await response;
	expectMode(saved);
	expect(saved.status()).toBe(200);
	const result = await saved.json();
	expect(result.type).toBe('success');
	expect(result.status).toBe(200);
	expect(result.data).toContain('Money');
});

Then('Kit receives a typed save result without a new document', async ({ page, documents, notes }) => {
	expect(documents.count).toBe(notes.get('before-save-documents'));
	await expect(page).toHaveURL(/\/actions(?:\?.*)?$/);
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
	await page.evaluate(() => window.scrollTo(0, 0));
});

When('I save Grace with the native form', async ({ page, documents, notes, $testInfo }) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const form = await fillGrace(page, 'native');
	notes.set('before-save-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		saveResponse(candidate) && candidate.request().resourceType() === 'document'
	);
	await form.getByRole('button', { name: 'Save profile' }).click();
	const saved = await response;
	expectMode(saved);
	expect(saved.status()).toBe(200);
	const html = await saved.text();
	expect(html).toContain('form: {');
	const browser = page.context().browser();
	if (!browser) throw new Error('native SSR check requires a Playwright browser');
	const noScript = await browser.newContext({ javaScriptEnabled: false });
	try {
		const rendered = await noScript.newPage();
		await rendered.setContent(html, { waitUntil: 'domcontentloaded' });
		await profile(rendered, { ...grace, state: 'Active' });
		await expect(rendered.getByTestId('action-status')).toHaveText('Status 200');
		await expect(rendered.getByTestId('action-receipt')).toHaveText('Saved Grace Hopper');
		await expect(rendered.getByTestId('action-money')).toHaveText('$7.50');
	} finally {
		await noScript.close();
	}
	notes.set('native-response-checked', 1);
});

Then('the native response already contains the Grace receipt and saved profile', async ({ documents, notes }) => {
	expect(notes.get('native-response-checked')).toBe(1);
	expect(documents.count).toBe((notes.get('before-save-documents') ?? 0) + 1);
	expect(documents.last?.request().method()).toBe('POST');
});

Then('the Actions page shows the Grace receipt and saved profile', async ({ page, shot }) => {
	await receipt(page);
	await shot('grace-save');
});

When('I exercise the hydrated Actions client', async ({ page, documents, data, remotes, notes }) => {
	const before = [documents.count, data.count, remotes.count];
	await page.getByTestId('client-interaction').click();
	await expect(page.getByTestId('client-interaction-done')).toHaveText('Client interaction complete');
	expect([documents.count, data.count, remotes.count]).toEqual(before);
	notes.set('hydration-observed', 1);
});

Then('the Grace receipt and saved profile survive hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await receipt(page);
	await shot('after-hydration');
});

Then('a later Actions GET retains the Grace profile', async ({ page, shot }) => {
	const response = await page.goto('/actions');
	expect(response?.request().method()).toBe('GET');
	expectMode(response!);
	await profile(page, { ...grace, state: 'Active' });
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	await shot('later-get');
});

Then('the Actions browser has JavaScript disabled', async ({ page, $testInfo }) => {
	expect($testInfo.project.name).toBe('noscript');
	expect($testInfo.project.use.javaScriptEnabled).toBe(false);
	await expect(page.getByTestId('actions-noscript')).toBeVisible();
});

When('I submit an invalid Grace edit with Kit enhancement', async ({ page, documents, notes }) => {
	await hydrated(page);
	const form = await fillInvalidGrace(page, 'enhanced');
	notes.set('before-action-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		saveResponse(candidate) && candidate.request().headers()['x-sveltekit-action'] === 'true'
	);
	await form.getByRole('button', { name: 'Save profile' }).click();
	const failed = await response;
	expectMode(failed);
	expect(failed.status()).toBe(422);
	const result = await failed.json();
	expect(result.type).toBe('failure');
	expect(result.status).toBe(422);
	expect(result.data).toContain('Enter a valid email address');
	expect(result.data).toContain('Keep this biography');
	notes.set('failure-result-checked', 1);
});

Then('Kit receives a validation failure without a new document', async ({ page, documents, notes }) => {
	expect(notes.get('failure-result-checked')).toBe(1);
	expect(documents.count).toBe(notes.get('before-action-documents'));
	await expect(page).toHaveURL(/\/actions(?:\?.*)?$/);
	await expect(page.getByTestId('page-status')).toHaveText('Page status 422');
	await page.evaluate(() => window.scrollTo(0, 0));
});

When('I submit an invalid Grace edit with the native form', async ({ page, documents, notes, $testInfo }) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const form = await fillInvalidGrace(page, 'native');
	notes.set('before-action-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		saveResponse(candidate) && candidate.request().resourceType() === 'document'
	);
	await form.getByRole('button', { name: 'Save profile' }).click();
	const failed = await response;
	expectMode(failed);
	expect(failed.status()).toBe(422);
	await inspectNativeHTML(page, await failed.text(), rejected);
	notes.set('native-failure-checked', 1);
});

Then('the native failure response already contains the retained fields and Ada profile', async ({ documents, notes }) => {
	expect(notes.get('native-failure-checked')).toBe(1);
	expect(documents.count).toBe((notes.get('before-action-documents') ?? 0) + 1);
	expect(documents.last?.request().method()).toBe('POST');
	expect(documents.last?.status()).toBe(422);
});

Then('the Actions page retains the invalid Grace fields and Ada profile', async ({ page, shot }) => {
	await rejected(page);
	await page.evaluate(() => window.scrollTo(0, 0));
	await shot('validation-failure');
});

Then('the retained failure and Ada profile survive hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await rejected(page);
	await page.evaluate(() => window.scrollTo(0, 0));
	await shot('validation-after-hydration');
});

Then('a later Actions GET retains the Ada profile', async ({ page, shot }) => {
	const response = await page.goto('/actions');
	expect(response?.request().method()).toBe('GET');
	expectMode(response!);
	await profile(page, ada);
	await expect(page.getByTestId('page-status')).toHaveText('Page status 200');
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	await shot('later-get-ada');
});

function archiveResponse(response: Response) {
	return response.request().method() === 'POST' && new URL(response.url()).searchParams.has('/archive');
}

When('I archive with Kit enhancement', async ({ page, documents, notes }) => {
	await hydrated(page);
	notes.set('before-action-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		archiveResponse(candidate) && candidate.request().headers()['x-sveltekit-action'] === 'true'
	);
	await page.getByTestId('enhanced-archive-form').getByRole('button', { name: 'Archive profile' }).click();
	const saved = await response;
	expectMode(saved);
	expect(saved.status()).toBe(200);
	const result = await saved.json();
	expect(result.type).toBe('success');
	expect(result.status).toBe(204);
	expect(result).not.toHaveProperty('data');
	notes.set('archive-result-checked', 1);
	notes.set('archive-enhanced', 1);
});

Then('Kit receives a no-data success without a new document', async ({ page, documents, notes }) => {
	expect(notes.get('archive-result-checked')).toBe(1);
	expect(documents.count).toBe(notes.get('before-action-documents'));
	await expect(page).toHaveURL(/\/actions(?:\?.*)?$/);
	await archived(page, 204);
	await page.evaluate(() => window.scrollTo(0, 0));
});

When('I archive with the native form', async ({ page, documents, notes, $testInfo }) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	notes.set('before-action-documents', documents.count);
	const response = page.waitForResponse((candidate) =>
		archiveResponse(candidate) && candidate.request().resourceType() === 'document'
	);
	await page.getByTestId('native-archive-form').getByRole('button', { name: 'Archive profile' }).click();
	const saved = await response;
	expectMode(saved);
	expect(saved.status()).toBe(200);
	const html = await saved.text();
	expect(html).toContain('form: null');
	await inspectNativeHTML(page, html, archived);
	notes.set('native-archive-checked', 1);
});

Then('the native archive response already contains Ada Archived and null form data', async ({ documents, notes }) => {
	expect(notes.get('native-archive-checked')).toBe(1);
	expect(documents.count).toBe((notes.get('before-action-documents') ?? 0) + 1);
	expect(documents.last?.request().method()).toBe('POST');
	expect(documents.last?.status()).toBe(200);
});

Then('the Actions page shows Ada Archived without a receipt', async ({ page, notes, shot }) => {
	await archived(page, notes.get('archive-enhanced') === 1 ? 204 : 200);
	await page.evaluate(() => window.scrollTo(0, 0));
	await shot('archive');
});

Then('Ada Archived without a receipt survives hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await archived(page);
	await page.evaluate(() => window.scrollTo(0, 0));
	await shot('archive-after-hydration');
});

Then('a later Actions GET retains Ada Archived', async ({ page, shot }) => {
	const response = await page.goto('/actions');
	expect(response?.request().method()).toBe('GET');
	expectMode(response!);
	await archived(page);
	await shot('later-get-archived');
});

type Submission = 'Kit enhancement' | 'native then hydration' | 'native without JavaScript';
type ExtraAction = 'signIn' | 'forbidden' | 'unavailable';

async function submitExtra(
	page: Page,
	documents: { count: number },
	notes: Map<string, number>,
	action: ExtraAction,
	submission: Submission
) {
	const enhanced = submission === 'Kit enhancement';
	if (submission !== 'native without JavaScript') await hydrated(page);
	notes.set('before-extra-documents', documents.count);
	const response = page.waitForResponse((candidate) => {
		const url = new URL(candidate.url());
		return candidate.request().method() === 'POST' && url.pathname === '/actions' &&
			url.searchParams.has(`/${action}`) &&
			(enhanced ? candidate.request().headers()['x-sveltekit-action'] === 'true' : candidate.request().resourceType() === 'document');
	});
	await page.getByTestId(`${enhanced ? 'enhanced' : 'native'}-${action.toLowerCase()}-form`)
		.getByRole('button').click();
	const submitted = await response;
	expectMode(submitted);
	expect(submitted.request().headers()['x-sveltekit-action'] === 'true').toBe(enhanced);
	notes.set('extra-enhanced', enhanced ? 1 : 0);
	notes.set('extra-response-checked', 1);
	return submitted;
}

When(/^I sign in as ada with (Kit enhancement|native then hydration|native without JavaScript)$/, async ({ page, documents, notes }, submission: Submission) => {
	const response = await submitExtra(page, documents, notes, 'signIn', submission);
	if (submission === 'Kit enhancement') {
		expect(response.status()).toBe(200);
		expect(await response.json()).toEqual({ type: 'redirect', status: 303, location: '/actions/signed-in' });
	} else {
		expect(response.status()).toBe(303);
		expect(response.headers()['location']).toBe('/actions/signed-in');
	}
	const cookie = (await response.headerValue('set-cookie')) ?? '';
	expect(cookie).toContain('skgo_actions_signin=ada');
	await expect(page).toHaveURL(/\/actions\/signed-in$/);
	notes.set('extra-submitting-status', response.status());
});

Then('the submitting action redirects separately to the signed-in page', async ({ page, documents, notes }) => {
	expect(notes.get('extra-response-checked')).toBe(1);
	expect(notes.get('extra-submitting-status')).toBe(notes.get('extra-enhanced') ? 200 : 303);
	if (notes.get('extra-enhanced')) expect(documents.count).toBe(notes.get('before-extra-documents'));
	else expect(documents.count).toBe((notes.get('before-extra-documents') ?? 0) + 2);
	await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
});

Then('the destination reads the session cookie and greets ada', async ({ page, shot }) => {
	await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
	await expect(page.getByTestId('signed-in-cookie')).toBeVisible();
	await shot('redirect-destination');
});

Then('the signed-in greeting survives hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
	await shot('redirect-after-hydration');
});

Then('a later signed-in GET still greets ada', async ({ page, shot }) => {
	const response = await page.goto('/actions/signed-in');
	expectMode(response!);
	expect(response?.request().method()).toBe('GET');
	await expect(page.getByTestId('signed-in-title')).toHaveText('Signed in as ada');
	await shot('redirect-later-get');
});

async function extraError(page: Page, documents: { count: number }, notes: Map<string, number>, action: ExtraAction, submission: Submission, status: number) {
	const response = await submitExtra(page, documents, notes, action, submission);
	expect(response.status()).toBe(status);
	const body = await response.text();
	expect(body).not.toContain('private-actions-database-token-4731');
	if (submission === 'Kit enhancement') {
		const result = JSON.parse(body);
		expect(result.type).toBe('error');
		expect(result.location).toBe('/actions');
		expect(result.error.status).toBe(status);
		expect(result.error.message).toBe(status === 403 ? 'You cannot edit this profile' : 'Something went wrong on our end.');
		expect(result.error.supportId).toBe('case-1121');
	} else {
		await inspectNativeHTML(page, body, async (rendered) => {
			await expect(rendered.getByTestId('action-error-title')).toHaveText(`Action error ${status}`);
			await expect(rendered.getByTestId('action-error-message')).toHaveText(status === 403 ? 'You cannot edit this profile' : 'Something went wrong on our end.');
			await expect(rendered.getByTestId('action-error-support-id')).toHaveText('case-1121');
		});
	}
	notes.set('extra-submitting-status', status);
}

When(/^I attempt a forbidden edit with (Kit enhancement|native then hydration|native without JavaScript)$/, async ({ page, documents, notes }, submission: Submission) => {
	await extraError(page, documents, notes, 'forbidden', submission, 403);
});

When(/^I trigger the demo service failure with (Kit enhancement|native then hydration|native without JavaScript)$/, async ({ page, documents, notes }, submission: Submission) => {
	await extraError(page, documents, notes, 'unavailable', submission, 500);
});

Then('the submitting action reports the expected 403 error', async ({ page, documents, notes }) => {
	expect(notes.get('extra-response-checked')).toBe(1);
	expect(notes.get('extra-submitting-status')).toBe(403);
	expect(documents.count).toBe((notes.get('before-extra-documents') ?? 0) + (notes.get('extra-enhanced') ? 0 : 1));
	await expect(page.getByTestId('action-error-title')).toHaveText('Action error 403');
});

Then('the submitting action reports a safe 500 error', async ({ page, documents, notes }) => {
	expect(notes.get('extra-response-checked')).toBe(1);
	expect(notes.get('extra-submitting-status')).toBe(500);
	expect(documents.count).toBe((notes.get('before-extra-documents') ?? 0) + (notes.get('extra-enhanced') ? 0 : 1));
	await expect(page.getByTestId('action-error-title')).toHaveText('Action error 500');
});

async function boundary(page: Page, status: number) {
	await expect(page.getByTestId('action-error-title')).toHaveText(`Action error ${status}`);
	await expect(page.getByTestId('action-error-message')).toHaveText(status === 403 ? 'You cannot edit this profile' : 'Something went wrong on our end.');
	await expect(page.getByTestId('action-error-support-id')).toHaveText('case-1121');
	await expect(page.getByTestId('action-error-return')).toBeVisible();
	await expect(page.locator('body')).not.toContainText('private-actions-database-token-4731');
}

Then('the Actions boundary shows the permission error', async ({ page, shot }) => {
	await boundary(page, 403);
	await shot('forbidden-boundary');
});

Then('the Actions boundary shows the public message and support ID', async ({ page, shot }) => {
	await boundary(page, 500);
	await shot('unexpected-boundary');
});

Then('the permission error survives hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await boundary(page, 403);
	await shot('forbidden-after-hydration');
});

Then('the safe failure survives hydration', async ({ page, notes, shot }) => {
	expect(notes.get('hydration-observed')).toBe(1);
	await boundary(page, 500);
	await shot('unexpected-after-hydration');
});

Then('returning from the error leaves the Ada fixture unchanged', async ({ page, shot }) => {
	await page.getByTestId('action-error-return').click();
	await expect(page).toHaveURL(/\/actions$/);
	await profile(page, ada);
	await expect(page.getByTestId('no-receipt')).toBeVisible();
	await shot('error-return-ada');
});
