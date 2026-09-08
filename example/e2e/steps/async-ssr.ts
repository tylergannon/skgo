import { randomUUID } from 'node:crypto';
import { createBdd } from 'playwright-bdd';
import { expect, test, hydrated } from './fixtures';
const { Given, When, Then } = createBdd(test);

let heldGroup = '';
let heldResponse: Promise<import('@playwright/test').Response | null>;

async function booted(page: import('@playwright/test').Page) {
 await hydrated(page);
 await expect.poll(() => page.evaluate(() => history.scrollRestoration)).toBe('manual');
}

Given('I open a fresh {string} concurrency page', async ({page,documents,remotes}, mode:string) => {
 remotes.mark();
 const response = await page.goto(`/async-ssr/${mode}/${randomUUID()}`);
 expect(response?.status()).toBe(200);
 expect(documents.last).not.toBeNull();
});

Then('the document and hydrated page contain exactly {string}', async ({page,documents}, expected:string) => {
 const values = expected.split(', ');
 const html = await documents.last!.text();
 const list = /<ul data-testid="probe-values"[^>]*>([\s\S]*?)<\/ul>/.exec(html);
 expect(list, 'server-rendered values must exist in the document').not.toBeNull();
 const items = [...list![1].matchAll(/<li[^>]*>([\s\S]*?)<\/li>/g)].map(match => match[1].replace(/<!--[\s\S]*?-->/g,'').trim());
 expect(items).toEqual(values);
 await booted(page);
 await expect(page.getByTestId('probe-values').locator('li')).toHaveText(values);
 await expect(page.getByTestId('probe-failure')).toHaveCount(0);
});

Then('the document and hydrated page show the expected harvest', async ({page,documents}) => {
 const group = new URL(page.url()).pathname.split('/').at(-1)!;
 const expected = `Harvest from seed:${group}`;
 const html = await documents.last!.text();
 expect(html).toContain(expected);
 await booted(page);
 await expect(page.getByTestId('dependent-value')).toHaveText(expected);
});

Then("the live query's browser reconnect settles without failing the page", async ({page}) => {
 await booted(page);
 // The live query seeds its first value from the document, then opens its own
 // connection to the server once hydrated. That reconnect has to outlive the
 // fixture's 2s overlap window before this is a real claim about the settled
 // state rather than the value the document happened to render.
 await page.waitForTimeout(2_500);
 await expect(page.getByTestId('probe-failure')).toHaveCount(0);
 await expect(page.getByTestId('probe-values').locator('li')).toHaveText(['Amber', 'Birch one', 'Birch two', 'Cobalt']);
});

Then('hydration did not refetch the ordinary queries', async ({page,remotes}) => {
 await booted(page);
 await page.waitForTimeout(300);
 expect(remotes.urlsSince).toEqual([]);
});

Given('I cancel a document after its Go operation has started', async ({context,request}) => {
 heldGroup = randomUUID();
 const abandoned = await context.newPage();
 const navigation = abandoned.goto(`/async-ssr/abandoned/${heldGroup}`).catch(() => null);
 await expect.poll(async () => (await (await request.get(`/async-ssr/control/${heldGroup}`)).json()).started).toBe(true);
 await abandoned.close();
 await navigation;
 await expect.poll(async () => (await (await request.get(`/async-ssr/control/${heldGroup}`)).json()).cancelled).toBe(true);
});

Given('I start another document while the abandoned operation is held', async ({page,request,remotes}) => {
 remotes.mark();
 heldResponse = page.goto(`/async-ssr/current/${heldGroup}`);
 await expect.poll(async () => (await (await request.get(`/async-ssr/control/${heldGroup}`)).json()).currentStarted).toBe(true);
 const state = await (await request.get(`/async-ssr/control/${heldGroup}`)).json();
 expect(state.finished).toBe(false);
});

When('the abandoned operation finishes before the new operation', async ({page,request}) => {
 const origin = { headers: { origin: process.env.BASE_URL ?? '' } };
 expect((await page.request.post(`/async-ssr/control/${heldGroup}?phase=abandoned`, origin)).status()).toBe(204);
 await expect.poll(async () => (await (await request.get(`/async-ssr/control/${heldGroup}`)).json()).finished).toBe(true);
 expect((await page.request.post(`/async-ssr/control/${heldGroup}?phase=current`, origin)).status()).toBe(204);
 expect((await heldResponse)?.status()).toBe(200);
});

Then('the new document and hydrated page show only their own result', async ({page,documents}) => {
 const expected = `Current: ${heldGroup}`;
 const html = await documents.last!.text();
 expect(html).toContain(`data-testid="held-value">${expected}</p>`);
 expect(html).not.toContain('Abandoned:');
 await booted(page);
 await expect(page.getByTestId('held-value')).toHaveText(expected);
 await expect(page.getByTestId('probe-failure')).toHaveCount(0);
 await expect(page.locator('body')).not.toContainText('Abandoned:');
});
