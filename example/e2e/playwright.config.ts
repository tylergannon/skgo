import { availableParallelism } from 'node:os';
import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

// The same scenarios run against the embedded production build and the live
// Vite module graph. The label separates their reports and failure artifacts;
// it never changes which scenarios exist or what they assert.
const run = process.env.SKGO_E2E_RUN ?? 'run';
const sourceEditStage = process.env.SKGO_E2E_STAGE === 'source-edit';

const outputDir = '.features-gen';

// Every project collects canonical generated names and nothing else:
// `<outputDir>/features/**/<name>.feature.spec.js`, exactly what bddgen writes
// for each authored feature. Playwright's own default and a bare substring
// pattern both match a Finder-style copy (`x.feature 2.spec.js`, a sibling
// `features 2/` directory), which then runs a second time — and for the
// source-edit project, races a duplicate of the scenario that edits the app.
// Each project's pattern is therefore the canonical shape with its own stem,
// and a file that is not canonical belongs to no project.
const canonical = (stem: string) =>
	new RegExp(`/${outputDir.replace('.', '\\.')}/features/(?:[^/]+/)*${stem}\\.feature\\.spec\\.js$`);

const ANY_FEATURE = canonical('[^/]+');
const NOSCRIPT = canonical('[^/]*form-noscript');

// Edits source under the running dev server, whose hot updates reach every
// open page. The Go e2e runner invokes this project only after the regular
// chromium and noscript projects have completed.
const SOURCE_EDIT = canonical('zz-source-update');

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
	// One worker per core, up to six, in both modes. A 4-vCPU CI runner was
	// fastest at four and no faster past it; a 10-core machine gains little
	// beyond six. Dev scales like prod, so it needs no lower count.
	workers: Math.min(availableParallelism(), 6),
	retries: 0,
	// `list` for the terminal; the HTML report carries a failed scenario's
	// trace and its screenshot at the moment it failed. A passing scenario
	// leaves nothing behind.
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
	//
	// A third, `source-edit`, is not about shared state but about the dev
	// server itself: editing a component or adding a route makes Vite update or
	// reload every open page, whichever scenario it belongs to. The Go e2e runner
	// runs this project in a second Playwright invocation so failures in the
	// regular projects cannot cause Playwright's dependency scheduler to skip it.
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] },
			testMatch: ANY_FEATURE,
			testIgnore: [NOSCRIPT, SOURCE_EDIT]
		},
		{
			name: 'source-edit',
			// dev.ts already permits 120s for generation and startup. The
			// scenario must allow that wait instead of Playwright's default
			// 30s, which removes newly added sources while Go still reads them.
			timeout: 120_000,
			use: { ...devices['Desktop Chrome'] },
			testMatch: SOURCE_EDIT,
			fullyParallel: false,
			// Keep direct Playwright runs safe. The Go runner selects this project
			// in its independent second invocation, where dependencies would
			// schedule regular scenarios again in the same process.
			...(sourceEditStage ? {} : { dependencies: ['chromium', 'noscript'] })
		},
		{
			name: 'noscript',
			use: { ...devices['Desktop Chrome'], javaScriptEnabled: false },
			testMatch: NOSCRIPT
		}
	]
});
