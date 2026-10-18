// A Playwright config for an skgo project, kept in web/playwright.config.ts.
// Instead of Vite's preview server, it starts the built binary with
// `just serve` from the project root and tests it on the project's origin.
export default {
  webServer: {
    command: "cd .. && just serve",
    url: "http://127.0.0.1:8080",
    reuseExistingServer: !process.env.CI,
  },
  use: { baseURL: "http://127.0.0.1:8080" },
};
