import type { Page } from '@playwright/test';
import { createBdd } from 'playwright-bdd';
import { copyFile, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, hydrated, test } from './fixtures';
import { tagged } from './ssr';

const { After, Then, When } = createBdd(test);

/** The vite root — the app whose sources a scenario may edit. */
const app = resolve(dirname(fileURLToPath(import.meta.url)), '../../web');

/**
 * What an editing scenario changed, and what it was before. The After hook puts
 * every one of them back and waits until Go is serving the original again, so
 * that the next scenario is not reading the last one's edit.
 */
const edited = new Map<string, string>();
const created = new Set<string>();

/** Steps whose assertions distinguish a live module graph from an embedded build. */

/**
 * The negative half of "Go answered". The generated `.remote.ts` and
 * `+*.server.ts` beside every Go function throw "skgo: implemented in Go", and
 * in dev kit's own module runner is the thing that would call them — so that
 * string appearing anywhere on the page is the stub having run.
 *
 * It is deliberately paired in every scenario with a positive claim about the
 * same page: on its own, "this text is absent" is satisfied by a page that
 * rendered nothing at all.
 */
Then('the page never mentions {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
	expect(await page.locator('body').innerText()).not.toContain(text);
	await shot();
});

/** Something the visitor can read, by its text rather than by a test id. */
Then('I see the words {string}', async ({ page, shot }, text: string) => {
	await expect(page.getByText(text, { exact: false }).first()).toBeVisible({
		timeout: 15_000
	});
	await shot();
});

Then(
	'exactly {int} data request was made since',
	async ({ page, data, shot }, expected: number) => {
		// A second round trip would already be in flight; wait a beat so it
		// lands and can be counted.
		await page.waitForTimeout(500);
		expect(data.since, `data requests: ${data.urlsSince.join(', ') || 'none'}`).toBe(expected);
		await shot('resolved');
	}
);

/**
 * Asks Go for a branch's data the way kit's client does, and reads the node
 * kit's client would render an error page from.
 *
 * `page.request` rather than the standalone `request` fixture, because the
 * session lives in a cookie the browser holds: a separate context would ask as
 * a signed-out visitor and /account/statement would answer with a redirect
 * instead of the 402 the scenario names.
 */
/**
 * Edits one of the app's own sources, the way an editor would.
 *
 * The replacement has to be unambiguous: a `from` that appears twice would make
 * the claim afterwards depend on which occurrence was hit.
 */
When(
	'{string} has {string} replaced with {string}',
	async ({ page }, file: string, from: string, to: string) => {
		// An edit before Kit's router and Vite's HMR client are ready can only
		// prove the next document. Let the same edit also be observable by the
		// already-open page.
		await hydrated(page);
		const path = join(app, file);
		const before = await readFile(path, 'utf-8');
		const occurrences = before.split(from).length - 1;
		expect(occurrences, `${file} contains ${occurrences} copies of ${JSON.stringify(from)}`).toBe(
			1
		);
		if (!edited.has(path)) edited.set(path, before);
		await writeFile(path, before.replace(from, to), 'utf-8');
	}
);

Then(
	'the open page hot-updates to {string} in dev or stays at built heading {string} without reloading',
	async ({ page, documents, browserConsole, shot }, liveHeading: string, builtHeading: string) => {
		const mode = documents.last?.headers()['x-skgo-mode'];
		expect(['dev', 'prod'], `x-skgo-mode was ${JSON.stringify(mode)}`).toContain(mode);
		const expected = mode === 'dev' ? liveHeading : builtHeading;
		await expect(page.getByTestId('title')).toHaveText(expected, {
			timeout: mode === 'dev' ? 30_000 : 2_000
		});
		expect(documents.count, documents.log.join('\n')).toBe(1);
		const hydrationFailures = browserConsole.messages.filter((message) =>
			/hydration (failed|mismatch)|hydration_mismatch/i.test(message)
		);
		expect(hydrationFailures, hydrationFailures.join('\n')).toHaveLength(0);
		await shot(mode === 'dev' ? 'hot-updated' : 'built-unchanged');
	}
);

/**
 * The bytes Go sends now, read as text rather than through a browser: a page
 * that hot-reloaded in the browser would show the edit whether or not Go had
 * it.
 *
 * It polls, because the edit has to reach vite's watcher before Go can be told
 * what to drop, and a fixed sleep is either flaky or slow. The claim is still
 * one that fails: a Go that never picked the edit up never satisfies it.
 */
