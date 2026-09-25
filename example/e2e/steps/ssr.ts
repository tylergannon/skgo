import { createBdd } from 'playwright-bdd';
import { booted, expect, test } from './fixtures';

const { Then } = createBdd(test);

/**
 * The bytes the server sent, before a single line of JavaScript ran.
 *
 * A page's markup is only proof of server-side rendering if it was in the
 * response: the same content appears in the DOM a moment later either way, so
 * asserting on the DOM alone cannot tell a rendered page from a hydrated one.
 */
async function documentText(documents: { last: import('@playwright/test').Response | null }) {
	expect(documents.last, 'no document response was observed').not.toBeNull();
	return await documents.last!.text();
}

/**
 * One tagged element, as a matcher that survives the attributes a compiler adds
 * around the claim.
 *
 * Svelte's dev compile keeps the scoping class on elements the build's compile
 * prunes it from, so the same component is `<p data-testid="site-name">skgo</p>`
 * in a built document and `<p data-testid="site-name" class="svelte-1uha8ag">skgo</p>`
 * in a served one. Both say the thing the scenario means. What is still exact is
 * everything the scenario is actually about: the element, its test id, and the
 * whole of its text.
 */
export function tagged(name: string, testid: string, text: string): RegExp {
	const escape = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
	return new RegExp(
		`<${name} data-testid="${escape(testid)}"[^>]*>${escape(text)}</${name}>`
	);
}

Then('the document already said {string}', async ({ documents }, text: string) => {
	expect(await documentText(documents)).toContain(text);
});

Then(
	'the document already said the site is named {string}',
	async ({ documents }, name: string) => {
		expect(await documentText(documents)).toMatch(tagged('p', 'site-name', name));
	}
);

// A boundary with a `pending` snippet renders the snippet instead of its
// children while the document is built, so the query behind it is never called
// there. This says the loading state was in the bytes — not merely that the
// plans were absent, which a page that rendered nothing at all would satisfy.
Then('the document carried the plans as still loading', async ({ documents }) => {
	const html = await documentText(documents);
	expect(html).toContain('data-testid="plans-pending"');
	expect(html).not.toContain('data-testid="plan"');
});

// The other side of the same coin. `ssr = false` means the document is kit's
// shell: none of the page is in it, and all of it is on screen once the client
// has booted — which the step after this one asserts, so this is not a claim
// about a page that simply never rendered.
Then('the document carried no rendered page', async ({ documents, page }) => {
	const html = await documentText(documents);
	expect(html).not.toContain('data-testid="title"');
	expect(html).not.toContain('data-testid="site-name"');
	expect(html, 'the shell still has to boot kit').toContain('kit.start(app, element');
	// The claim is about bytes that have already been read, so the frame left
	// behind can wait for the shell to have become a page. Without this it is a
	// photograph of an empty document — true, and no use to anyone looking at
	// it.
	await expect(page.getByTestId('app-nav')).toBeVisible({ timeout: 15_000 });
});

Then(
	'exactly {int} remote requests were made since',
	async ({ page, remotes }, expected: number) => {
		// A count of zero is also what a page whose client never started would
		// show, so the client has to be running before the count means anything.
		await booted(page);
		// A refetch would already be in flight; wait a beat so it lands and can
		// be counted.
		await page.waitForTimeout(500);
		expect(remotes.since, `remote requests: ${remotes.urlsSince.join(', ') || 'none'}`).toBe(
			expected
		);
	}
);

