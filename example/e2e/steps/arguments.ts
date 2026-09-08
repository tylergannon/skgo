import { createBdd } from 'playwright-bdd';
import type { Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { expect, test } from './fixtures';

const { When, Then } = createBdd(test);

/** Kit's remote-function envelope, as the browser receives it. */
type Envelope = {
	type: string;
	data?: string;
	error?: { status: number; message: string };
};

/** The last envelope each scenario's page was handed. */
const answers = new WeakMap<Page, Envelope>();

/**
 * The ids the built frontend calls, read from the list `skgo generate` wrote
 * and the adapter copied into the build.
 *
 * A scenario names the export — `renameTodo` — because that is what a developer
 * wrote; the `<hash>/<name>` the client addresses is kit's hash of the module
 * path and belongs in no feature file.
 */
function remoteId(name: string): string {
	const raw = readFileSync(new URL('../../web/skgo.remotes.json', import.meta.url), 'utf8');
	const { remotes } = JSON.parse(raw) as { remotes: string[] };
	const found = remotes.filter((id) => id.endsWith(`/${name}`));
	if (found.length !== 1) {
		throw new Error(`expected exactly one remote function called ${name}, found ${found.length}`);
	}
	return found[0];
}

/**
 * Makes the call from inside the page, so it is the app's own origin, the app's
 * own cookies and the endpoint kit's client uses. `fetch` rather than
 * Playwright's request context for exactly that reason: a call from outside the
 * browser would be refused by the cross-site check before the argument was ever
 * looked at, and the claim here is about the argument.
 */
When(
	'the page calls {string} with the argument {}',
	async ({ page }, name: string, argument: string) => {
		const id = remoteId(name);
		// The devalue document the feature file wrote, base64url with no
		// padding: kit's own `stringify_command_arg` produces exactly this.
		const payload = Buffer.from(argument, 'utf8').toString('base64url');
		const envelope = await page.evaluate(
			async (call: { url: string; body: string }) => {
				const response = await fetch(call.url, {
					method: 'POST',
					headers: { 'content-type': 'application/json' },
					body: call.body
				});
				return (await response.json()) as unknown;
			},
			{ url: `/_app/remote/${id}`, body: JSON.stringify({ payload, refreshes: [] }) }
		);
		answers.set(page, envelope as Envelope);
	}
);

/**
 * The same call for a query, which kit's client makes with GET and a `payload`
 * query parameter. A query answered over POST is a 405 before its argument is
 * looked at, and the claim here is about the argument.
 */
When(
	'the page asks {string} with the argument {}',
	async ({ page }, name: string, argument: string) => {
		const id = remoteId(name);
		const payload = Buffer.from(argument, 'utf8').toString('base64url');
		const envelope = await page.evaluate(async (url: string) => {
			const response = await fetch(url);
			return (await response.json()) as unknown;
		}, `/_app/remote/${id}?payload=${payload}`);
		answers.set(page, envelope as Envelope);
	}
);

Then(
	'the server refused it with {int} {string}',
	async ({ page }, status: number, message: string) => {
		const envelope = answers.get(page);
		expect(envelope, 'no remote call was made in this scenario').toBeDefined();
		// Kit answers a runtime remote error inside the envelope rather than
		// with an HTTP status, and its client reads the envelope either way.
		expect(envelope!.type, `envelope was ${JSON.stringify(envelope)}`).toBe('error');
		expect(envelope!.error?.status).toBe(status);
		expect(envelope!.error?.message).toBe(message);
	}
);

Then('the server answered it', async ({ page }) => {
	const envelope = answers.get(page);
	expect(envelope, 'no remote call was made in this scenario').toBeDefined();
	expect(envelope!.type, `envelope was ${JSON.stringify(envelope)}`).toBe('result');
	expect(envelope!.data, 'a result envelope with no data').toBeTruthy();
});
