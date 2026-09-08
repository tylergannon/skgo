import { createBdd, DataTable } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

/**
 * The index, row by row, in the order a visitor reads it.
 *
 * The expected rows come from the feature file. Reading the page and comparing
 * it with itself would pass over an empty list and go on passing after somebody
 * deleted an entry, which is the one thing this scenario exists to catch.
 */
Then(
	'the front page lists exactly these capabilities, in this order:',
	async ({ page, shot }, table: DataTable) => {
		const expected = table.hashes();
		const rows = page.getByTestId('capability');
		await expect(rows).toHaveCount(expected.length);

		const names = await rows.getByTestId('capability-link').allTextContents();
		expect(names.map((name) => name.trim())).toEqual(expected.map((row) => row.capability));

		const paths = await rows.getByTestId('capability-link').evaluateAll((links) =>
			links.map((link) => new URL((link as HTMLAnchorElement).href).pathname + new URL((link as HTMLAnchorElement).href).search)
		);
		expect(paths).toEqual(expected.map((row) => row.path));
		await shot('index');
	}
);

// An index whose entries say only where they go is a nav. Each row has to say
// what the page proves, in a sentence — so this asserts there is one per row and
// that it is a sentence rather than a word.
Then('every entry says what to look for', async ({ page, shot }) => {
	const rows = page.getByTestId('capability');
	const count = await rows.count();
	expect(count, 'the front page listed nothing at all').toBeGreaterThan(0);

	const lines = await rows.getByTestId('capability-look').allTextContents();
	expect(lines).toHaveLength(count);
	for (const [i, line] of lines.entries()) {
		expect(line.trim().length, `entry ${i + 1} says nothing about what to look for`).toBeGreaterThan(
			30
		);
	}
	await shot('what-to-look-for');
});

// By its own name, inside the index — the root layout's nav carries links of its
// own and several of them name the same pages.
When('I open the capability {string}', async ({ page }, capability: string) => {
	const link = page
		.getByTestId('capabilities')
		.getByRole('link', { name: capability, exact: true });
	await expect(link).toHaveCount(1);

	// Where the entry says it goes, read before the click so the step can wait
	// for the browser to actually be there. One of these entries is a document
	// request rather than a client-side navigation, and `click()` returns
	// before that response has arrived — without this the step after it looks
	// at the document the visitor was on a moment ago.
	const target = new URL((await link.getAttribute('href'))!, page.url());
	await hydrated(page);
	await link.click();
	await page.waitForURL(
		(url) => url.pathname === target.pathname && url.search === target.search,
		{ timeout: 15_000 }
	);
});

// The root layout's load, seen from the page rather than from the bytes.
Then('the app says it is deployed as {string}', async ({ page, shot }, deployment: string) => {
	await expect(page.getByTestId('deployment')).toHaveText(deployment, { timeout: 15_000 });
	await shot();
});
