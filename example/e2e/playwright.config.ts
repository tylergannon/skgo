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
	// `list` for the terminal; the HTML report is where the frames each
	// scenario left behind can be looked at afterwards, which is the point of
	// taking them.
	reporter: [
		['list'],
		['html', { open: 'never', outputFolder: `playwright-report/${process.env.EXPECTED_MODE ?? 'unknown'}` }]
	],
	outputDir: `test-results/${process.env.EXPECTED_MODE ?? 'unknown'}`,
	use: {
		baseURL: process.env.BASE_URL,
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure'
	},
	projects: [{ name: 'chromium', use: { ...devices['Desktop Chrome'] } }]
});
