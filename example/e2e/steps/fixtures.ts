import { expect, type Page, type Response } from '@playwright/test';
import { createBdd, test as base } from 'playwright-bdd';
import { mkdirSync, writeFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';

/** Records the document (top-level navigation) traffic of one scenario. */
export type Documents = {
	/** How many document requests the browser made. */
	count: number;
	/** The most recent document response. */
	last: Response | null;
	/** `<method> <url> -> <status>` for each, in order, to explain a failure. */
	log: string[];
};

/**
 * Records the programmatic remote-function traffic (`/_app/remote/...`) of one
 * scenario: queries, live-query streams and command invocations all go there.
 */
export type Remotes = {
	/** How many remote requests the browser made in the scenario so far. */
	count: number;
	/** Their URLs, in order — used to explain a failed count. */
	urls: string[];
	/** Freeze the current count so `since` can measure a single interaction. */
	mark(): void;
	/** How many remote requests were made since the last `mark()`. */
	readonly since: number;
	/** The URLs of those requests. */
	readonly urlsSince: string[];
};

/**
 * Records the data traffic (`/<route>/__data.json`) of one scenario. It is the
 * only endpoint kit's client calls on its own initiative, once per navigation,
 * so counting it is how a scenario says "and nothing went back for more".
 */
export type Data = {
	/** How many data requests the browser made in the scenario so far. */
	count: number;
	/** Their URLs, in order — used to explain a failed count. */
	urls: string[];
	/**
	 * The most recent data response. It is the other half of a streaming
	 * claim: a client-side navigation gets the page's values on this response
	 * rather than on a document, so the order they arrive in is only visible
	 * here.
	 */
	last: Response | null;
	/** Freeze the current count so `since` can measure a single interaction. */
	mark(): void;
	/** How many data requests were made since the last `mark()`. */
	readonly since: number;
	/** The URLs of those requests. */
	readonly urlsSince: string[];
};

/** Scratch values a scenario carries from one step to the next. */
export type Notes = Map<string, number>;

/**
 * The browser's own console, for the one claim only it can make: that a CSP
 * the app configured did not block anything. A blocked inline script does not
 * throw where a `Then` step could catch it — the browser silently drops the
 * element and reports the refusal to the console instead, so this is the only
 * place a false CSP header (or a real one the boot script's own hash does not
 * satisfy) would be visible at all.
 */
export type BrowserConsole = {
	/** Every message the page's console produced, in order. */
	messages: string[];
};

/**
 * Screenshots the page in whatever state the scenario left it, named after the
 * scenario. Every loads scenario leaves one behind, because the sprint's
 * acceptance is somebody looking at all of them.
 */
export type Shot = (name?: string) => Promise<void>;

type BrowserFrame = {
	capture(file: string): Promise<void>;
};

export const test = base.extend<{
	documents: Documents;
	remotes: Remotes;
	data: Data;
	notes: Notes;
	shot: Shot;
	browserConsole: BrowserConsole;
	browserFrame: BrowserFrame;
}>({
	browserFrame: [
		async ({ page }, use) => {
			const session = await page.context().newCDPSession(page);
			await use({
				async capture(file: string) {
					// Direct CDP capture avoids Playwright's navigation wait (which
					// cannot finish while a streaming document is still open), and
					// asks for the current frame instead of reusing a stale screencast.
					const { data } = await session.send('Page.captureScreenshot', { format: 'png' });
					writeFileSync(file, Buffer.from(data, 'base64'));
				}
			});
			await session.detach().catch(() => undefined);
		},
		{ auto: true }
	],
	// `auto` so the listener is attached before the scenario's first
	// navigation — a violation reported while the very first document loads
	// would otherwise be missed.
	browserConsole: [
		async ({ page }, use) => {
			const browserConsole: BrowserConsole = { messages: [] };
			page.on('console', (msg) => browserConsole.messages.push(msg.text()));
			await use(browserConsole);
		},
		{ auto: true }
	],

	documents: async ({ page }, use) => {
		const documents: Documents = { count: 0, last: null, log: [] };

		page.on('request', (request) => {
			if (request.resourceType() === 'document') documents.count++;
		});
		page.on('response', (response) => {
			const request = response.request();
			if (request.resourceType() !== 'document') return;
			documents.last = response;
			documents.log.push(`${request.method()} ${request.url()} -> ${response.status()}`);
		});

		await use(documents);
	},

	// `auto` so the listener is attached before the scenario's first navigation,
	// however late the first step that reads it runs.
	remotes: [
		async ({ page }, use) => {
			let marked = 0;
			const remotes: Remotes = {
				count: 0,
				urls: [],
				mark() {
					marked = remotes.count;
				},
				get since() {
					return remotes.count - marked;
				},
				get urlsSince() {
					return remotes.urls.slice(marked);
				}
			};

			page.on('request', (request) => {
				if (!request.url().includes('/_app/remote/')) return;
				remotes.count++;
				remotes.urls.push(`${request.method()} ${new URL(request.url()).pathname}`);
			});

			await use(remotes);
		},
		{ auto: true }
	],

	// `auto` so the listener is attached before the scenario's first navigation.
	data: [
		async ({ page }, use) => {
			let marked = 0;
			const data: Data = {
				count: 0,
				urls: [],
				last: null,
				mark() {
					marked = data.count;
				},
				get since() {
					return data.count - marked;
				},
				get urlsSince() {
					return data.urls.slice(marked);
				}
			};

			page.on('request', (request) => {
				if (!request.url().includes('/__data.json')) return;
				data.count++;
				data.urls.push(`${request.method()} ${new URL(request.url()).pathname}`);
			});
			page.on('response', (response) => {
				if (!response.url().includes('/__data.json')) return;
				data.last = response;
			});

			await use(data);
		},
		{ auto: true }
	],

	// eslint-disable-next-line no-empty-pattern -- Playwright infers fixture deps from this pattern.
	notes: async ({}, use) => {
		await use(new Map<string, number>());
	},

	shot: async ({ browserFrame }, use, testInfo) => {
		// The title of a Scenario Outline's example is just "Example #1", so the
		// scenario's own title has to come along or three redirects overwrite
		// each other.
		const slug = testInfo.titlePath
			.slice(-2)
			.join('-')
			.replace(/[^a-z0-9]+/gi, '-')
			.replace(/^-|-$/g, '')
			.toLowerCase();
		// The mode is part of the path for the same reason it is part of the
		// AfterStep frames': the same scenarios run twice, against the embedded
		// build and against `vp dev`, and a shared path means the second run
		// silently overwrites the first — the tracked evidence then shows one
		// mode while the claim is about two.
		const mode = process.env.SKGO_E2E_RUN ?? 'run';
		// And the feature, so the tracked evidence is not one heap named after
		// whichever feature happened to need screenshots first.
		const feature =
			basename(testInfo.file).replace(/\.feature\.spec\.[jt]s$/, '') || 'unknown';
		let taken = 0;
		const shot: Shot = async (name) => {
			const suffix = name ? `-${name}` : taken > 0 ? `-${taken}` : '';
			taken += 1;
			if (!captureScenarioFrame(feature, mode, name)) return;
			const file = `../../ephemeral/screenshots/${feature}/${mode}/${slug}${suffix}.png`;
			mkdirSync(dirname(file), { recursive: true });
			await browserFrame.capture(file);
		};
		await use(shot);
		// A scenario that took no shot of its own still leaves the state it
		// finished in.
		if (taken === 0) await shot('final');
	}
});

/** The mode named by the test invocation, independent of the server under test. */
export function expectedMode(): 'dev' | 'prod' {
	const mode = process.env.SKGO_EXPECTED_MODE;
	expect(['dev', 'prod'], `SKGO_EXPECTED_MODE was ${JSON.stringify(mode)}`).toContain(mode);
	return mode as 'dev' | 'prod';
}

/** Refuses a server other than the one this test leg intended to exercise. */
export function expectMode(response: { headers(): Record<string, string> }): 'dev' | 'prod' {
	const expected = expectedMode();
	const observed = response.headers()['x-skgo-mode'];
	expect(observed, `expected ${expected} mode, X-Skgo-Mode was ${JSON.stringify(observed)}`).toBe(
		expected
	);
	return expected;
}

const { AfterStep } = createBdd(test);

/**
 * Photographs the page the instant a scenario asserts.
 *
 * An exit code says a scenario passed; it does not say what the visitor was
 * looking at when it did. Every step Gherkin classifies as an outcome — Then,
 * and the And/But that continue it — leaves a frame of the real page in the
 * real state the sentence claims, so the run can be checked by looking rather
 * than by rerunning it. The same frames are taken when a step fails, which is
 * the moment they are worth most.
 *
 * Kept out of the step definitions on purpose: a screenshot nobody has to
 * remember to write cannot be forgotten from the next scenario somebody adds.
 */
AfterStep(async ({ browserFrame, $step, $bddContext, $testInfo }) => {
	if (screenshotPolicy() !== 'all') return;
	const step = $bddContext.bddTestData?.steps?.[$bddContext.stepIndex];
	if (step?.keywordType !== 'Outcome') return;

	const file = join(
		screenshotDir(),
		slug($bddContext.featureUri.replace(/^features\//, '').replace(/\.feature$/, '')),
		slug($testInfo.title),
		`${String($bddContext.stepIndex + 1).padStart(2, '0')}-${slug(step.textWithKeyword ?? $step.title)}.png`
	);
	mkdirSync(dirname(file), { recursive: true });
	await shoot(browserFrame, file);
	await $testInfo.attach(step.textWithKeyword ?? $step.title, { path: file, contentType: 'image/png' });
});

type ScreenshotPolicy = 'all' | 'curated' | 'none';

function screenshotPolicy(): ScreenshotPolicy {
	const policy = process.env.SKGO_E2E_SCREENSHOTS ?? 'all';
	if (policy === 'all' || policy === 'curated' || policy === 'none') return policy;
	throw new Error(`SKGO_E2E_SCREENSHOTS must be all, curated, or none; got ${JSON.stringify(policy)}`);
}

export function capturesAllScreenshots(): boolean {
	return screenshotPolicy() === 'all';
}

/**
 * Release qualification keeps one production control and one development HMR
 * result. The generated-project contract contributes the other eight frames.
 */
function captureScenarioFrame(feature: string, mode: string, name?: string): boolean {
	switch (screenshotPolicy()) {
		case 'all':
			return true;
		case 'none':
			return false;
		case 'curated':
			return (
				feature === 'zz-source-update' &&
				((mode === 'prod' && name === 'built-unchanged') ||
					(mode === 'dev' && name === 'hot-updated'))
			);
	}
}

/**
 * Where this run's frames go. The mode is part of the path because the same
 * scenarios run twice — against the embedded build and against `vp dev` — and
 * the two sets are only useful side by side.
 */
function screenshotDir(): string {
	return join('screenshots', process.env.SKGO_E2E_RUN ?? 'run');
}

/**
 * Takes the picture. A page mid-navigation (a step that just swapped the
 * document) can briefly refuse a screenshot, so a couple of retries cover
 * that ordinary timing case. A capture that still fails after retrying is not
 * swallowed: a missing screenshot is a missing screenshot, and the step it
 * belongs to must fail rather than report a pass nobody can check.
 */
async function shoot(browserFrame: BrowserFrame, file: string): Promise<void> {
	for (let attempt = 1; ; attempt++) {
		try {
			await browserFrame.capture(file);
			return;
		} catch (error) {
			if (attempt >= 3) throw error;
			await new Promise((resolve) => setTimeout(resolve, 150));
		}
	}
}

function slug(text: string): string {
	return text
		.replace(/[^\w\s.-]/g, '')
		.trim()
		.replace(/\s+/g, '-')
		.slice(0, 80);
}

/**
 * Waits until kit's client has hydrated the document and started its router.
 *
 * It exists because Go now renders the document in dev too, and a rendered
 * document is on screen — and clickable — before the browser has finished
 * loading the several hundred unbundled modules `vp dev` serves. A step that
 * typed into an input and clicked a button in that window lost both: Svelte's
 * `bind:value` writes the component's own empty state over what was typed the
 * moment it hydrates, and a click before that reaches a form with no handler on
 * it. In dev the window is seconds wide; in prod the bundle closes it, which is
 * why nobody had seen it.
 *
 * `history.scrollRestoration` is kit's own marker rather than one this suite
 * invented: `_start_router` sets it to "manual" as its first statement
 * (packages/kit/src/runtime/client/client.js), and `_start_router` is what runs
 * after `_hydrate` resolves. It is still a claim that can fail — a page that
 * never boots never sets it.
 */
export async function hydrated(page: Page): Promise<void> {
	// Two pages never hydrate and must not be waited on: one whose branch turns
	// CSR off, which carries no boot script at all, and any page in the noscript
	// project, whose context runs none of the script it was sent. Both are
	// visible as the absence of the `__sveltekit_*` object the boot script
	// assigns before it imports anything — the very first thing that runs.
	try {
		await page.waitForFunction(
			() => Object.keys(globalThis).some((key) => key.startsWith('__sveltekit_')),
			undefined,
			{ timeout: 2_000 }
		);
	} catch {
		return;
	}
	await page.waitForFunction(() => history.scrollRestoration === 'manual', undefined, {
		timeout: 30_000
	});
}

export { expect };
