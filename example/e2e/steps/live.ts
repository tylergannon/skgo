import type { Page } from '@playwright/test';
import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';
import { tagged } from './ssr';

const { Given, When, Then } = createBdd(test);

/**
 * The other tab. It is a second page in the *same* browser context, so it
 * carries the same session cookie: a change it makes is a change made by the
 * visitor watching the board, which is what "elsewhere" has to mean for the
 * board's own visibility rule to be the thing under test.
 */
const other: { page: Page | null } = { page: null };

Given('another tab is open at {string}', async ({ page }, path: string) => {
	other.page = await page.context().newPage();
	await other.page.goto((process.env.BASE_URL ?? '') + path);
	await expect(other.page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
	await expect(other.page.getByTestId('todo').first()).toBeVisible({ timeout: 15_000 });
});

When('the other tab adds the todo {string}', async ({}, text: string) => {
	const tab = otherTab();
	await hydrated(tab);
	const matching = tab.getByTestId('todo').filter({ hasText: text });
	const before = await matching.count();
	await tab.getByTestId('new-todo').fill(text);
	await tab.getByTestId('add-todo').click();
	// The other tab's own list has to have taken the change before the scenario
	// asks the watching tab about it; otherwise a board that had not moved yet
	// and a board that never would look the same. Count this named fixture's
	// occurrences so rerunning against a long-lived server cannot satisfy the
	// wait with an older row carrying the same text.
	await expect(matching).toHaveCount(before + 1, { timeout: 15_000 });
});

Then('the board is on stream frame {int}', async ({ page, shot }, frame: number) => {
	await expect(page.getByTestId('board-push')).toHaveText(String(frame), { timeout: 15_000 });
	await shot();
});

Then("the board's stream is open", async ({ remotes }) => {
	// Seeing frame 1 proves the document carried the initial value, but it does
	// not prove a slower browser has opened the subscription yet. Wait for the
	// request itself before establishing the no-refetch baseline below.
	await expect
		.poll(() => remotes.urls.some((url) => /GET \/_app\/remote\/[a-z0-9]+\/watchBoard$/.test(url)), {
			timeout: 15_000
		})
		.toBe(true);
});

Then('the board\'s newest todo is {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByTestId('board-newest')).toHaveText(text, { timeout: 15_000 });
	await shot();
});

// The number on the board is the live query's answer; the rows in the other tab
// are `getTodos`' answer, on a different endpoint in a different tab. Asserting
// one against the other compares two real numbers rather than reading the
// board's own arithmetic back to itself.
Then("the board's count is the number of todos in the other tab", async ({ page, shot }) => {
	const rows = await otherTab().getByTestId('todo').count();
	expect(rows, 'the other tab showed no todos at all').toBeGreaterThan(0);
	await expect(page.getByTestId('board-count')).toHaveText(String(rows), { timeout: 15_000 });
	await shot();
});

Then(
	"the document already said the board's newest todo is {string}",
	async ({ documents, shot }, text: string) => {
		const html = await documentText(documents);
		expect(html).toMatch(tagged('strong', 'board-newest', text));
		await shot();
	}
);

Then(
	'the document already said the board is on stream frame {int}',
	async ({ documents, shot }, frame: number) => {
		const html = await documentText(documents);
		expect(html).toMatch(tagged('strong', 'board-push', String(frame)));
		await shot();
	}
);

// The count on /todos is a live query too, and this is the same claim about it:
// the number was in the bytes, not put there by the client afterwards. It is
// asserted against the rows the same document carried, because the store grows
// as the suite runs and a literal would be a fixture with two owners.
Then('the document already carried the todo count', async ({ documents, shot }) => {
	const html = await documentText(documents);
	const count = /<strong data-testid="count">(\d+)<\/strong>/.exec(html);
	expect(count, 'the document carried no todo count at all').not.toBeNull();
	const rows = html.match(/data-testid="todo"/g) ?? [];
	expect(rows.length, 'the document carried no todo rows at all').toBeGreaterThan(0);
	expect(Number(count![1])).toBe(rows.length);
	await shot();
});

Then("the only remote request since was the board's stream", async ({ remotes, shot }) => {
	expect(remotes.urlsSince).toHaveLength(1);
	expect(remotes.urlsSince[0]).toMatch(/^GET \/_app\/remote\/[a-z0-9]+\/watchBoard$/);
	await shot();
});

function otherTab(): Page {
	expect(other.page, 'no other tab was opened').not.toBeNull();
	return other.page!;
}

async function documentText(documents: {
	last: import('@playwright/test').Response | null;
}): Promise<string> {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	return await documents.last!.text();
}
