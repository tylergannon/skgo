import { fileURLToPath } from 'node:url';
import { createBdd } from 'playwright-bdd';
import type { Locator, Page } from '@playwright/test';
import { expect, hydrated, test } from './fixtures';

const { When, Then } = createBdd(test);

const fixture = (name: string) =>
	fileURLToPath(new URL(`../fixtures/${name}`, import.meta.url));

/**
 * The messages in the inbox whose body is exactly `body`. A scenario always
 * looks its own message up by what it sent, never by position, so it cannot
 * pass by reading back one an earlier scenario left behind.
 *
 * The count is deliberately not asserted. The inbox is the server's, and it
 * outlives a scenario — running the suite twice against one process leaves two
 * copies of every message — while nothing here claims a submission is
 * deduplicated. What each scenario claims is that its message is there, from
 * the right sender, with the right attachment.
 */
function messageSaying(page: Page, body: string): Locator {
	return page.getByTestId('message').filter({ has: page.getByTestId('message-body').getByText(body, { exact: true }) });
}

When('the contact page has loaded', async ({ page }) => {
	// The inbox query has to have resolved before anything is counted, and a
	// boundary that fell over must not be mistaken for an empty page. The list
	// is checked for attachment rather than visibility: an inbox with nothing
	// in it yet is a zero-height <ul>, which Playwright calls hidden.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('messages')).toBeAttached();
	await expect(page.getByTestId('contact-form')).toBeVisible();
});

When(
	'I fill the contact form with name {string}, email {string} and message {string}',
	async ({ page }, name: string, email: string, body: string) => {
		await hydrated(page);
		await page.getByTestId('field-from').fill(name);
		await page.getByTestId('field-email').fill(email);
		await page.getByTestId('field-body').fill(body);
	}
);

When('I attach the fixture {string}', async ({ page }, name: string) => {
	await hydrated(page);
	await page.getByTestId('field-attachment').setInputFiles(fixture(name));
});

When('I send the message', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('send').click();
});

Then('the receipt greets {string}', async ({ page }, name: string) => {
	await expect(page.getByTestId('receipt')).toHaveText(new RegExp(`Thanks, ${name} — message m\\d+ is in\\.`));
});

Then(
	'the inbox shows a message from {string} saying {string}',
	async ({ page }, name: string, body: string) => {
		const message = messageSaying(page, body).first();
		await expect(message).toBeVisible();
		await expect(message.getByTestId('message-from')).toHaveText(name);
		// The refreshed inbox and the enhance callback settle independently.
		// When this assertion follows a successful enhanced submission, wait for
		// Kit to finish the callback's reset before a later step types again.
		// After a rejected submission `rejected` is present and preserving the
		// fields is the behavior under test, so that path deliberately skips this.
		if (
			(await page.getByTestId('contact-form').count()) > 0 &&
			(await page.getByTestId('rejected').count()) === 0
		) {
			await expect(page.getByTestId('receipt')).toBeVisible();
			await expect(page.getByTestId('field-from')).toHaveValue('');
			await expect(page.getByTestId('field-email')).toHaveValue('');
			await expect(page.getByTestId('field-body')).toHaveValue('');
		}
	}
);

Then(
	'the message saying {string} reports the attachment {string} of {int} bytes with digest {string}',
	async ({ page }, body: string, name: string, bytes: number, digest: string) => {
		// The three values are written into the scenario, computed from the
		// fixture on disk with `shasum`, and never read off the page and
		// compared with itself. The digest is what makes this load-bearing: a
		// truncated or reordered upload has the right name and can be made to
		// have the right length, but not the right SHA-256.
		const message = messageSaying(page, body).first();
		await expect(message).toBeVisible();
		await expect(message.getByTestId('message-attachment')).toHaveText(name);
		await expect(message.getByTestId('message-attachment-bytes')).toHaveText(String(bytes));
		await expect(message.getByTestId('message-attachment-digest')).toHaveText(digest);
	}
);

Then('the form reports it was not sent', async ({ page }) => {
	await expect(page.getByTestId('rejected')).toBeVisible();
	await expect(page.getByTestId('receipt')).toHaveCount(0);
});

Then(
	'the field {string} carries the message {string}',
	async ({ page }, field: string, message: string) => {
		await expect(page.getByTestId(`issue-${field}`)).toHaveText(message);
	}
);

Then('the field {string} carries no message', async ({ page }, field: string) => {
	// Only meaningful because the sibling steps have already proved that the
	// issue elements render at all — an empty page would satisfy this alone.
	await expect(page.getByTestId(`issue-${field}`)).toHaveCount(0);
});

Then(
	'the contact form still holds name {string}, email {string} and message {string}',
	async ({ page }, name: string, email: string, body: string) => {
		// Kit does not reset an enhanced form, and a rejected submission never
		// navigates, so the visitor's input is still in the controls. The
		// values compared against are the literals the scenario typed.
		await expect(page.getByTestId('field-from')).toHaveValue(name);
		await expect(page.getByTestId('field-email')).toHaveValue(email);
		await expect(page.getByTestId('field-body')).toHaveValue(body);
	}
);

Then('the inbox has no message saying {string}', async ({ page }, body: string) => {
	// An absence is only evidence when the list that would have shown it is
	// demonstrably rendering. The scenario puts a message in the inbox and
	// checks it is still on screen before it asks about this one, so a page
	// that rendered nothing at all cannot satisfy the pair.
	await expect(page.locator('[data-testid$="-failed"]')).toHaveCount(0);
	await expect(page.getByTestId('message')).not.toHaveCount(0);
	await expect(messageSaying(page, body)).toHaveCount(0);
});

