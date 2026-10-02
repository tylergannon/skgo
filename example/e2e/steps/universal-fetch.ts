import { createBdd, type DataTable } from 'playwright-bdd';
import { booted, expect, hydrated, test } from './fixtures';

const { Given, When, Then } = createBdd(test);

const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// The fixture already named this scenario's run; the step is where the
// scenario says it counts only its own traffic.
Given('I use a fresh replay run', async ({ replayRun }) => {
	expect(replayRun.id).toMatch(/^r[a-z0-9]+$/);
});

When(
	'I visit the universal fetch page at step {int}',
	async ({ page, replayRun }, step: number) => {
		await page.goto(`/universal-fetch?run=${replayRun.id}&step=${step}`);
	}
);

When('I follow the universal fetch next step link', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('next-step').click();
});

// What the bytes said, before a line of script ran: each element's whole text,
// the same way for a built document and a dev one that adds a scoping class.
Then('the document already said these values:', async ({ documents }, table: DataTable) => {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	const html = await documents.last!.text();
	for (const [testid, text] of table.rows()) {
		expect(html, testid).toMatch(
			new RegExp(`data-testid="${escape(testid)}"[^>]*>${escape(text)}<`)
		);
	}
});

Then('the page shows these values:', async ({ page }, table: DataTable) => {
	for (const [testid, text] of table.rows()) {
		await expect(page.getByTestId(testid)).toHaveText(text, { timeout: 15_000 });
	}
});

Then('neither the document nor the page carries {string}', async ({ page, documents }, secret: string) => {
	await booted(page);
	expect(await documents.last!.text()).not.toContain(secret);
	expect(await page.content()).not.toContain(secret);
});

// The server counts every request to its own endpoints that names this run.
// The table is the whole of what it has seen, so an endpoint that is missing
// from it must not have been called at all, and the numbers must hold still.
Then(
	'the replay endpoints have been called exactly:',
	async ({ page, replayRun }, table: DataTable) => {
		await booted(page);
		const want: Record<string, number> = {};
		for (const [request, count] of table.rows()) want[request] = Number(count);
		const read = async () => {
			const response = await page.request.get(`/api/replay-count?run=${replayRun.id}`);
			expect(response.status()).toBe(200);
			return (await response.json()) as Record<string, number>;
		};
		await expect.poll(read, { timeout: 15_000 }).toEqual(want);
		// A second request already in flight would land in this beat.
		await page.waitForTimeout(500);
		expect(await read()).toEqual(want);
	}
);
