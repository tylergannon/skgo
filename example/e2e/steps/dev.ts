import { createBdd } from 'playwright-bdd';
import { mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { expect, test } from './fixtures';
import { tagged } from './ssr';

const { After, Given, Then, When } = createBdd(test);

/** The vite root — the app whose sources a scenario may edit. */
const app = resolve(dirname(fileURLToPath(import.meta.url)), '../../web');

/**
 * What an editing scenario changed, and what it was before. The After hook puts
 * every one of them back and waits until Go is serving the original again, so
 * that the next scenario is not reading the last one's edit.
 */
const edited = new Map<string, string>();

/**
 * The route directories an editing scenario created. The After hook deletes
 * them and waits until Go is answering 404 for them again, so the next scenario
 * is not running against the last one's route tree — and so the checkout the
 * suite ran in is the checkout it started with.
 */
const added = new Set<string>();

/**
 * The steps dev.feature needs and prod has no use for.
 *
 * In dev the document is kit's shell, so nothing can be claimed about its
 * bytes; what is left to assert is that the page filled itself in from Go.
 * Two kinds of claim do that here: what the visitor can see, and what Go's own
 * data endpoint says when it is asked directly.
 */

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
Then(
	"Go's data endpoint for {string} answered {int}",
	async ({ page, shot }, path: string, status: number) => {
		const body = await dataResponse(page, path);
		const nodes = body.nodes as Array<{ type?: string; error?: { status?: number } } | null>;
		expect(Array.isArray(nodes), `the data response was ${JSON.stringify(body)}`).toBe(true);
		const failed = nodes.find((node) => node?.type === 'error');
		expect(failed, `no node in the branch failed: ${JSON.stringify(body)}`).toBeTruthy();
		expect(failed!.error?.status).toBe(status);
		await appIsUp(page);
		await shot();
	}
);

Then(
	"Go's data endpoint for {string} answers with a redirect to {string}",
	async ({ page, shot }, path: string, location: string) => {
		const body = await dataResponse(page, path);
		expect(body.type).toBe('redirect');
		expect(body.location).toBe(location);
		await appIsUp(page);
		await shot();
	}
);

/**
 * Waits for the shell to have become a page before a frame is taken of it. The
 * claims above are about a response the step already read, so the picture may
 * as well be of something; without this it is a photograph of a blank document.
 */
async function appIsUp(page: import('@playwright/test').Page) {
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
}

async function dataResponse(
	page: import('@playwright/test').Page,
	path: string
): Promise<Record<string, unknown>> {
	const url = `${path === '/' ? '' : path}/__data.json`;
	const response = await page.request.get(url);
	const text = await response.text();
	expect(
		response.headers()['content-type'] ?? '',
		`${url} answered ${response.status()} with ${text.slice(0, 200)}`
	).toContain('application/json');
	return JSON.parse(text) as Record<string, unknown>;
}


/**
 * Edits one of the app's own sources, the way an editor would.
 *
 * The replacement has to be unambiguous: a `from` that appears twice would make
 * the claim afterwards depend on which occurrence was hit.
 */
When(
	'{string} has {string} replaced with {string}',
	async ({}, file: string, from: string, to: string) => {
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

/**
 * The bytes Go sends now, read as text rather than through a browser: a page
 * that hot-reloaded in the browser would show the edit whether or not Go had
 * it.
 *
 * It polls, because the edit has to reach vite's watcher before Go can be told
 * what to drop, and a fixed sleep is either flaky or slow. The claim is still
 * one that fails: a Go that never picked the edit up never satisfies it.
 */
let latest = '';

Then(
	"the document Go sends for {string} has the page's heading {string}",
	async ({ page, shot }, path: string, heading: string) => {
		await expect
			.poll(
				async () => {
					latest = await (await page.request.get(path)).text();
					return latest;
				},
				{ timeout: 30_000, intervals: [250, 250, 500, 500, 1000] }
			)
			.toMatch(tagged('h1', 'title', heading));
		// The frame is of the page as a visitor would now see it, which is the
		// same edit arriving by the other route.
		await page.goto(path);
		await shot('edited');
	}
);

Then("that document no longer has the heading {string}", async ({}, heading: string) => {
	expect(latest, 'no document has been fetched by this scenario').not.toBe('');
	expect(latest).not.toMatch(tagged('h1', 'title', heading));
});

After(async ({ page }) => {
	for (const [path, before] of edited) {
		await writeFile(path, before, 'utf-8');
	}
	if (edited.size === 0) return;
	edited.clear();
	// Wait for Go to be serving the restored source again, so the next scenario
	// does not read this one's edit.
	await expect
		.poll(async () => (await page.request.get('/')).text(), {
			timeout: 30_000,
			intervals: [250, 250, 500, 500, 1000]
		})
		.toMatch(tagged('h1', 'title', 'Home'));
});

/**
 * A precondition, not an assertion: a leftover route from a run that died
 * halfway through would make everything below pass for the wrong reason.
 */
Given('the app has no route {string}', async ({ page }, path: string) => {
	const response = await page.request.get(path);
	expect(
		response.status(),
		`${path} already exists; a previous run left ${join(app, 'src/routes', path)} behind`
	).toBe(404);
});

/**
 * Writes a route the way a developer would: a `+page.svelte` in a new directory
 * under `src/routes`, while both servers are running.
 *
 * The page shows two things. One is a literal this scenario supplied, which
 * nothing in the app contains, so a document carrying it can only be the file
 * just written. The other is the answer to `getSite` in
 * src/routes/site.remote.go — whose generated `.remote.ts` throws — so a
 * document carrying that is Go having answered a remote function called from a
 * route that did not exist when Go started.
 */
When(
	'a page is added at {string} showing {string} and the site name',
	async ({}, path: string, literal: string) => {
		const dir = join(app, 'src/routes', path);
		added.add(dir);
		await mkdir(dir, { recursive: true });
		await writeFile(
			join(dir, '+page.svelte'),
			[
				'<script lang="ts">',
				`\timport { getSite } from '${'../'.repeat(path.split('/').filter(Boolean).length)}site.remote';`,
				'</script>',
				'',
				'<h1 data-testid="title">Added</h1>',
				`<p data-testid="added">${literal}</p>`,
				'<svelte:boundary>',
				'\t<p data-testid="added-site">{(await getSite()).name}</p>',
				'</svelte:boundary>',
				''
			].join('\n'),
			'utf-8'
		);
	}
);

/**
 * The bytes Go sends now, read as text. It polls, because the file has to reach
 * vite's watcher before Go can be told the route tree moved; a Go that never
 * picks it up never satisfies this.
 */
Then(
	'the document Go sends for {string} carries {string}',
	async ({ page, shot }, path: string, text: string) => {
		await expect
			.poll(
				async () => {
					latest = await (await page.request.get(path)).text();
					return latest;
				},
				{ timeout: 30_000, intervals: [250, 250, 500, 500, 1000] }
			)
			.toContain(text);
		await shot();
	}
);

Then('that document also carries {string}', async ({}, text: string) => {
	expect(latest, 'no document has been fetched by this scenario').not.toBe('');
	expect(latest).toContain(text);
});

Then('that document never mentions {string}', async ({}, text: string) => {
	expect(latest, 'no document has been fetched by this scenario').not.toBe('');
	expect(latest).not.toContain(text);
});

After(async ({ page }) => {
	if (added.size === 0) return;
	const paths = [...added].map((dir) => '/' + dir.slice(join(app, 'src/routes').length + 1));
	for (const dir of added) await rm(dir, { recursive: true, force: true });
	added.clear();
	// Wait for Go to have stopped serving them, so the next scenario starts
	// from the route tree the app is checked in with.
	for (const path of paths) {
		await expect
			.poll(async () => (await page.request.get(path)).status(), {
				timeout: 30_000,
				intervals: [250, 250, 500, 500, 1000]
			})
			.toBe(404);
	}
});
