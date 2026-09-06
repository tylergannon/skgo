import { defineConfig, devices } from '@playwright/test';

// The suite always targets an already-running Go server on BASE_URL. Every
// scenario screenshots the page at the moment it asserts, into shots/.
export default defineConfig({
	testDir: '.',
	testMatch: 'specs/*.spec.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	timeout: 30_000,
	reporter: 'list',
	use: {
		baseURL: process.env.BASE_URL ?? 'http://127.0.0.1:8090',
		trace: 'retain-on-failure'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