Then(
	'a new document for {string} carries {string} from live source or {string} from its build',
	async ({ page, shot }, path: string, liveHeading: string, builtHeading: string) => {
		const first = await page.request.get(path);
		const mode = first.headers()['x-skgo-mode'];
		expect(['dev', 'prod'], `x-skgo-mode was ${JSON.stringify(mode)}`).toContain(mode);
		const expected = mode === 'dev' ? liveHeading : builtHeading;
		const rejected = mode === 'dev' ? builtHeading : liveHeading;

		await expect
			.poll(async () => (await page.request.get(path)).text(), {
				timeout: mode === 'dev' ? 30_000 : 2_000,
				intervals: [250, 250, 500, 500, 1000]
			})
			.toMatch(tagged('h1', 'title', expected));

		const html = await (await page.request.get(path)).text();
		expect(html).not.toMatch(tagged('h1', 'title', rejected));
		await page.goto(path);
		await shot(mode === 'dev' ? 'live-source' : 'built-source');
	}
);

/**
 * Waits for the reloads a route change causes to stop. When the route tree
 * moves, kit rewrites its generated client node files and vite broadcasts a
 * full reload for each, in batches about a second apart, so a tab holding the
 * dev client reloads once per batch and a navigation started between two of
 * them is aborted by the second (`page.goto: net::ERR_ABORTED`, seen on CI
 * three times and never on a fast machine). Resolves once the page has gone
 * 1500 ms without navigating, or after 15 s.
 */
async function settled(page: Page) {
	const quiet = 1_500;
	const deadline = Date.now() + 15_000;
	let last = Date.now();
	const bump = () => {
		last = Date.now();
	};
	page.on('framenavigated', bump);
	try {
		while (Date.now() < deadline) {
			if (Date.now() - last >= quiet) return;
			await page.waitForTimeout(100);
		}
	} finally {
		page.off('framenavigated', bump);
	}
}

When('the route fixture is added while the servers keep running', async () => {
	const source = resolve(dirname(fileURLToPath(import.meta.url)), '../fixtures/dev-added');
	const target = join(app, 'src/routes/dev-added');
	await mkdir(target, { recursive: true });
	for (const file of ['+page.svelte', '+page.server.ts']) {
		await copyFile(join(source, file), join(target, file));
	}
	created.add(target);
});

Then(
	'the live dev route renders its Go load while the production build stays unchanged',
	async ({ page, shot }) => {
		const mode = (await page.request.get('/')).headers()['x-skgo-mode'];
		expect(['dev', 'prod'], `x-skgo-mode was ${JSON.stringify(mode)}`).toContain(mode);

		if (mode === 'dev') {
			await expect
				.poll(async () => {
					const response = await page.request.get('/dev-added');
					return { status: response.status(), body: await response.text() };
				}, {
					timeout: 30_000,
					intervals: [250, 250, 500, 500, 1000]
				})
				.toEqual({
					status: 200,
					body: expect.stringContaining('loaded by the already-running Go process')
				});
			// Vite publishes the route manifest and browser graph through separate
			// invalidations. The raw document above proves Go has the route; wait for
			// a navigation that also uses Kit's matching live browser graph. A
			// navigation a reload batch aborts is retried, not failed.
			await settled(page);
			await expect
				.poll(async () => {
					try {
						await page.goto('/dev-added');
					} catch {
						return null;
					}
					return page.getByTestId('title').textContent();
				}, {
					timeout: 30_000,
					intervals: [250, 250, 500, 500, 1000]
				})
				.toBe('Added while running');
			await expect(page.getByTestId('live-route-load')).toHaveText(
				'loaded by the already-running Go process'
			);
			expect(await page.locator('style[data-sveltekit]').textContent()).toContain('color: #176b47');
		} else {
			const response = await page.goto('/dev-added');
			expect(response?.status()).toBe(404);
			expect(await page.locator('body').innerText()).not.toContain('Added while running');
		}
		await shot(mode === 'dev' ? 'live-route' : 'built-route');
	}
);

After(async ({ page }) => {
	const changedRoutes = created.size > 0;
	for (const path of created) {
		await rm(path, { recursive: true, force: true });
	}
	created.clear();
	for (const [path, before] of edited) {
		await writeFile(path, before, 'utf-8');
	}
	const changedSource = edited.size > 0;
	edited.clear();
	if (!changedRoutes && !changedSource) return;
	// Wait for Go to be serving the restored source again, so the next scenario
	// does not read this one's edit or a transiently renumbered route graph.
	await expect
		.poll(async () => (await page.request.get('/')).text(), {
			timeout: 30_000,
			intervals: [250, 250, 500, 500, 1000]
		})
		.toMatch(tagged('h1', 'title', 'Home'));
	if (changedRoutes) {
		// Kit rewrites its generated browser graph separately from the manifest
		// snapshot Go consumes. Do not let the next scenario begin until a fresh
		// browser can hydrate the restored graph as Home as well.
		await settled(page);
		await page.goto('/');
		await hydrated(page);
		await expect(page.getByTestId('title')).toHaveText('Home');
	}
});
