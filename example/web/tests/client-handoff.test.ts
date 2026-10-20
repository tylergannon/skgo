import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { chromium, type Browser, type Page } from "playwright-core";
import { afterAll, beforeAll, expect, onTestFailed, test } from "vite-plus/test";

// A Go test owns the server (HANDOFF_ORIGIN) and the browser: Lightpanda, a
// headless engine with no layout or rendering, reached over CDP (LIGHTPANDA_CDP).
// Vitest keeps BASE_URL for Vite's base path.
const origin = process.env.HANDOFF_ORIGIN!;
// The temp copy of the example a development session serves. Its Go test sets
// it; against a production build there is no source to edit.
const devApp = process.env.SKGO_DEV_APP;

let browser: Browser;
let page: Page;

beforeAll(async () => {
  browser = await chromium.connectOverCDP(process.env.LIGHTPANDA_CDP!);
  page = await (await browser.newContext()).newPage();
});

afterAll(async () => {
  await browser?.close();
});

// Runs before any of the page's scripts. It keeps the nodes the HTML parser
// inserts, notes what the server rendered once parsing ends, and notes when
// Vite's development socket connects.
function observe() {
  const testids = ["app-nav", "title"];
  const parsed: Record<string, Element> = {};
  const state = {
    parsed,
    rendered: null as null | { hydrated: boolean; deployment?: string; todos: string[] },
    socket: false,
  };
  Object.assign(window, { __handoff: state });
  new MutationObserver(() => {
    for (const id of testids) {
      parsed[id] ??= document.querySelector(`[data-testid="${id}"]`) ?? undefined!;
    }
  }).observe(document, { childList: true, subtree: true });
  document.addEventListener("DOMContentLoaded", () => {
    state.rendered = {
      // Kit's router sets manual scroll restoration once hydration finishes.
      hydrated: history.scrollRestoration === "manual",
      deployment: document.querySelector('[data-testid="deployment"]')?.textContent ?? undefined,
      todos: [...document.querySelectorAll('[data-testid="todos"] [data-testid="todo"]')].map(
        (node) => node.textContent ?? "",
      ),
    };
  });
  const PageSocket = window.WebSocket;
  window.WebSocket = class extends PageSocket {
    constructor(...args: ConstructorParameters<typeof WebSocket>) {
      super(...args);
      this.addEventListener("message", (event) => {
        if (String(event.data).includes('"type":"connected"')) state.socket = true;
      });
    }
  };
}

test("Kit's client takes over the page Go rendered", async () => {
  onTestFailed(async () => {
    const results = join("test-results", process.env.HANDOFF_MODE ?? "handoff");
    mkdirSync(results, { recursive: true });
    writeFileSync(join(results, "page.html"), await page.content().catch(String));
  });
  const remote: string[] = [];
  page.on("request", (request) => {
    const { pathname } = new URL(request.url());
    if (pathname.includes("/_app/remote/")) remote.push(`${request.method()} ${pathname}`);
  });
  const todoFetches = () => remote.filter((call) => call.endsWith("/getTodos"));
  const todos = page.getByTestId("todos").getByTestId("todo");
  await page.addInitScript(observe);

  // 1. Go's page, as the parser saw it before Kit's client started.
  await page.goto(new URL("/todos", origin).href);
  const rendered = await page.evaluate(() => (window as any).__handoff.rendered);
  expect(rendered).toMatchObject({ hydrated: false, deployment: "skgo example" });
  expect(rendered.todos).toEqual(
    expect.arrayContaining(["write the adapter", "serve remote functions"]),
  );

  // 2. Hydration adopts the nodes Go rendered.
  await page.waitForFunction(() => history.scrollRestoration === "manual");
  // The live count opens its stream after hydration; that request is expected.
  await expect.poll(() => remote.some((call) => call.endsWith("/watchCount"))).toBe(true);
  const kept = await page.evaluate(() =>
    Object.entries((window as any).__handoff.parsed as Record<string, Element>).map(
      ([id, node]) => [
        id,
        node.isConnected && document.querySelector(`[data-testid="${id}"]`) === node,
      ],
    ),
  );
  expect(Object.fromEntries(kept), "hydration replaced the server's nodes").toEqual({
    "app-nav": true,
    title: true,
  });
  expect(await page.getByTestId("title").textContent()).toBe("Todos");
  expect(await page.getByTestId("deployment").textContent()).toBe("skgo example");
  expect(await todos.allTextContents()).toEqual(rendered.todos);
  // Go embedded getTodos' result in the page. A request for it means Go filed
  // the result under a key the client does not look up.
  expect(todoFetches(), `remote requests: ${remote.join(", ")}`).toEqual([]);

  // 3. A command answered by Go, carrying the refreshed list in one flight.
  // Lightpanda lays nothing out, so the click is dispatched to the button.
  const text = `handoff ${Date.now().toString(36)}`;
  await page.getByTestId("new-todo").fill(text);
  const added = page.waitForResponse((response) =>
    new URL(response.url()).pathname.endsWith("/addTodo"),
  );
  await page.getByTestId("add-todo").dispatchEvent("click");
  expect((await added).status()).toBe(200);
  await todos.filter({ hasText: text }).waitFor();
  expect(todoFetches(), `remote requests: ${remote.join(", ")}`).toEqual([]);

  // 4. Development only: edit Go with the page still open, and wait for
  // skgo's refresh after the rebuild.
  if (devApp) {
    await page.waitForFunction(() => (window as any).__handoff.socket);
    // A reload would start a new window without this mark.
    await page.evaluate(() => Object.assign(window, { __handoffOpen: true }));
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
    await todos.filter({ hasText: "composition-go-edit" }).waitFor({ timeout: 90_000 });
    expect(
      await page.evaluate(() => (window as any).__handoffOpen),
      "the page reloaded instead of refreshing",
    ).toBe(true);
  }
});
