import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';
import { tagged } from './ssr';
import type { Response } from '@playwright/test';

const { Then } = createBdd(test);

/**
 * The bytes the server sent, before a single line of JavaScript ran.
 *
 * An error page is the one place this matters most: the client renders the same
 * `+error.svelte` from `__data.json` a moment later either way, so a DOM
 * assertion cannot tell a rendered error document from a shell that booted and
 * discovered the error for itself.
 */
async function documentText(documents: { last: Response | null }) {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	return await documents.last!.text();
}

// The support id example.HandleError adds to every error it sees, read back
// from the client-rendered page rather than the raw document — the other
// half of the same claim "the document already said" makes: kit hydrates
// from the same page.error the server rendered, so the value the browser
// shows after booting has to be the hook's, not just the bytes Go sent.
Then(
	'the error page shows the support id {string}',
	async ({ page }, id: string) => {
		await expect(page.getByTestId('error-support-id')).toHaveText(id, { timeout: 15_000 });
	}
);

// The page's own heading, in the bytes. It is what separates a page that
// handled its failure and kept rendering from one that was replaced by an error
// page — both of which can carry the same error message.
Then(
	"the document already said the page's own heading is {string}",
	async ({ documents }, heading: string) => {
		expect(await documentText(documents)).toMatch(tagged('h1', 'title', heading));
	}
);
