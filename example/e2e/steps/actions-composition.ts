import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import type { Page, Response } from '@playwright/test';

const { When, Then } = createBdd(test);

const edits = {
	ada: { name: 'Ada Byron', email: 'ada.byron@example.test', biography: 'Analytical engine notes', state: 'Active' },
	grace: { name: 'Grace Murray Hopper', email: 'grace.murray@example.test', biography: 'COBOL language pioneer', state: 'Active' }
};
const fixtures = {
	ada: { name: 'Ada Lovelace', email: 'ada@example.test', biography: 'First programmer', state: 'Active' },
	grace: { name: 'Grace Hopper', email: 'grace@example.test', biography: 'Compiler pioneer', state: 'Active' }
};

type Selected = keyof typeof edits;
type Submission = 'Kit enhancement' | 'native form';

async function assertRecord(page: Page, selected: Selected, values: typeof fixtures.ada) {
	await expect(page.getByTestId(`${selected}-name`)).toHaveText(values.name);
	await expect(page.getByTestId(`${selected}-email`)).toHaveText(values.email);
	await expect(page.getByTestId(`${selected}-biography`)).toHaveText(values.biography);
	await expect(page.getByTestId(`${selected}-state`)).toHaveText(values.state);
}

async function actionResponse(page: Page, path: string, submit: () => Promise<void>): Promise<Response> {
	const response = page.waitForResponse((candidate) => candidate.request().method() === 'POST' && new URL(candidate.url()).pathname === path);
	await submit();
	const answer = await response;
	expectMode(answer);
	return answer;
}

function assertDelivery(response: Response, submission: Submission) {
	if (submission === 'Kit enhancement') {
		expect(response.request().headers()['x-sveltekit-action']).toBe('true');
		expect(response.request().resourceType()).not.toBe('document');
	} else {
		expect(response.request().headers()['x-sveltekit-action']).toBeUndefined();
		expect(response.request().resourceType()).toBe('document');
	}
}

When('I follow the default action example', async ({ page }) => {
	await page.getByRole('link', { name: 'Default action without a load' }).click();
	await expect(page).toHaveURL(/\/actions\/default$/);
});

Then('the default page has no load and offers the Grace form', async ({ page, $testInfo }) => {
	await expect(page.getByTestId('default-title')).toHaveText('Default action, no page load');
	await expect(page.getByTestId('native-default-form')).toBeVisible();
	await expect(page.getByTestId('enhanced-default-form')).toBeVisible();
	await expect(page.getByTestId('default-receipt')).toHaveCount(0);
	if ($testInfo.project.name === 'noscript') await expect(page.getByTestId('default-noscript')).toBeVisible();
});

When(/^I submit the unnamed Grace action with (Kit enhancement|native form)$/, async ({ page, $testInfo }, submission: Submission) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const mode = submission === 'Kit enhancement' ? 'enhanced' : 'native';
	const response = await actionResponse(page, '/actions/default', () => page.getByTestId(`${mode}-default-form`).getByRole('button', { name: 'Save Grace' }).click());
	assertDelivery(response, submission);
	expect(response.status()).toBe(200);
	expect(new URL(response.url()).search).toBe('');
	const body = await response.text();
	expect(body).toContain('Saved Grace Hopper');
	if (submission === 'Kit enhancement') {
		const result = JSON.parse(body);
		expect(result.type).toBe('success');
		expect(result.status).toBe(200);
	}
});

