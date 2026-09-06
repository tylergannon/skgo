import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

const testDir = defineBddConfig({
	features: 'features/**/*.feature',
	steps: 'steps/**/*.ts'
});

// The suite always targets an already-running server: BASE_URL points at Go,
// and EXPECTED_MODE says which handler should be answering there.
export default defineConfig({
	testDir,
	fullyParallel: false,
	workers: 1,
	retries: 0,
	reporter: 'list',
	use: {
		baseURL: process.env.BASE_URL,
		trace: 'retain-on-failure'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
