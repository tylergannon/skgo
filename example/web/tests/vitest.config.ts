import { defineConfig } from "vite-plus";

// The client handoff check runs in Node and drives Lightpanda; it needs none of
// the app's Vite plugins. Go tests run it with `vp test run --config` and own
// the server and browser it reaches.
export default defineConfig({
  test: {
    include: ["tests/*.test.ts"],
    // A development session rebuilds Go during the check.
    testTimeout: 120_000,
    hookTimeout: 30_000,
  },
});