Then('the default action shows the exact Grace receipt and profile', async ({ page }) => {
	await expect(page.getByTestId('default-title')).toHaveText('Default action, no page load');
	await expect(page.getByTestId('default-status')).toHaveText('Page status 200');
	await expect(page.getByTestId('default-receipt')).toHaveText('Saved Grace Hopper');
	await expect(page.getByTestId('default-saved-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('default-saved-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('default-saved-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('default-saved-state')).toHaveText('Active');
	await page.evaluate(() => window.scrollTo(0, 0));
});

Then('a later Go load reads the stored default Grace profile', async ({ page }) => {
	const response = await page.goto('/actions/default/saved');
	expectMode(response!);
	expect(response?.status()).toBe(200);
	await expect(page.getByTestId('default-stored-title')).toHaveText('Stored default profile');
	await expect(page.getByTestId('default-stored-name')).toHaveText('Grace Hopper');
	await expect(page.getByTestId('default-stored-email')).toHaveText('grace@example.test');
	await expect(page.getByTestId('default-stored-biography')).toHaveText('Compiler pioneer');
	await expect(page.getByTestId('default-stored-state')).toHaveText('Active');
	await expect(page.getByTestId('default-receipt')).toHaveCount(0);
	await page.evaluate(() => window.scrollTo(0, 0));
});

When(/^I follow the two profile editor for (ada|grace)$/, async ({ page }, selected: Selected) => {
	await page.getByRole('link', { name: 'Two profile editor' }).click();
	if (selected === 'grace') await page.getByRole('link', { name: 'Edit Grace' }).click();
	await expect(page).toHaveURL(new RegExp(`/actions/profiles/${selected}$`));
	await expect(page.getByTestId('selected-profile')).toHaveText(`Editing ${selected}`);
});

Then('both profiles show their independent fixtures', async ({ page, $testInfo }) => {
	await expect(page.getByTestId('profiles-title')).toHaveText('Two visitor profiles');
	await assertRecord(page, 'ada', fixtures.ada);
	await assertRecord(page, 'grace', fixtures.grace);
	if ($testInfo.project.name === 'noscript') await expect(page.getByTestId('profiles-noscript')).toBeVisible();
	await page.evaluate(() => window.scrollTo(0, 0));
});

When(/^I save the selected profile with (Kit enhancement|native form)$/, async ({ page, $testInfo }, submission: Submission) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const selected = page.url().endsWith('/ada') ? 'ada' : 'grace';
	const values = edits[selected];
	const mode = submission === 'Kit enhancement' ? 'enhanced' : 'native';
	const form = page.getByTestId(`${mode}-profile-form`);
	await form.getByRole('textbox', { name: 'Name' }).fill(values.name);
	await form.getByRole('textbox', { name: 'Email' }).fill(values.email);
	await form.getByRole('textbox', { name: 'Biography' }).fill(values.biography);
	const response = await actionResponse(page, `/actions/profiles/${selected}`, () => form.getByRole('button', { name: 'Save selected profile' }).click());
	assertDelivery(response, submission);
	expect(response.status()).toBe(200);
	expect(new URL(response.url()).searchParams.has('/save')).toBe(true);
	const body = await response.text();
	expect(body).toContain(`Saved ${values.name}`);
});

Then(/^only (ada|grace) has the literal edited values$/, async ({ page }, selected: Selected) => {
	await expect(page.getByTestId('profiles-title')).toHaveText('Two visitor profiles');
	await expect(page.getByTestId('selected-profile')).toHaveText(`Editing ${selected}`);
	await expect(page.getByTestId('profile-receipt')).toHaveText(`Saved ${edits[selected].name}`);
	await assertRecord(page, selected, edits[selected]);
	await assertRecord(page, selected === 'ada' ? 'grace' : 'ada', fixtures[selected === 'ada' ? 'grace' : 'ada']);
	await page.evaluate(() => window.scrollTo(0, 0));
});

Then('a later profile GET retains both literal records', async ({ page }) => {
	const selected = page.url().includes('/profiles/ada') ? 'ada' : 'grace';
	const response = await page.goto(`/actions/profiles/${selected}`);
	expectMode(response!);
	expect(response?.status()).toBe(200);
	await expect(page.getByTestId('profiles-title')).toHaveText('Two visitor profiles');
	await assertRecord(page, selected, edits[selected]);
	await assertRecord(page, selected === 'ada' ? 'grace' : 'ada', fixtures[selected === 'ada' ? 'grace' : 'ada']);
	await expect(page.getByTestId('profile-receipt')).toHaveCount(0);
	await page.evaluate(() => window.scrollTo(0, 0));
});
