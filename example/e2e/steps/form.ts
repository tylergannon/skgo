import { fileURLToPath } from 'node:url';
import { createBdd } from 'playwright-bdd';
import type { Locator, Page } from '@playwright/test';
import { expect, test } from './fixtures';

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
		await page.getByTestId('field-from').fill(name);
		await page.getByTestId('field-email').fill(email);
		await page.getByTestId('field-body').fill(body);
	}
);

When('I attach the fixture {string}', async ({ page }, name: string) => {
	await page.getByTestId('field-attachment').setInputFiles(fixture(name));
});

When('I send the message', async ({ page }) => {
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
