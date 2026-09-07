import { defineConfig, devices } from '@playwright/test';
import { defineBddConfig } from 'playwright-bdd';

// Which mode this run is for. It decides two things: what the suite tells the
// server it expects to be answering (EXPECTED_MODE), and which scenarios exist
// at all.
//
// A handful of claims are true in one mode and not the other, because the two
// modes really do behave differently: in prod Go renders the document, and in
// dev kit's own server owns it and the app turns server rendering off (see
// web/src/routes/+layout.ts). Rather than let a step quietly mean two things,
// each such scenario is written twice — `@prod` beside `@dev` — and the run
// generates only the half that belongs to it. Everything untagged runs in both.
const mode = process.env.EXPECTED_MODE === 'dev' ? 'dev' : 'prod';

const testDir = defineBddConfig({
	features: 'features/**/*.feature',
	steps: 'steps/**/*.ts',
	tags: mode === 'dev' ? 'not @prod' : 'not @dev',
	// Per mode, so a run cannot execute the spec files the other mode's
	// generation left behind if bddgen ever fails to overwrite them.
	outputDir: `.features-gen/${mode}`
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
