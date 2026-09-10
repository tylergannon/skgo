import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

// The same scenarios run against the embedded production build and the live
// Vite module graph. The label separates their reports and screenshots; it
// never changes which scenarios exist or what they assert.
const run = process.env.SKGO_E2E_RUN ?? 'run';

const testDir = defineBddConfig({
	features: 'features/**/*.feature',
	steps: 'steps/**/*.ts',
	outputDir: '.features-gen'
});

// The suite always targets an already-running Go server through BASE_URL.
export default defineConfig({
	testDir,
	globalSetup: './global-setup.ts',
	fullyParallel: false,
	workers: 1,
	retries: 0,
	// `list` for the terminal; the HTML report is where the frames each
	// scenario left behind can be looked at afterwards, which is the point of
	// taking them.
	reporter: [
		['list'],
		['html', { open: 'never', outputFolder: `playwright-report/${run}` }]
	],
	outputDir: `test-results/${run}`,
	use: {
		baseURL: process.env.BASE_URL,
		trace: 'retain-on-failure',
		screenshot: 'only-on-failure'
	},
	// Two projects, because one claim in the suite is about a browser that runs
	// no JavaScript at all and that is a property of the context, not of a
	// step. `form-noscript.feature` is the only feature that belongs to the
	// second one, and it is excluded from the first — a scenario about the
	// non-enhanced path would prove nothing in a browser where kit's client
	// intercepts the submit.
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] },
			testIgnore: /form-noscript/
		},
		{
			name: 'noscript',
			use: { ...devices['Desktop Chrome'], javaScriptEnabled: false },
			testMatch: /form-noscript/
		}
	]
});
