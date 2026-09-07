import { createBdd } from 'playwright-bdd';
import { readFileSync, statSync } from 'node:fs';
import { expect, test } from './fixtures';

const { Given, Then } = createBdd(test);

/**
 * The server's log, as an operator reads it.
 *
 * `just serve` copies the process's output to this file so that one claim in
 * the suite can be checked at all: what the rendering engine writes to
 * `console` goes to the server's log and nowhere else — a browser never sees
 * it. The path is the same one the Justfile hands the suite.
 */
function logPath(): string {
	return process.env.SKGO_LOG || 'server.log';
}

/**
 * How far the log had got before the scenario did anything.
 *
 * The offset matters: the fixture writes the same sentence on every visit, so
 * "the log contains it" would be satisfied by a line an earlier scenario — or
 * an earlier run — left behind. Only what was appended after this point counts.
 */
Given("I note where the server's log has got to", async ({ notes }) => {
	let size: number;
	try {
		size = statSync(logPath()).size;
	} catch (error) {
		throw new Error(
			`skgo e2e: the server's log is not at ${logPath()} (${(error as Error).message}). ` +
				'Start the server with `just serve`, which copies it there.'
		);
	}
	notes.set('log-offset', size);
});

Then('the page says it reported a failure while it rendered', async ({ page, shot }) => {
	await expect(page.getByTestId('console-note')).toHaveText(
		'This page reported a failure to the console while it rendered.',
		{ timeout: 15_000 }
	);
	await shot('rendered');
});

Then("the server's log has since carried {string}", async ({ notes }, text: string) => {
	const offset = notes.get('log-offset');
	expect(offset, "the scenario did not note where the server's log had got to").not.toBeUndefined();

	// Playwright's own retry does not cover a file, and the log is written by
	// another process: the render finished before the response was sent, but
	// the bytes may still be on their way through `tee`.
	await expect
		.poll(() => readFileSync(logPath(), 'utf-8').slice(offset), {
			timeout: 10_000,
			message: `nothing carrying "${text}" was appended to ${logPath()}`
		})
		.toContain(text);
});
