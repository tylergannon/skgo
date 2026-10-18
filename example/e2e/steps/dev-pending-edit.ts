import { createBdd } from 'playwright-bdd';
import type { Response } from '@playwright/test';
import { expect, interactive, test as base } from './fixtures';

// The opt-in Go integration test supplies an isolated development application
// and control server. Ordinary production/dev example runs exclude this tag.
export const test = base.extend<{
	pendingNavigation: { response?: Promise<Response | null> };
}>({
	pendingNavigation: async ({}, use) => {
		await use({});
	}
});
const { Given, When, Then } = createBdd(test);

function control(): string {
	const url = process.env.SKGO_E2E_PENDING_EDIT;
	if (!url)
		throw new Error(
			'run TestRealSessionRouteEditDuringPendingLoad for this isolated fixture'
		);
	return url;
}

Given(
	'a development page is waiting for a Go network response',
	async ({ page, request, pendingNavigation }) => {
		pendingNavigation.response = page.goto('/pending');
		// Attach a rejection handler immediately, including if the entered assertion fails.
		void pendingNavigation.response.catch(() => {});
		await expect
			.poll(async () => {
				const response = await request.get(`${control()}/entered`);
				return response.ok() ? await response.text() : '';
			})
			.toBe('literal-go-waiting-113');
	}
);

Then(
	'an independent Go endpoint responds while the page waits',
	async ({ request, pendingNavigation }) => {
		let completed = false;
		void pendingNavigation.response!.then(
			() => {
				completed = true;
			},
			() => {
				completed = true;
			}
		);
		const response = await request.get('/orders', { timeout: 2_000 });
		expect(response.status()).toBe(200);
		expect(await response.text()).toBe('literal-4101:8');
		expect(
			completed,
			'held document completed before the Go load was released'
		).toBe(false);
	}
);

When(
	'the route and layout structure changes before releasing the Go load',
	async ({ request }) => {
		const edited = await request.post(`${control()}/edit`);
		expect(edited.status()).toBe(200);
		expect(await edited.text()).toBe('literal-structure-edited-113');
		const released = await request.post(`${control()}/release`);
		expect(released.status()).toBe(200);
	}
);

Then(
	'the held page finishes with its Go data',
	async ({ page, pendingNavigation }) => {
		const response = await pendingNavigation.response!;
		expect(response?.status()).toBe(200);
		await expect(
			page.getByRole('heading', { name: 'literal-held-page-113' })
		).toBeVisible();
		await expect(page.getByTestId('go-receipt')).toHaveText(
			'literal-go-released-113'
		);
	}
);

Then(
	'the changed route has its new layout and hydrates',
	async ({ page, request }) => {
		// Kit debounces add/unlink manifest updates. Wait for the new route after
		// releasing the pending load, as a developer's next request would.
		await expect
			.poll(
				async () => {
					const response = await request.get('/pending/changed');
					return (
						response.status() === 200 &&
						(await response.text()).includes('literal-new-layout-113')
					);
				},
				{ timeout: 20_000 }
			)
			.toBe(true);
		const response = await page.goto('/pending/changed');
		expect(response?.status()).toBe(200);
		await expect(page).toHaveURL(/\/pending\/changed$/);
		await expect(page.getByTestId('edited-layout')).toContainText(
			'literal-new-layout-113'
		);
		await expect(
			page.getByRole('heading', { name: 'literal-new-route-113' })
		).toBeVisible();
		await expect(page.getByTestId('edit-counter')).toHaveText('Clicked 0');
		await interactive(page, 'edit-counter');
		await page.getByTestId('edit-counter').click();
		await expect(page.getByTestId('edit-counter')).toHaveText('Clicked 1');
	}
);
