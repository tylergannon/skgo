import { createBdd } from 'playwright-bdd';
import { readFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, hydrated, test } from './fixtures';
import { documentText } from './ssr';

const { Given, Then } = createBdd(test);

/** The vite root — the app whose components declare the rules asserted here. */
const app = resolve(dirname(fileURLToPath(import.meta.url)), '../../web');

/**
 * A declaration the app makes, in the document the app was served by.
 *
 * The component is named so the fixture can be checked before the claim is: a
 * declaration this file believes in but the app no longer makes would otherwise
 * be a scenario that fails for the wrong reason, or — worse — one nobody could
 * tell from a document that carries no styles at all.
 */
Then(
	'the document carries {string} from {string}',
	async ({ documents, shot }, rule: string, file: string) => {
		const source = await readFile(join(app, file), 'utf-8');
		expect(source, `${file} no longer declares ${JSON.stringify(rule)}`).toContain(rule);
		expect(await documentText(documents)).toContain(rule);
		await shot();
	}
);

/**
 * What the browser drew, which no amount of markup can account for on its own.
 *
 * The root layout gives the nav `display: flex` and a rem of padding above and
 * below; without those rules its links are inline text and it is about as tall
 * as one line. The number is the scenario's, not the page's.
 */
Then("the app's nav is more than {int} pixels tall", async ({ page, shot }, pixels: number) => {
	await measure(page, '[data-testid="app-nav"]', pixels);
	await shot('styled');
});

/**
 * The same claim for the front page's own component: its title is sized with a
 * `clamp()` no default stylesheet would produce, and unstyled it is one 2em
 * heading.
 */
Then("the page's title is more than {int} pixels tall", async ({ page, shot }, pixels: number) => {
	await measure(page, '[data-testid="title"]', pixels);
	await shot('title');
});

async function measure(
	page: import('@playwright/test').Page,
	selector: string,
	pixels: number
): Promise<void> {
	const element = page.locator(selector).first();
	await expect(element).toBeVisible({ timeout: 15_000 });
	const box = await element.boundingBox();
	expect(box, `${selector} has no box`).not.toBeNull();
	expect(box!.height, `${selector} is ${box!.height}px tall`).toBeGreaterThan(pixels);
}

/**
 * Positive evidence that this context runs nothing, rather than trust in the
 * project's configuration: the boot script's very first statement assigns the
 * `__sveltekit_dev` global, so a context that ran any of it would have one.
 */
Then('the browser ran no script at all', async ({ page }) => {
	const booted = await page.evaluate(() =>
		Object.keys(globalThis).some((key) => key.startsWith('__sveltekit_'))
	);
	expect(booted, 'the boot script ran, so this is not a scriptless context').toBe(false);
});

/**
 * A session for a browser that cannot make one.
 *
 * Signing in is a command kit's client sends, so a scriptless context has no
 * way to start a session at all. A second context that does run script signs in
 * the way every other scenario does, and this one is handed the cookie it came
 * back with — which is the only thing the server ever looks at.
 */
Given(
	'a signed-in visitor whose browser runs no script',
	async ({ page, browser }) => {
		// `javaScriptEnabled` explicitly, because the `browser` fixture applies
		// this project's own context options to every context made from it —
		// and this project's are the ones that turn scripting off.
		const scripted = await browser.newContext({
			baseURL: process.env.BASE_URL,
			javaScriptEnabled: true
		});
		try {
			const helper = await scripted.newPage();
			await helper.goto('/todos');
			await hydrated(helper);
			await helper.getByTestId('user').fill('ada');
			await helper.getByTestId('sign-in').click();
			await expect(helper.getByTestId('session')).toHaveText('Signed in as ada', {
				timeout: 15_000
			});
			await page.context().addCookies(await scripted.cookies());
		} finally {
			await scripted.close();
		}
	}
);
