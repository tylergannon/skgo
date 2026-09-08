import { createHash } from 'node:crypto';
import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

/**
 * The header and the boot script have to agree, independently of anything
 * skgo claims about either: this recomputes the hash from the exact bytes the
 * response sent, with Node's own `crypto`, and checks it against the exact
 * bytes the header named. A header carrying *a* sha256 source would pass a
 * weaker check; only the one the script itself earns proves skgo hashed what
 * it actually wrote rather than something else, or something stale.
 */
Then(
	"the response carries a Content-Security-Policy header naming the boot script's own hash",
	async ({ documents, shot }) => {
		// `documents.last` is filled in by a `page.on('response', ...)` listener
		// (fixtures.ts), which fires asynchronously relative to `page.goto`
		// returning — every other assertion in this suite that depends on
		// browser/network timing polls rather than reading state once
		// (`toHaveText(..., { timeout: 15_000 })` throughout), and this is the
		// one place that read `documents.last` synchronously instead.
		await expect
			.poll(() => documents.last?.headers()['content-security-policy'], {
				timeout: 15_000,
				message: 'no Content-Security-Policy header was set'
			})
			.toBeTruthy();
		const header = documents.last!.headers()['content-security-policy'];

		const html = await documents.last!.text();
		const match = /<script>([\s\S]*?)<\/script>/.exec(html);
		expect(match, 'no boot script was found in the document').not.toBeNull();
		const hash = createHash('sha256').update(match![1]).digest('base64');

		expect(header, `header did not name the boot script's own hash (sha256-${hash})`).toContain(
			`'sha256-${hash}'`
		);
		await shot();
	}
);

/**
 * A CSP violation never throws where a step could catch it as a failure —
 * the browser just refuses the element and writes a line to the console
 * instead ("Refused to execute inline script because it violates the
 * following Content Security Policy directive: ..."). This is the only
 * place that would be visible at all, which is why every scenario that
 * exercises a CSP-configured page ends by checking it.
 */
Then('the browser reported no CSP violations', async ({ browserConsole, shot }) => {
	const violations = browserConsole.messages.filter((message) =>
		/content security policy/i.test(message)
	);
	expect(violations, `the browser reported: ${violations.join('; ')}`).toHaveLength(0);
	await shot();
});
