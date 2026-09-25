import { createBdd } from 'playwright-bdd';
import { expect, expectMode, hydrated, test } from './fixtures';
import { resolve } from 'node:path';
import type { Page, Response } from '@playwright/test';

const { When, Then } = createBdd(test);
type Submission = 'Kit enhancement' | 'native then hydration' | 'native form';
type Encoding = 'standard' | 'multipart';

async function send(page: Page, formName: string, buttonName: string, submission: Submission): Promise<Response> {
	const response = page.waitForResponse((candidate) => candidate.request().method() === 'POST' && new URL(candidate.url()).pathname === '/actions' && new URL(candidate.url()).searchParams.has(formName));
	await page.getByTestId(`${submission === 'Kit enhancement' ? 'enhanced' : 'native'}-${formName === '/upload' ? 'upload' : 'encoding'}-form`).getByRole('button', { name: buttonName }).click();
	const answer = await response;
	expectMode(answer);
	expect(answer.status()).toBe(200);
	expect(answer.request().headers()['x-sveltekit-action'] === 'true').toBe(submission === 'Kit enhancement');
	expect(answer.request().resourceType() === 'document').toBe(submission !== 'Kit enhancement');
	if (submission === 'Kit enhancement') {
		const result = await answer.json();
		expect(result.type).toBe('success');
		expect(result.status).toBe(200);
	} else {
		const html = await answer.text();
		expect(html).toContain('form: {');
	}
	return answer;
}

When(/^I send the haiku with (Kit enhancement|native then hydration|native form)$/, async ({ page, notes, $testInfo }, submission: Submission) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const form = page.getByTestId(`${submission === 'Kit enhancement' ? 'enhanced' : 'native'}-upload-form`);
	await form.locator('input[type=file]').setInputFiles(resolve('fixtures/haiku.txt'));
	await form.locator('input[value=math]').check();
	await form.locator('input[value=computing]').check();
	const answer = await send(page, '/upload', 'Send poem', submission);
	expect(answer.request().headers()['content-type']).toMatch(/^multipart\/form-data; boundary=/);
	if (submission !== 'Kit enhancement') {
		const html = await answer.text();
		for (const literal of ['haiku.txt', '72 bytes', '84cee680e822', 'math, computing', 'uploadButton=send-poem', '$7.50']) {
			expect(html).toContain(literal);
		}
	}
	notes.set('form-data-native-hydration', submission === 'native then hydration' ? 1 : 0);
});

Then('Go reports haiku.txt, 72 bytes, its SHA-256 prefix, and both interests', async ({ page }) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('upload-name')).toHaveText('haiku.txt');
	await expect(page.getByTestId('upload-bytes')).toHaveText('72 bytes');
	await expect(page.getByTestId('upload-hash')).toHaveText('SHA-256 prefix 84cee680e822');
	await expect(page.getByTestId('upload-interests')).toHaveText('math, computing');
	await expect(page.getByTestId('upload-submitter')).toHaveText('uploadButton=send-poem');
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
});

Then("the upload's Money survives the required browser path", async ({ page, notes }) => {
	if (notes.get('form-data-native-hydration') === 1) {
		// The native POST answered with a new document; its client must boot
		// before the button does anything.
		await hydrated(page);
		await page.getByTestId('client-interaction').click();
		await expect(page.getByTestId('client-interaction-done')).toHaveText('Client interaction complete');
		await expect(page.getByTestId('upload-name')).toHaveText('haiku.txt');
		await expect(page.getByTestId('upload-hash')).toHaveText('SHA-256 prefix 84cee680e822');
	}
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
});

When(/^I submit the (standard|multipart) encoding button with (Kit enhancement|native then hydration|native form)$/, async ({ page, notes, $testInfo }, encoding: Encoding, submission: Submission) => {
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	const answer = await send(page, '/inspect', encoding === 'standard' ? 'Send urlencoded' : 'Send multipart override', submission);
	const contentType = answer.request().headers()['content-type'];
	expect(contentType).toMatch(encoding === 'standard' ? /^application\/x-www-form-urlencoded/ : /^multipart\/form-data; boundary=/);
	const body = answer.request().postDataBuffer()?.toString('utf8') ?? '';
	expect(body).toContain('encodingButton');
	expect(body).toContain(encoding);
	if (submission !== 'Kit enhancement') {
		const html = await answer.text();
		for (const literal of ['math, computing', `encodingButton=${encoding}`, encoding === 'standard' ? 'application/x-www-form-urlencoded' : 'multipart/form-data', '$7.50']) {
			expect(html).toContain(literal);
		}
	}
	notes.set('form-data-native-hydration', submission === 'native then hydration' ? 1 : 0);
});

Then(/^Go reports the chosen (standard|multipart) button and ordered interests$/, async ({ page, notes }, encoding: Encoding) => {
	await expect(page.getByTestId('title')).toHaveText('Actions');
	await expect(page.getByTestId('encoding-interests')).toHaveText('math, computing');
	await expect(page.getByTestId('encoding-submitter')).toHaveText(`encodingButton=${encoding}`);
	await expect(page.getByTestId('encoding-type')).toHaveText(encoding === 'standard' ? 'application/x-www-form-urlencoded' : 'multipart/form-data');
	await expect(page.getByTestId('action-money')).toHaveText('$7.50');
	if (notes.get('form-data-native-hydration') === 1) {
		// The native POST answered with a new document; its client must boot
		// before the button does anything.
		await hydrated(page);
		await page.getByTestId('client-interaction').click();
		await expect(page.getByTestId('client-interaction-done')).toHaveText('Client interaction complete');
		await expect(page.getByTestId('encoding-interests')).toHaveText('math, computing');
		await expect(page.getByTestId('action-money')).toHaveText('$7.50');
	}
});
