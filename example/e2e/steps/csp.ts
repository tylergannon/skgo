import { createHash } from 'node:crypto';
import { createBdd } from 'playwright-bdd';
import { expect, test } from './fixtures';

const { Then } = createBdd(test);

/**
 * The header and the boot script have to agree, independently of anything
 * skgo claims about either: this reads the nonce out of the exact bytes the
 * response sent — the boot script's own `<script nonce="...">` attribute —
 * and checks it against the exact bytes the header named. A header carrying
 * *a* nonce would pass a weaker check; only the one the script itself carries
 * proves skgo handed the engine, the header and the streamed chunks all the
 * same value.
 */
Then(
	"the response carries a Content-Security-Policy header naming the boot script's own nonce",
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
		const match = /<script nonce="([^"]+)">/.exec(html);
		expect(match, 'no nonced boot script was found in the document').not.toBeNull();
		const nonce = match![1];

		expect(header, `header did not name the boot script's own nonce ('nonce-${nonce}')`).toContain(
			`'nonce-${nonce}'`
		);
		await shot();
	}
);

/**
 * The other half of kit's own `auto` ternary: a page kit prerendered carries
 * no Content-Security-Policy response header at all — kit's own
 * `render.js` only sets one on the branch that is not prerendering — and the
 * policy lives in the document itself instead, as a `<meta http-equiv>` tag
 * kit's build baked in. This recomputes the hash from the exact bytes the
 * boot script carries and checks it against the exact bytes the meta tag
 * named, the same independence the header/nonce check above has.
 */
Then(
	"the page carries a Content-Security-Policy meta tag naming the boot script's own hash",
	async ({ documents, shot }) => {
		await expect
			.poll(async () => (await documents.last?.text())?.includes('http-equiv="content-security-policy"'), {
				timeout: 15_000,
				message: 'no Content-Security-Policy meta tag was found in the document'
			})
			.toBe(true);
		const html = await documents.last!.text();

		const metaMatch = /<meta http-equiv="content-security-policy" content="([^"]*)">/.exec(html);
		expect(metaMatch, 'no Content-Security-Policy meta tag was found in the document').not.toBeNull();
		const content = metaMatch![1];

		const scriptMatch = /<script>([\s\S]*?)<\/script>/.exec(html);
		expect(scriptMatch, 'no boot script was found in the document').not.toBeNull();
		const hash = createHash('sha256').update(scriptMatch![1]).digest('base64');

		expect(content, `meta tag did not name the boot script's own hash (sha256-${hash})`).toContain(
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
