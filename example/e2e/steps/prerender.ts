import { createBdd } from "playwright-bdd";
import { booted, expect, test } from "./fixtures";

const { Then } = createBdd(test);

Then(
  "the prerender consumer shows the built receipt without fetching it",
  async ({ page, documents, remotes, $testInfo }) => {
    const response = documents.last;
    expect(response, "no document response was observed").not.toBeNull();
    const html = await response!.text();
    expect(html).toContain(
      '<p data-testid="prerender-consumer-receipt">Go prerender remote: atlas</p>',
    );
    await expect(page.getByTestId("prerender-consumer-receipt")).toHaveText(
      "Go prerender remote: atlas",
    );
    if ($testInfo.project.use.javaScriptEnabled !== false) await booted(page);
    await page.waitForTimeout(500);
    expect(remotes.urls.filter((url) => url.includes("/buildReceipt"))).toEqual([]);
  },
);

Then("the entry receipt is {string}", async ({ page }, receipt: string) => {
  await expect(page.getByTestId("entry-receipt")).toHaveText(receipt);
});

Then("the prerendered Go values are visible", async ({ page, remotes }) => {
  await booted(page);
  await expect(page.getByTestId("prerender-parent")).toHaveText("skgo example");
  await expect(page.getByTestId("prerender-price")).toHaveText("$7.50");
  await expect(page.getByTestId("prerender-deferred")).toHaveText("GO_PRERENDER_DEFERRED");
  await expect(page.getByTestId("prerender-remote")).toHaveText("Go prerender remote: atlas");
  expect(remotes.urls.filter((url) => url.includes("/buildReceipt"))).toEqual([]);
});
