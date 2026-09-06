import { expect, test } from '@playwright/test';

import { shot, waitForHydration } from './helpers';

// Junkyard: e2e/platform.spec.ts
test('the app sees its own URL, client address, and hook-set locals', async ({ page, baseURL }) => {
	const response = await page.goto('/whoami');
	await waitForHydration(page);
	await shot(page, 'whoami');
	await expect(page.getByTestId('url')).toHaveText(`${baseURL}/whoami`);
	await expect(page.getByTestId('client-address')).not.toBeEmpty();
	await expect(page.getByTestId('host')).toHaveText(new URL(baseURL!).host);

	const requestId = await page.getByTestId('request-id').textContent();
	expect(requestId).toMatch(/^[0-9a-f-]{32,36}$/);
	expect(response?.headers()['x-guestbook-request-id']).toBe(requestId);
});

test('static assets: content type, immutable caching, etag revalidation', async ({
	page,
	request
}) => {
	const logo = await request.get('/logo.svg');
	expect(logo.status()).toBe(200);
	expect(logo.headers()['content-type']).toContain('image/svg+xml');

	const scripts: string[] = [];
	page.on('request', (r) => {
		if (r.url().includes('/_app/immutable/')) scripts.push(r.url());
	});
	await page.goto('/');
	expect(scripts.length).toBeGreaterThan(0);

	const asset = await request.get(scripts[0]);
	expect(asset.status()).toBe(200);
	expect(asset.headers()['cache-control']).toContain('immutable');

	const etag = asset.headers()['etag'];
	expect(etag).toBeTruthy();
	const revalidated = await request.get(scripts[0], { headers: { 'if-none-match': etag } });
	expect(revalidated.status()).toBe(304);
});

test('precompressed assets are negotiated', async ({ page, request }) => {
	const scripts: string[] = [];
	page.on('request', (r) => {
		if (r.url().includes('/_app/immutable/') && r.url().endsWith('.js')) scripts.push(r.url());
	});
	await page.goto('/');

	const br = await request.get(scripts[0], { headers: { 'accept-encoding': 'br' } });
	expect(br.headers()['content-encoding']).toBe('br');
	expect(br.headers()['vary']?.toLowerCase()).toContain('accept-encoding');

	const identity = await request.get(scripts[0], { headers: { 'accept-encoding': 'identity' } });
	expect(identity.headers()['content-encoding']).toBeUndefined();
});

test('request bodies over the limit are rejected with 413', async ({ request }) => {
	const small = await request.post('/api/echo', { data: 'x'.repeat(100_000) });
	expect(small.status()).toBe(200);
	expect(await small.json()).toEqual({ bytes: 100_000 });

	const large = await request.post('/api/echo', { data: 'x'.repeat(600_000) });
	expect(large.status()).toBe(413);
});