When('the contact form is on the page', async ({ page }) => {
	// The noscript counterpart of "the contact page has loaded". It cannot wait
	// for the inbox: with scripting off the boundary around it never leaves its
	// pending snippet, because Svelte's server renderer does not render the
	// children of a boundary that has one.
	await expect(page.getByTestId('title')).toHaveText('Contact');
	await expect(page.getByTestId('contact-form')).toBeVisible();
});

When('the browser submits the form itself, without kit\'s client', async ({ page }) => {
	// `HTMLFormElement.submit()` does not fire a submit event, so kit's
	// `enhance` never sees it and the browser performs the form's own POST —
	// the same request a visitor with scripting off makes, in a browser that
	// will then hydrate the answer.
	//
	// The wait has to be armed first: `form.submit()` returns before the
	// browser has even issued the request, and `waitForLoadState('load')` on a
	// page that is still the old one resolves immediately — which is how this
	// step first passed while asserting nothing had happened.
	const navigated = page.waitForResponse(
		(response) =>
			response.request().resourceType() === 'document' &&
			response.request().method() === 'POST'
	);
	await page.getByTestId('contact-form').evaluate((form: HTMLFormElement) => form.submit());
	await navigated;
	await page.waitForLoadState('load');
});

Then('a new document answered the submission', async ({ documents }) => {
	// The visitor is looking at the response to their own POST — not at the
	// page they were on with a patch applied to it, which is what the enhanced
	// path produces and what this whole feature exists to be distinct from.
	expect(documents.last, 'no document response was observed').not.toBeNull();
	expect(documents.last!.request().method(), documents.log.join('\n')).toBe('POST');
	expect(documents.last!.status()).toBe(200);
});

Then('the browser ran no script', async ({ page }) => {
	// Positive evidence rather than an absence: the inbox sits inside a
	// boundary whose pending snippet is what the server renders, and the only
	// thing that ever replaces it is kit's client running the query. Still
	// seeing "loading…" is that client not existing.
	await expect(page.getByTestId('messages-pending')).toBeVisible();
});

Then('the form does not report it was not sent', async ({ page }) => {
	await expect(page.getByTestId('rejected')).toHaveCount(0);
});

Then(
	'the contact form still holds name {string} and email {string}',
	async ({ page }, name: string, email: string) => {
		// The two text inputs only. A submission that navigated leaves the
		// message behind: kit's field proxy puts the value in a `value`
		// attribute, which a <textarea> ignores — its value is its content —
		// and with no client to assign the property there is nothing to put it
		// back. That is kit's shape, and claiming otherwise here would be a
		// scenario that fails for a reason skgo cannot fix.
		await expect(page.getByTestId('field-from')).toHaveValue(name);
		await expect(page.getByTestId('field-email')).toHaveValue(email);
	}
);

// The keyed instance's own steps below. They exist to prove two things at
// once: that `sendMessage.for("k1")`'s key reaches the Go handler exactly the
// way kit's own client encodes it, and that its result/issues are its own —
// never the bare `sendMessage` form's — because a keyed instance is cached
// separately (`runtime/app/server/remote/form.js`, `state.remote.forms`).

When('the keyed contact form is on the page', async ({ page }) => {
	await expect(page.getByTestId('keyed-contact-form')).toBeVisible();
});

When(
	'I fill the keyed contact form with name {string}, email {string} and message {string}',
	async ({ page }, name: string, email: string, body: string) => {
		await hydrated(page);
		await page.getByTestId('keyed-field-from').fill(name);
		await page.getByTestId('keyed-field-email').fill(email);
		await page.getByTestId('keyed-field-body').fill(body);
	}
);

When('I send the keyed message', async ({ page }) => {
	await hydrated(page);
	await page.getByTestId('keyed-send').click();
});

Then('the keyed receipt greets {string}', async ({ page }, name: string) => {
	await expect(page.getByTestId('keyed-receipt')).toHaveText(new RegExp(`Thanks, ${name} — message m\\d+ is in\\.`));
});

Then('the keyed receipt carries the key {string}', async ({ page }, key: string) => {
	// The key the page shows is `sendMessage.result.key` (contact.remote.go's
	// `Receipt.Key`, set from `Draft.ForKey`) — Go's own record of what
	// arrived on the `id` field of the argument, not a value this step
	// invented or read off the URL itself.
	await expect(page.getByTestId('keyed-receipt-key')).toHaveText(`carried key: ${key}`);
});

Then('the keyed form reports it was not sent', async ({ page }) => {
	await expect(page.getByTestId('keyed-rejected')).toBeVisible();
	await expect(page.getByTestId('keyed-receipt')).toHaveCount(0);
});

Then('the keyed form does not report it was not sent', async ({ page }) => {
	await expect(page.getByTestId('keyed-rejected')).toHaveCount(0);
});

Then(
	'the keyed field {string} carries the message {string}',
	async ({ page }, field: string, message: string) => {
		await expect(page.getByTestId(`keyed-issue-${field}`)).toHaveText(message);
	}
);

Then(
	'the keyed contact form still holds name {string} and email {string}',
	async ({ page }, name: string, email: string) => {
		await expect(page.getByTestId('keyed-field-from')).toHaveValue(name);
		await expect(page.getByTestId('keyed-field-email')).toHaveValue(email);
	}
);

Then('the bare contact form reports nothing sent and nothing refused', async ({ page }) => {
	// The instance this scenario never touched. If the keyed submission's
	// outcome leaked onto it — the two instances sharing a cache slot rather
	// than each keeping its own — one of these would now show something.
	await expect(page.getByTestId('receipt')).toHaveCount(0);
	await expect(page.getByTestId('rejected')).toHaveCount(0);
});
