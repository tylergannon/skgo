import { createBdd } from 'playwright-bdd';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

// By its own name, inside the index — the root layout's nav carries links of its
// own and several of them name the same pages.
When('I open the capability {string}', async ({ page, $testInfo }, capability: string) => {
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
	if ($testInfo.project.name !== 'noscript') await hydrated(page);
	await link.click();
	await page.waitForURL(
		(url) => url.pathname === target.pathname && url.search === target.search,
		{ timeout: 15_000 }
	);
});

// The root layout's load, seen from the page rather than from the bytes.
Then('the app says it is deployed as {string}', async ({ page }, deployment: string) => {
	await expect(page.getByTestId('deployment')).toHaveText(deployment, { timeout: 15_000 });
});
