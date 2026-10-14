import { availableParallelism } from 'node:os';
import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

const outputDir = '.features-gen';

// Every project collects canonical generated names and nothing else:
// `<outputDir>/features/**/<name>.feature.spec.js`, exactly what bddgen writes
// for each authored feature. Playwright's own default and a bare substring
// pattern both match a Finder-style copy (`x.feature 2.spec.js`, a sibling
// `features 2/` directory), which then runs a second time.
// Each project's pattern is therefore the canonical shape with its own stem,
// and a file that is not canonical belongs to no project.
const canonical = (stem: string) =>
	new RegExp(`/${outputDir.replace('.', '\\.')}/features/(?:[^/]+/)*${stem}\\.feature\\.spec\\.js$`);

const ANY_FEATURE = canonical('[^/]+');
const NOSCRIPT = canonical('[^/]*form-noscript');

const testDir = defineBddConfig({
	features: 'features/**/*.feature',
	steps: 'steps/**/*.ts',
	outputDir
});

// The suite always targets an already-running Go server through BASE_URL.
export default defineConfig({
	testDir,
	globalSetup: './global-setup.ts',
	// Scenarios run in parallel. Each gets a fresh browser context, and the
	// example gives each browser its own todos, gate panels and account serial
	// (example/businesslogic/visitor), so no scenario can move a number another
	// one asserts. A scenario that cannot share the server with its neighbours
	// is a test that derives its expectation from state it did not set up.
	fullyParallel: true,
	// One worker per core, up to six. A 4-vCPU CI runner was
	// fastest at four and no faster past it; a 10-core machine gains little
	// beyond six.
	workers: Math.min(availableParallelism(), 6),
	retries: 0,
	// `list` for the terminal; the HTML report carries a failed scenario's
	// trace and its screenshot at the moment it failed. A passing scenario
	// leaves nothing behind.
	reporter: [
		['list'],
		['html', { open: 'never', outputFolder: 'playwright-report/prod' }]
	],
	outputDir: 'test-results/prod',
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
			testMatch: ANY_FEATURE,
			testIgnore: NOSCRIPT
		},
		{
			name: 'noscript',
			use: { ...devices['Desktop Chrome'], javaScriptEnabled: false },
			testMatch: NOSCRIPT
		}
	]
});
