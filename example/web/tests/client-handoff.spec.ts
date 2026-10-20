import { readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { expect, test } from "@playwright/test";

// The temp copy of the example a development session serves. Its Go test sets
// it; against a production build there is no source to edit.
const devApp = process.env.SKGO_DEV_APP;

test("Kit's client takes over the page Go rendered", async ({ page }) => {
  const remote: string[] = [];
  let documents = 0;
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (pathname.includes("/_app/remote/")) {
      remote.push(`${request.method()} ${pathname}`);
    }
    if (request.isNavigationRequest() && request.frame() === page.mainFrame()) {
      documents++;
    }
  });
  const todoFetches = () => remote.filter((call) => call.endsWith("/getTodos"));

  // In development, skgo announces a published Go rebuild on Vite's socket.
  const hmrConnected = new Promise<void>((resolve) => {
    page.on("websocket", (socket) =>
      socket.on("framereceived", ({ payload }) => {
        if (String(payload).includes('"type":"connected"')) resolve();
      }),
    );
  });

  // 1. Kit's boot script is inline and imports the client's entry. Holding
  // every script keeps the client from starting, whatever the entry is called
  // in development or production.
  let release!: () => void;
  const released = new Promise<void>((resolve) => (release = resolve));
  await page.route("**/*", async (route) => {
    if (route.request().resourceType() === "script") await released;
    await route.continue();
  });
  await page.goto("/todos", { waitUntil: "commit" });
  // The boot script assigns Kit's global before it imports anything.
  await page.waitForFunction(() =>
    Object.keys(globalThis).some((key) => key.startsWith("__sveltekit_")),
  );
  const todos = page.getByTestId("todos").getByTestId("todo");
  await expect(page.getByTestId("deployment")).toHaveText("skgo example");
  await expect(todos).toContainText(["write the adapter", "serve remote functions"]);
  const rendered = await todos.allTextContents();
  const nav = await page.getByTestId("app-nav").elementHandle();
  const title = await page.getByTestId("title").elementHandle();

  // 2. Release the client. Hydration adopts the nodes Go rendered.
  const watching = page.waitForRequest((request) =>
    new URL(request.url()).pathname.endsWith("/watchCount"),
  );
  release();
  await page.unrouteAll({ behavior: "wait" });
  // Kit starts its router, which sets manual scroll restoration, once
  // hydration has finished.
  await page.waitForFunction(() => history.scrollRestoration === "manual");
  // The live count opens its stream after hydration; that request is expected.
  await watching;
  for (const [testid, node] of [
    ["app-nav", nav],
    ["title", title],
  ] as const) {
    const kept = await node.evaluate(
      (held, testid) =>
        held.isConnected && document.querySelector(`[data-testid="${testid}"]`) === held,
      testid,
    );
    expect(kept, `hydration replaced the server's ${testid} node`).toBe(true);
  }
  await expect(page.getByTestId("title")).toHaveText("Todos");
  await expect(page.getByTestId("deployment")).toHaveText("skgo example");
  await expect(todos).toHaveText(rendered);
  // Go embedded getTodos' result in the page. A request for it means Go filed
  // the result under a key the client does not look up.
  expect(todoFetches(), `remote requests: ${remote.join(", ")}`).toEqual([]);

  // 3. A command answered by Go, carrying the refreshed list in one flight.
  const text = `handoff ${Date.now().toString(36)}`;
  await page.getByTestId("new-todo").fill(text);
  const added = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith("/addTodo"),
  );
  await page.getByTestId("add-todo").click();
  expect((await added).status()).toBe(200);
  await expect(todos.filter({ hasText: text })).toHaveCount(1);
  // The page made any hydration refetch before this command, so it has been
  // seen by now.
  expect(todoFetches(), `remote requests: ${remote.join(", ")}`).toEqual([]);

  // 4. Development only: edit Go with the page still open, and wait for
  // skgo's refresh after the rebuild.
  if (devApp) {
    await hmrConnected;
    const source = join(devApp, "web/src/routes/todos/todos.remote.go");
    const original = readFileSync(source, "utf8");
    const body = "return visitor.Store(ctx).Todos(signedIn(ctx)), nil";
    expect(original).toContain(body);
    writeFileSync(
      source,
      original.replace(
        body,
        'return append(visitor.Store(ctx).Todos(signedIn(ctx)), businesslogic.Todo{ID: "composition-edit", Text: "composition-go-edit"}), nil',
      ),
    );
    await expect(todos.filter({ hasText: "composition-go-edit" })).toHaveCount(1, {
      timeout: 90_000,
    });
    expect(documents, "the page reloaded instead of refreshing").toBe(1);
  }
});
