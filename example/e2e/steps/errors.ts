import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';
import type { Response } from '@playwright/test';

const { When, Then } = createBdd(test);

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

Then('the document was answered with {int}', async ({ documents, shot }, status: number) => {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.status()).toBe(status);
	await shot();
});

// The title and the message together, because either alone would pass over a
// page that rendered the wrong error: the status without the message is any
// failure at all, and the message without the status is the client's own render
// of it.
Then(
	'the document already said the error page shows {string} and {string}',
	async ({ documents, shot }, title: string, message: string) => {
		const html = await documentText(documents);
		expect(html).toContain(`<h1 data-testid="title">${title}</h1>`);
		expect(html).toContain(`<p data-testid="error-message">${message}</p>`);
		await shot();
	}
);

// An error page is rendered *inside* the layouts above it. Without this the
// same assertions would pass over kit's static error page, which is a whole
// document of its own and has no app in it.
Then('the document already carried the root layout', async ({ documents, shot }) => {
	expect(await documentText(documents)).toContain('data-testid="app-nav"');
	await shot();
});

// The other end of the same scale: the document kit falls back to when no
// component can be trusted to render. It carries no app markup and no script at
// all, so nothing boots and nothing tries again.
Then(
	"the document is kit's static error page saying {int} and {string}",
	async ({ documents, page, shot }, status: number, message: string) => {
		const html = await documentText(documents);
		expect(html).not.toContain('<script');
		expect(html).not.toContain('data-testid="app-nav"');
		expect(html).toContain(`<span class="status">${status}</span>`);
		expect(html).toContain(`<h1>${message}</h1>`);
		// And it is a page a person can read, not a blank one.
		await expect(page.locator('.status')).toHaveText(String(status));
		await expect(page.getByRole('heading', { name: message })).toBeVisible();
		await shot();
	}
);

// The page that called the command shows a tally when it works. It renders an
// error page instead, and the tally is nowhere — neither in the bytes nor on
// screen.
Then('the tally is nowhere on the page', async ({ documents, page, shot }) => {
	expect(await documentText(documents)).not.toContain('data-testid="tally"');
	await expect(page.getByTestId('tally')).toHaveCount(0);
	await shot();
});

/** The last answer to a request the scenario made itself, without a browser. */
const asked = { status: 0, location: '', body: '' };

// A browser follows a redirect before anything can look at it. This asks the
// way `curl -i` asks, so the scenario can say what the server actually sent.
When(
	'I ask for {string} without following redirects',
	async ({ request }, path: string) => {
		const response = await request.get(path, { maxRedirects: 0 });
		asked.status = response.status();
		asked.location = response.headers()['location'] ?? '';
		asked.body = await response.text();
	}
);

Then(
	'it answered {int} to {string} with no body',
	async ({ shot }, status: number, location: string) => {
		expect(asked.status).toBe(status);
		expect(asked.location).toBe(location);
		// Kit's `redirect_response` sends a Location and nothing else. A body
		// here would be a document skgo rendered for a page the visitor is
		// never going to see.
		expect(asked.body, `the redirect carried ${asked.body.length} bytes of body`).toBe('');
		await shot('redirect');
	}
);
