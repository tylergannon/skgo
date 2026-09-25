import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

// The same scenarios run against the embedded production build and the live
// Vite module graph. The label separates their reports and failure artifacts;
// it never changes which scenarios exist or what they assert.
const run = process.env.SKGO_E2E_RUN ?? 'run';

const TODOS_STORE = /\/(auth|remote|live|csp|refresh)\.feature/;
const FILE_SERIAL = /\/gate\.feature/;
// Edits source under the running dev server, whose hot updates reach every
// open page, so it runs alone after everything else has finished.
const SOURCE_EDIT = /\/zz-source-update\.feature/;

const testDir = defineBddConfig({
	features: 'features/**/*.feature',
	steps: 'steps/**/*.ts',
	outputDir: '.features-gen'
});

// The suite always targets an already-running Go server through BASE_URL.
export default defineConfig({
	testDir,
	globalSetup: './global-setup.ts',
	// Scenarios run in parallel. A browser scenario that cannot share the
	// server with its neighbours is a test that derives its expectation from
	// state it did not set up; the two lanes below hold the ones that still do,
	// until the example scopes that state per visitor and the lanes go away.
	fullyParallel: true,
	workers: 4,
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
	// Temporary lanes for scenarios that race on the example's shared stores.
	// `todos-store`: auth, remote, live, csp and refresh all write to or count the one
	// global todo list (a rename pushes a live-board frame too), across files, so
	// they run one at a time. `file-serial`: gate races only with itself, so it runs its
	// scenarios in order beside everything else. Delete
	// both lanes when the stores are scoped per visitor.
	projects: [
		{
			name: 'chromium',
			use: { ...devices['Desktop Chrome'] },
			testIgnore: [/form-noscript/, TODOS_STORE, FILE_SERIAL, SOURCE_EDIT]
		},
		{
			name: 'todos-store',
			use: { ...devices['Desktop Chrome'] },
			testMatch: TODOS_STORE,
			fullyParallel: false,
			workers: 1
		},
		{
			name: 'file-serial',
			use: { ...devices['Desktop Chrome'] },
			testMatch: FILE_SERIAL,
			fullyParallel: false
		},
		{
			name: 'source-edit',
			use: { ...devices['Desktop Chrome'] },
			testMatch: SOURCE_EDIT,
			fullyParallel: false,
			dependencies: ['chromium', 'todos-store', 'file-serial', 'noscript']
		},
		{
			name: 'noscript',
			use: { ...devices['Desktop Chrome'], javaScriptEnabled: false },
			testMatch: /form-noscript/
		}
	]
});
