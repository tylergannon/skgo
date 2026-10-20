import { defineConfig, devices } from "@playwright/test";

// The one browser check. Go tests own the server it runs against: they pass its
// address as BASE_URL and pick an --output directory under test-results/.
export default defineConfig({
  testDir: "tests",
  retries: 0,
  workers: 1,
  // A development session rebuilds Go during the check.
  timeout: 120_000,
  use: {
    ...devices["Desktop Chrome"],
    baseURL: process.env.BASE_URL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
