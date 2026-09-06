import { expect, test } from '@playwright/test';

import { recordRequests, shot, waitForHydration } from './helpers';

const unique = (label: string) => `${label} ${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;

// Junkyard: e2e/remote.spec.ts
test('query renders server data, prerendered remote renders its banner', async ({ page }) => {
	await page.goto('/messages');
	await waitForHydration(page);
	await shot(page, 'messages');
	await expect(page.getByTestId('banner')).toHaveText('guestbook — prerendered banner');
	await expect(page.getByTestId('message').first()).toBeVisible();
	await expect(page.getByTestId('messages')).toContainText('welcome to the guestbook');
});

test('command updates the query in a single flight', async ({ page }) => {
	await page.goto('/messages');
	await waitForHydration(page);
	const before = Number(await page.getByTestId('live-count').textContent());
	const requests = recordRequests(page);

	const text = unique('single flight');
	await page.getByTestId('new-text').fill(text);
	await page.getByTestId('post').click();
	await page.waitForTimeout(1000);
	await shot(page, 'messages-after-command');

	await expect(page.getByTestId('messages')).toContainText(text);
	await expect(page.getByTestId('live-count')).toHaveText(String(before + 1));

	expect(requests.data()).toHaveLength(1);
	expect(requests.documents()).toHaveLength(0);
});

test('live query pushes updates made from another tab', async ({ browser, page }) => {
	await page.goto('/messages');
	await waitForHydration(page);
	const before = Number(await page.getByTestId('live-count').textContent());

	const other = await (await browser.newContext()).newPage();
	await other.goto('/messages');
	await waitForHydration(other);
	await other.getByTestId('new-text').fill(unique('from another tab'));
	await other.getByTestId('post').click();
	await expect(other.getByTestId('live-count')).toHaveText(String(before + 1));

	await expect(page.getByTestId('live-count')).toHaveText(String(before + 1), { timeout: 10_000 });
	await shot(page, 'messages-live-push');
	await other.context().close();
});

// Junkyard: a remote `form` with a file upload, working both enhanced and via
// the no-JS page-reload fallback.
test('remote form uploads a file without a page reload', async ({ page }) => {
	await page.goto('/messages');
	await waitForHydration(page);
	await shot(page, 'messages-attach-form');
	await expect(page.getByTestId('attach-text')).toBeVisible();
	await page.getByTestId('attach-text').fill(unique('enhanced upload'));
	await page.getByTestId('attach-file').setInputFiles({
		name: 'hello.txt',
		mimeType: 'text/plain',
		buffer: Buffer.from('hello, guestbook')
	});
	await page.getByTestId('attach-submit').click();
	await expect(page.getByTestId('attach-result')).toContainText('uploaded hello.txt (16 bytes)');
});
