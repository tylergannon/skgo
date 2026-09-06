import { createBdd } from 'playwright-bdd';
import type { Page } from '@playwright/test';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

When('the todo list has loaded', async ({ page }) => {
	// Both the list query and the live-count stream must have gone out before a
	// scenario starts counting remote requests.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible();
	await expect(page.getByTestId('count')).toBeVisible();
});

When('I note the remote request count', async ({ remotes }) => {
	remotes.mark();
});

When('I note the live count as {string}', async ({ page, notes }, label: string) => {
	notes.set(label, await liveCount(page));
});

When('I add the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('new-todo').fill(text);
	await page.getByTestId('add-todo').click();
});

When('the todo detail has loaded', async ({ page }) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).not.toBeEmpty();
});

When('I open the todo {string}', async ({ page }, text: string) => {
	await page.getByTestId('todos').getByRole('link', { name: text }).click();
});

When('I rename the open todo to {string}', async ({ page }, text: string) => {
	await page.getByTestId('rename-todo').fill(text);
	await page.getByTestId('save-todo').click();
});

Then('I do not see the todo {string}', async ({ page }, text: string) => {
	// An absence is only evidence when the thing that would have shown it is
	// working. A crashed query renders the boundary's `failed` snippet and no
	// todos at all, and a bare count-of-zero would call that a pass.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('todo').first()).toBeVisible({ timeout: 15_000 });
	await expect(page.getByTestId('todo').filter({ hasText: text })).toHaveCount(0, {
		timeout: 15_000
	});
});

Then('I see the todo {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('todo').filter({ hasText: text }).first()).toBeVisible({
		timeout: 15_000
	});
});

Then('the todo detail shows {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('todo-detail').getByTestId('todo-text')).toHaveText(text, {
		timeout: 15_000
	});
});

Then(
	'exactly {int} remote request was made since',
	async ({ page, remotes }, expected: number) => {
		// A second, non-single-flight round trip would already be in flight by
		// now; wait a beat so it lands and can be counted.
		await page.waitForTimeout(500);
		expect(remotes.since, `remote requests: ${remotes.urlsSince.join(', ') || 'none'}`).toBe(
			expected
		);
	}
);

Then(
	'the live count is {int} more than {string}',
	async ({ page, notes }, by: number, label: string) => {
		await expectLiveCount(page, noted(notes, label) + by);
	}
);

Then('the live count is the same as {string}', async ({ page, notes }, label: string) => {
	await expectLiveCount(page, noted(notes, label));
});

// The number on screen is the live query's answer; the list beside it is the
// `getTodos` query's answer. They are two different endpoints, so asserting one
// against the other is an assertion about two real numbers, and it is the thing
// a signed-out visitor can check with their own eyes.
Then('the todo count is the number of todos on the page', async ({ page }) => {
	await countMatchesList(page, 'todo-count');
});

// The same check after a command has driven the live query, kept under its own
// name so both moments leave a screenshot behind.
Then('the todo count still matches the todos on the page', async ({ page }) => {
	await countMatchesList(page, 'live-count-after-command');
});

async function countMatchesList(page: Page, shot: string): Promise<void> {
	const path = `../../ephemeral/screenshots/${shot}-${await audience(page)}-${
		process.env.EXPECTED_MODE ?? 'unknown'
	}.png`;
	try {
		// Zero against zero is not agreement, it is two broken components
		// agreeing about nothing. Both halves have to be on screen first.
		await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
		await expect(page.getByTestId('todo').first()).toBeVisible({ timeout: 15_000 });
		await expect(async () => {
			const listed = await page.getByTestId('todo').count();
			const counted = await liveCount(page);
			expect(counted, `the counter says ${counted}; the page lists ${listed} todos`).toBe(listed);
		}).toPass({ timeout: 15_000 });
	} finally {
		await page.screenshot({ path, fullPage: true });
	}
}

async function expectLiveCount(page: Page, expected: number): Promise<void> {
	await expect(page.getByTestId('count')).toHaveText(String(expected), { timeout: 15_000 });
}

function noted(notes: Map<string, number>, label: string): number {
	const value = notes.get(label);
	expect(value, `the live count was never noted as ${JSON.stringify(label)}`).not.toBeUndefined();
	return value!;
}

/** "signed-out", or "signed-in-<user>" — used to name the screenshot. */
async function audience(page: Page): Promise<string> {
	const session = (await page.getByTestId('session').innerText()).trim();
	const user = session.match(/^Signed in as (.+)$/);
	return user ? `signed-in-${user[1]}` : 'signed-out';
}

async function liveCount(page: Page): Promise<number> {
	const text = await page.getByTestId('count').innerText();
	const value = Number(text.trim());
	expect(Number.isFinite(value), `live count was ${JSON.stringify(text)}`).toBe(true);
	return value;
}
