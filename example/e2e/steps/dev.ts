import { createBdd } from 'playwright-bdd';
import type { Browser, Page, Response } from '@playwright/test';
import { existsSync } from 'node:fs';
import { mkdir, readdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, expectedMode, expectMode, hydrated, booted, test } from './fixtures';
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
const createdFiles = new Set<string>();

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
Then('the page never mentions {string}', async ({ page }, text: string) => {
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
	expect(await page.locator('body').innerText()).not.toContain(text);
});

/** Something the visitor can read, by its text rather than by a test id. */
Then('I see the words {string}', async ({ page }, text: string) => {
	await expect(page.getByText(text, { exact: false }).first()).toBeVisible({
		timeout: 15_000
	});
});

Then(
	'exactly {int} data request was made since',
	async ({ page, data }, expected: number) => {
		// A second round trip would already be in flight; wait a beat so it
		// lands and can be counted.
		await page.waitForTimeout(500);
		expect(data.since, `data requests: ${data.urlsSince.join(', ') || 'none'}`).toBe(expected);
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
	async ({ page, documents, browserConsole }, liveHeading: string, builtHeading: string) => {
		expect(documents.last, 'no document response was observed').not.toBeNull();
		const mode = expectMode(documents.last!);
		const expected = mode === 'dev' ? liveHeading : builtHeading;
		await expect(page.getByTestId('title')).toHaveText(expected, {
			timeout: mode === 'dev' ? 30_000 : 2_000
		});
		expect(documents.count, documents.log.join('\n')).toBe(1);
		const hydrationFailures = browserConsole.messages.filter((message) =>
			/hydration (failed|mismatch)|hydration_mismatch/i.test(message)
		);
		expect(hydrationFailures, hydrationFailures.join('\n')).toHaveLength(0);
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
	async ({ page }, path: string, liveHeading: string, builtHeading: string) => {
		const first = await page.request.get(path);
		const mode = expectMode(first);
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
	}
);

const fixtures = resolve(dirname(fileURLToPath(import.meta.url)), '../fixtures');
const routes = join(app, 'src/routes');
const developmentTimeout = 120_000;
const intervals = [250, 250, 500, 500, 1000];

/**
 * Saves an authored file from a fixture, the way an editor would. The fixtures
 * are `.txt` so that nothing compiles them before a scenario writes them: no
 * binary the dev server built earlier can contain what they say.
 */
When('{string} is replaced by the fixture {string}', async ({ page }, file: string, fixture: string) => {
	await hydrated(page);
	const path = join(app, file);
	const before = await readFile(path, 'utf-8');
	if (!edited.has(path)) edited.set(path, before);
	await writeFile(path, await readFile(join(fixtures, fixture), 'utf-8'), 'utf-8');
});

When(
	'the route {string} is created from the fixture directory {string}',
	async ({ page }, route: string, directory: string) => {
		await hydrated(page);
		const target = join(routes, route);
		expect(existsSync(target), `${target} already exists`).toBe(false);
		created.add(target);
		await mkdir(target, { recursive: true });
		for (const name of await readdir(join(fixtures, directory))) {
			await writeFile(
				join(target, name.replace(/\.txt$/, '')),
				await readFile(join(fixtures, directory, name), 'utf-8'),
				'utf-8'
			);
		}
	}
);

When('the route {string} is removed', async ({ page }, route: string) => {
	await hydrated(page);
	const target = join(routes, route);
	expect(existsSync(target), `${target} does not exist`).toBe(true);
	await rm(target, { recursive: true });
	created.delete(target);
});


When('the file {string} is written from the fixture {string}', async ({ page }, file: string, fixture: string) => {
	await hydrated(page);
	const path = join(app, file);
	if (!existsSync(path)) createdFiles.add(path);
	else if (!createdFiles.has(path) && !edited.has(path)) edited.set(path, await readFile(path, 'utf-8'));
	await writeFile(path, await readFile(join(fixtures, fixture), 'utf-8'), 'utf-8');
});

Then(
	'the enhanced Go development form answers {string} in dev or {string} in prod',
	async ({ page, documents }, live: string, built: string) => {
		const mode = expectedMode();
		let documentsDuring = -1;
		await expect
			.poll(
				async () => {
					try {
						await page.goto('/go-dev');
						await hydrated(page);
						const before = documents.count;
						await page.getByTestId('enhanced-go-dev-form').getByRole('button').click();
						await expect(page.getByTestId('go-dev-action')).toBeVisible({ timeout: 10_000 });
						documentsDuring = documents.count - before;
						return await page.getByTestId('go-dev-action').textContent();
					} catch (error) {
						const message = String(error);
						if (message.includes('net::ERR_ABORTED') || message.includes('interrupted') || message.includes('Timeout') || message.includes('Execution context was destroyed')) return null;
						throw error;
					}
				},
				{ timeout: mode === 'dev' ? developmentTimeout : 15_000, intervals }
			)
			.toBe(mode === 'dev' ? live : built);
		expect(documentsDuring, "Kit's enhanced submit loaded a document").toBe(0);
	}
);

/** A real browser context that runs no script posts the form natively. */
async function nativeAnswer(browser: Browser): Promise<string> {
	const context = await browser.newContext({ baseURL: process.env.BASE_URL, javaScriptEnabled: false });
	try {
		const page = await context.newPage();
		await page.goto('/go-dev');
		await Promise.all([
			page.waitForURL(/\/go-dev\?\/revise$/),
			page.getByTestId('native-go-dev-form').getByRole('button').click()
		]);
		return (await page.getByTestId('go-dev-action').textContent()) ?? '';
	} finally {
		await context.close();
	}
}

Then(
	'the native Go development form with scripting disabled answers {string} in dev or {string} in prod',
	async ({ browser }, live: string, built: string) => {
		const mode = expectedMode();
		await expect
			.poll(() => nativeAnswer(browser).catch(() => ''), {
				timeout: mode === 'dev' ? developmentTimeout : 15_000,
				intervals
			})
			.toBe(mode === 'dev' ? live : built);
	}
);

async function endpointAnswer(page: Page, path: string) {
	const response = await page.request.get(path);
	return {
		status: response.status(),
		body: await response.text(),
		revision: response.headers()['x-go-revision'] ?? ''
	};
}

Then(
	'the endpoint {string} answers {int} {string} with header {string} in dev or {int} {string} with header {string} in prod',
	async (
		{ page },
		path: string,
		liveStatus: number,
		liveBody: string,
		liveHeader: string,
		builtStatus: number,
		builtBody: string,
		builtHeader: string
	) => {
		const mode = expectedMode();
		const expected =
			mode === 'dev'
				? { status: liveStatus, body: liveBody, revision: liveHeader }
				: { status: builtStatus, body: builtBody, revision: builtHeader };
		await expect
			.poll(() => endpointAnswer(page, path), {
				timeout: mode === 'dev' ? developmentTimeout : 2_000,
				intervals
			})
			.toEqual(expected);
	}
);

Then(
	'in dev the endpoint {string} answers {int} {string} with header {string}',
	async ({ page }, path: string, status: number, body: string, revision: string) => {
		if (expectedMode() !== 'dev') return;
		await expect
			.poll(() => endpointAnswer(page, path), { timeout: developmentTimeout, intervals })
			.toEqual({ status, body, revision });
	}
);

Then(
	"the hydrated page's fetch of the endpoint reads status {int} body {string} header {string} in dev or status {int} body {string} header {string} in prod",
	async (
		{ page },
		liveStatus: number,
		liveBody: string,
		liveHeader: string,
		builtStatus: number,
		builtBody: string,
		builtHeader: string
	) => {
		const mode = expectedMode();
		const expected =
			mode === 'dev'
				? { status: liveStatus, body: liveBody, revision: liveHeader }
				: { status: builtStatus, body: builtBody, revision: builtHeader };
		await expect
			.poll(
				async () => {
					try {
						await page.goto('/go-dev');
						await hydrated(page);
						await page.getByTestId('go-dev-endpoint-button').click();
						await expect(page.getByTestId('go-dev-endpoint')).not.toHaveText('No endpoint request yet.', { timeout: 10_000 });
						return JSON.parse((await page.getByTestId('go-dev-endpoint').textContent()) ?? 'null');
					} catch (error) {
						const message = String(error);
						if (message.includes('Timeout') || message.includes('interrupted') || message.includes('net::ERR_ABORTED') || message.includes('Execution context was destroyed')) return null;
						throw error;
					}
				},
				{ timeout: mode === 'dev' ? developmentTimeout : 15_000, intervals }
			)
			.toEqual(expected);
	}
);

/**
 * Kit reloads app-template edits and Vite reloads its generated route graph.
 * HTTP readiness does not mean the open browser has consumed either event:
 * its reload can replace our navigation. Require the full load event with a
 * bounded timeout: a stalled module request is a failure, not readiness. Only
 * explicit navigation interruptions are retried; callers assert visible state.
 */
async function editedDocument(page: Page, path: string, live: boolean): Promise<Response> {
	if (!live) {
		const response = await page.goto(path);
		expect(response, `no document response for ${path}`).not.toBeNull();
		return response!;
	}
	let response: Response | null = null;
	await expect
		.poll(async () => {
			response = await tryDocumentNavigation(page, path);
			return response !== null;
		}, { timeout: developmentTimeout, intervals, message: `no completed document navigation to ${path}` })
		.toBe(true);
	return response!;
}

async function tryDocumentNavigation(page: Page, path: string): Promise<Response | null> {
	try {
		return await page.goto(path, { waitUntil: 'load', timeout: 15_000 });
	} catch (error) {
		if (/net::ERR_ABORTED|is interrupted by another navigation/.test(String(error))) return null;
		throw error;
	}
}

async function templateMarker(page: Page, path: string): Promise<string> {
	const response = await page.request.get(path);
	const match = (await response.text()).match(/<meta name="go-template" content="([^"]*)"/);
	return match ? match[1] : '';
}

Then(
	'a cold document for {string} has the template marker {string} in dev or {string} in prod',
	async ({ page }, path: string, live: string, built: string) => {
		const mode = expectedMode();
		await expect
			.poll(() => templateMarker(page, path), {
				timeout: mode === 'dev' ? developmentTimeout : 2_000,
				intervals
			})
			.toBe(mode === 'dev' ? live : built);
		const response = await editedDocument(page, path, mode === 'dev');
		expect(response.status()).toBe(200);
		await expect(page).toHaveURL(path);
		await expect(page.locator('meta[name="go-template"]')).toHaveAttribute('content', mode === 'dev' ? live : built);
		await expect(page.getByTestId('app-nav')).toBeVisible();
		await booted(page);
		if (path === '/go-dev') await expect(page.getByTestId('go-dev-load')).toHaveText('Go revision one');
	}
);

Then(
	'a cold document for {string} has no script in dev or a script in prod',
	async ({ page }, path: string) => {
		const mode = expectedMode();
		const scripts = async () => ((await (await page.request.get(path)).text()).match(/<script/g) ?? []).length;
		if (mode === 'dev') {
			await expect.poll(scripts, { timeout: developmentTimeout, intervals }).toBe(0);
		} else {
			expect(await scripts()).toBeGreaterThan(0);
		}
		await editedDocument(page, path, mode === 'dev');
		await expect(page.getByTestId('go-dev-load')).toHaveText('Go revision one');
		if (mode === 'dev') expect(await page.locator('script').count()).toBe(0);
		else expect(await page.locator('script').count()).toBeGreaterThan(0);
	}
);

Then(
	'the enhanced Go development form submits as a document in dev or as a Kit fetch in prod',
	async ({ page, documents }) => {
		const mode = expectedMode();
		await page.goto('/go-dev');
		await hydrated(page);
		const before = documents.count;
		await page.getByTestId('enhanced-go-dev-form').getByRole('button').click();
		await expect(page.getByTestId('go-dev-action')).toBeVisible({ timeout: 15_000 });
		expect(documents.count - before, documents.log.join('\n')).toBe(mode === 'dev' ? 1 : 0);
	}
);

Then(
	'a cold document for {string} has no {string} markup in dev or has it in prod',
	async ({ page }, path: string, testid: string) => {
		const mode = expectedMode();
		await expect
			.poll(() => documentSays(page, path, testid), {
				timeout: mode === 'dev' ? developmentTimeout : 2_000,
				intervals
			})
			.toBe(mode === 'dev' ? '' : 'Go revision one');
	}
);

Then(
	'Kit renders {string} as {string} from the data Go answers',
	async ({ page, data }, testid: string, text: string) => {
		await page.goto('/go-dev');
		await hydrated(page);
		await expect(page.getByTestId(testid)).toHaveText(text, { timeout: 15_000 });
		if (expectedMode() === 'dev') {
			expect(data.urls.join('\n'), 'Kit never asked Go for the page data').toContain('/go-dev/__data.json');
		}
	}
);

/** What a served page says in one test id, read from the bytes Go sent. */
async function documentSays(page: Page, path: string, testid: string): Promise<string> {
	const response = await page.request.get(path);
	if (response.status() !== 200) return `${response.status()}`;
	const match = (await response.text()).match(
		new RegExp(`data-testid="${testid}"[^>]*>([^<]*)<`)
	);
	return match ? match[1].replaceAll('&quot;', '"') : '';
}

/**
 * Cold documents are read as text so a page that merely hot-reloaded in the
 * browser cannot satisfy them; the browser then loads the same document and
 * must hydrate it to the same literal.
 */
async function coldDocument(page: Page, path: string, testid: string, expected: string, live: boolean) {
	await expect
		.poll(() => documentSays(page, path, testid), {
			timeout: live ? developmentTimeout : 2_000,
			intervals
		})
		.toBe(expected);
	await editedDocument(page, path, live);
	await expect(page.getByTestId(testid)).toHaveText(expected);
	await hydrated(page);
	await expect(page.getByTestId(testid)).toHaveText(expected);
}

Then(
	'a cold document for {string} has {string} reading {string} in dev or {string} in prod',
	async ({ page }, path: string, testid: string, live: string, built: string) => {
		const mode = expectedMode();
		await coldDocument(page, path, testid, mode === 'dev' ? live : built, mode === 'dev');
	}
);

Then(
	'in dev a cold document for {string} has {string} reading {string}',
	async ({ page }, path: string, testid: string, live: string) => {
		const mode = expectedMode();
		if (mode === 'dev') await coldDocument(page, path, testid, live, true);
	}
);

/**
 * Kit's own client navigates, rather than the browser loading a document: a
 * marker on `window` survives only if no document was loaded, and the data or
 * remote request that supplied the value is on record. Kit's generated browser
 * graph can reload the client while a route is added; only that transient is
 * retried.
 */
async function clientNavigation(
	page: Page,
	data: { mark(): void; readonly since: number },
	remotes: { mark(): void; readonly since: number },
	from: string,
	to: string,
	testid: string,
	expected: string,
	live: boolean
) {
	await expect
		.poll(
			async () => {
				try {
					await page.goto(from);
					await hydrated(page);
					await page.evaluate((href) => {
						(window as unknown as { __skgoNavigation: string }).__skgoNavigation = 'client';
						const anchor = document.createElement('a');
						anchor.id = 'skgo-dev-link';
						anchor.href = href;
						anchor.textContent = 'navigate';
						document.body.append(anchor);
					}, to);
					data.mark();
					remotes.mark();
					await page.locator('#skgo-dev-link').click();
					await expect(page.getByTestId(testid)).toBeVisible({ timeout: 10_000 });
					return await page.getByTestId(testid).textContent();
				} catch (error) {
					const message = String(error);
					if (
						message.includes('net::ERR_ABORTED') ||
						message.includes('is interrupted by another navigation') ||
						message.includes('Execution context was destroyed') ||
						message.includes('Timeout')
					) {
						return null;
					}
					throw error;
				}
			},
			{ timeout: live ? developmentTimeout : 15_000, intervals }
		)
		.toBe(expected);
	expect(
		await page.evaluate(
			() => (window as unknown as { __skgoNavigation?: string }).__skgoNavigation
		),
		'the navigation loaded a document instead of Kit navigating on the client'
	).toBe('client');
	expect(
		data.since + remotes.since,
		'nothing was fetched from Go for the navigation'
	).toBeGreaterThan(0);
}

Then(
	'client navigation from {string} to {string} shows {string} as {string} in dev or {string} in prod',
	async (
		{ page, data, remotes },
		from: string,
		to: string,
		testid: string,
		live: string,
		built: string
	) => {
		const mode = expectedMode();
		await clientNavigation(page, data, remotes, from, to, testid, mode === 'dev' ? live : built, mode === 'dev');
	}
);

Then(
	'in dev client navigation from {string} to {string} shows {string} as {string}',
	async ({ page, data, remotes }, from: string, to: string, testid: string, live: string) => {
		const mode = expectedMode();
		if (mode === 'dev') await clientNavigation(page, data, remotes, from, to, testid, live, true);
	}
);

Then(
	'the generated file {string} contains {string} in dev or {string} in prod',
	async ({ page }, file: string, live: string, built: string) => {
		const mode = expectedMode();
		const expected = mode === 'dev' ? live : built;
		await expect
			.poll(async () => (await readFile(join(app, file), 'utf-8')).includes(expected), {
				timeout: mode === 'dev' ? developmentTimeout : 2_000,
				intervals,
				message: `${file} never contained ${JSON.stringify(expected)}`
			})
			.toBe(true);
	}
);

Then(
	'the generated file {string} contains {string} in dev or does not exist in prod',
	async ({ page }, file: string, live: string) => {
		const mode = expectedMode();
		if (mode === 'prod') {
			expect(existsSync(join(app, file)), `${file} exists`).toBe(false);
			return;
		}
		await expect
			.poll(async () => existsSync(join(app, file)) && (await readFile(join(app, file), 'utf-8')).includes(live), {
				timeout: developmentTimeout,
				intervals,
				message: `${file} never contained ${JSON.stringify(live)}`
			})
			.toBe(true);
	}
);

Then(
	'a cold request for {string} is answered {int} in dev and {int} in prod',
	async ({ page }, path: string, live: number, built: number) => {
		const mode = expectedMode();
		const expected = mode === 'dev' ? live : built;
		await expect
			.poll(async () => (await page.request.get(path)).status(), {
				timeout: mode === 'dev' ? developmentTimeout : 2_000,
				intervals
			})
			.toBe(expected);
		const response = await editedDocument(page, path, mode === 'dev');
		expect(response.status()).toBe(expected);
		await expect(page).toHaveURL(path);
		if (path === '/go-dev-endpoint' && expected === 200) {
			expect(response.headers()['content-type']).toMatch(/^text\/plain/);
			expect(response.headers()['x-go-revision']).toBe('new-one');
			await expect(page.locator('body')).toHaveText('New Go endpoint revision one');
		} else {
			await booted(page);
			if (expected === 404) await expect(page.getByRole('heading', { name: '404', exact: true })).toBeVisible();
		}
	}
);

/**
 * The failure a developer sees: the page's request is answered by the build
 * diagnostic, naming the authored file, and not by the last good page.
 */
Then(
	'a request for {string} is answered by the build failure naming {string} in dev or by {string} reading {string} in prod',
	async ({ page }, path: string, file: string, testid: string, built: string) => {
		const mode = expectedMode();
		if (mode === 'prod') {
			expect(await documentSays(page, path, testid)).toBe(built);
			return;
		}
		let failure: { status: number; header: string | undefined; body: string } | null = null;
		await expect
			.poll(
				async () => {
					const response = await page.request.get(path);
					failure = {
						status: response.status(),
						header: response.headers()['x-skgo-dev-error'],
						body: await response.text()
					};
					return failure.status;
				},
				{ timeout: developmentTimeout, intervals }
			)
			.toBe(500);
		expect(failure!.header, 'the failure is not marked as a build failure').toBeTruthy();
		expect(failure!.body).toContain(file);
		expect(failure!.body).not.toContain('Go revision');
	}
);

After(async ({ page }) => {
	const changedRoutes = created.size > 0;
	const removedRoutes = [...created];
	const removedFiles = [...createdFiles];
	for (const path of createdFiles) {
		await rm(path, { force: true });
	}
	createdFiles.clear();
	for (const path of created) {
		await rm(path, { recursive: true, force: true });
	}
	created.clear();
	const restored = [...edited.keys()];
	for (const [path, before] of edited) {
		await writeFile(path, before, 'utf-8');
	}
	const changedSource = edited.size > 0;
	edited.clear();
	if (!changedRoutes && !changedSource && removedFiles.length === 0) return;
	// Wait for Go to be serving the restored source again, so the next scenario
	// does not read this one's edit or a transiently renumbered route graph.
	await expect
		.poll(async () => (await page.request.get('/')).text(), {
			timeout: developmentTimeout,
			intervals
		})
		.toMatch(tagged('h1', 'title', 'Home'));
	if (restored.some((path) => path.endsWith('.go')) || removedRoutes.length > 0) {
		await expect
			.poll(() => documentSays(page, '/go-dev', 'go-dev-load'), { timeout: developmentTimeout, intervals })
			.toBe('Go revision one');
		await expect
			.poll(() => documentSays(page, '/go-dev', 'go-dev-remote'), { timeout: developmentTimeout, intervals })
			.toBe('"Go wire revision one"');
		for (const path of removedRoutes) {
			const route = '/' + path.slice(routes.length + 1);
			await expect
				.poll(async () => (await page.request.get(route)).status(), { timeout: developmentTimeout, intervals })
				.toBe(404);
		}
	}
	if (expectedMode() === 'dev') {
		if (restored.some((path) => path.endsWith('endpoint/server.go'))) {
			await expect
				.poll(() => endpointAnswer(page, '/go-dev/endpoint'), { timeout: developmentTimeout, intervals })
				.toEqual({ status: 200, body: 'Go endpoint revision one', revision: 'one' });
		}
		if (restored.some((path) => path.endsWith('go-dev/page.server.go'))) {
			await expect
				.poll(
					async () =>
						(
							await page.request.post('/go-dev?/revise', {
								headers: { accept: 'application/json', 'x-sveltekit-action': 'true' },
								form: {}
							})
						).text(),
					{ timeout: developmentTimeout, intervals }
				)
				.toContain('Go action revision one');
		}
		if (restored.some((path) => path.endsWith('app.html'))) {
			await expect
				.poll(() => templateMarker(page, '/go-dev'), { timeout: developmentTimeout, intervals })
				.toBe('Go template revision one');
		}
		if (removedFiles.length > 0) {
			await expect
				.poll(async () => (await documentSays(page, '/go-dev', 'go-dev-load')), { timeout: developmentTimeout, intervals })
				.toBe('Go revision one');
			await expect
				.poll(async () => ((await (await page.request.get('/go-dev')).text()).match(/<script/g) ?? []).length, { timeout: developmentTimeout, intervals })
				.toBeGreaterThan(0);
		}
	}
	if (changedRoutes) {
		// Kit rewrites its generated browser graph separately from the manifest
		// snapshot Go consumes. Do not let the next scenario begin until a fresh
		// browser can hydrate the restored graph as Home as well.
		await navigateThroughRouteUpdate(page, '/', 'Home');
		await hydrated(page);
		await expect(page.getByTestId('title')).toHaveText('Home');
	}
});

/**
 * Kit invalidates the generated browser graph when a route is added or removed.
 * If that invalidation lands during a navigation, Vite aborts it and reloads
 * the client. Retry only that deliberate transient; every other navigation
 * error remains a failure, and success still requires the expected live page.
 */
async function navigateThroughRouteUpdate(page: Page, path: string, title: string): Promise<void> {
	await expect
		.poll(
			async () => {
				const response = await tryDocumentNavigation(page, path);
				if (response?.status() !== 200) return null;
				return await page.getByTestId('title').textContent();
			},
			{ timeout: 30_000, intervals: [250, 250, 500, 500, 1000] }
		)
		.toBe(title);
}
