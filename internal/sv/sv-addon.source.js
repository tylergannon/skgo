import fs from 'node:fs';
import path from 'node:path';
import { defineAddon, defineAddonOptions } from 'sv';
import { pnpm, svelteConfig, transforms } from '@sveltejs/sv-utils';

const options = defineAddonOptions()
	.add('starter', {
		type: 'select',
		question: 'Which skgo starting point should be installed?',
		default: 'minimal',
		options: [
			{ value: 'minimal', label: 'minimal' },
			{ value: 'examples', label: 'examples' }
		]
	})
	.add('adapter', {
		type: 'string',
		question: 'Which adapter version should be installed?',
		default: ''
	})
	.add('name', {
		type: 'string',
		question: 'What is the application name?',
		default: 'skgo-app'
	})
	.build();

const examplesPage = (ts) => `<script${ts ? ' lang="ts"' : ''}>
	import { record, status } from './example.remote';

	let name = $state('Svelte developer');
	let saving = $state(false);
	let greeting = $state('');

	async function submit(event${ts ? ': SubmitEvent' : ''}) {
		event.preventDefault();
		if (saving) return;
		saving = true;
		try {
			const result = await record(name).updates(status());
			greeting = result.message;
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head><title>skgo examples</title></svelte:head>

<main>
	<p class="eyebrow">SvelteKit on the frontend · Go on the server</p>
	<h1>skgo examples</h1>
	<svelte:boundary>
		{@const current = await status()}
		<p data-testid="go-message">{current.message}</p>
		<p>Writes handled by Go: <strong data-testid="write-count">{current.writes}</strong></p>
		{#if greeting}<p data-testid="go-greeting">{greeting}</p>{/if}
		{#snippet failed(error)}<p class="error">{${ts ? '(error as Error)' : 'error'}.message}</p>{/snippet}
	</svelte:boundary>
	<form onsubmit={submit}>
		<label for="name">Who should Go greet?</label>
		<div>
			<input id="name" bind:value={name} />
			<button type="submit" disabled={saving}>{saving ? 'Writing…' : 'Write through Go'}</button>
		</div>
	</form>
</main>

<style>
	:global(body) { margin: 0; background: #f3f0e8; color: #17211b; font-family: ui-sans-serif, system-ui, sans-serif; }
	main { max-width: 46rem; margin: 12vh auto; padding: 3rem; background: #fffdf7; border: 1px solid #d8d1c2; border-radius: 1.25rem; box-shadow: 0 1.5rem 4rem #1f33201f; }
	.eyebrow { color: #326b4a; font-weight: 700; letter-spacing: .05em; text-transform: uppercase; }
	h1 { margin: .25rem 0 2rem; font-family: ui-serif, Georgia, serif; font-size: clamp(3rem, 8vw, 5.5rem); line-height: .9; }
	form { margin-top: 2.5rem; padding-top: 2rem; border-top: 1px solid #d8d1c2; }
	label { display: block; margin-bottom: .6rem; font-weight: 700; }
	form div { display: flex; gap: .75rem; }
	input, button { border-radius: .55rem; padding: .8rem 1rem; font: inherit; }
	input { flex: 1; border: 1px solid #9caa9e; }
	button { border: 0; background: #173f2a; color: white; font-weight: 700; cursor: pointer; }
	button:disabled { opacity: .6; }
	.error { color: #9d2d24; }
</style>
`;

// Everything sv's demo template puts on top of the minimal one. The demo is a
// JavaScript server application (Sverdle's form actions and cookies); the skgo
// examples starting point replaces it rather than shipping routes Go cannot answer.
const demoPaths = [
	'src/routes/sverdle',
	'src/routes/about',
	'src/routes/Header.svelte',
	'src/routes/Counter.svelte',
	'src/routes/+page.js',
	'src/routes/+page.ts',
	'src/lib/images'
];
const demoDependencies = ['@fontsource/fira-mono', '@neoconfetti/svelte'];

export default defineAddon({
	id: 'skgo',
	shortDescription: 'Go application server integration',
	homepage: 'https://github.com/tylergannon/skgo',
	options,
	setup: ({ isKit, unsupported, runsAfter }) => {
		if (!isKit) unsupported('Requires SvelteKit');
		runsAfter('vitest');
	},
	run: ({ sv, file, cwd, options, language }) => {
		const adapterVersion = decodeURIComponent(options.adapter);
		const applicationName = decodeURIComponent(options.name);
		if (!adapterVersion) throw new Error('skgo requires an explicit adapter version');

		sv.file(
			file.package,
			transforms.json(({ data }) => {
				data.name = applicationName;
				// Vitest's upstream add-on writes `npm run`, but VitePlus records
				// pnpm as the only valid package manager in devEngines.
				data.scripts.test = 'pnpm run test:unit --run';
				for (const dependency of Object.keys(data.devDependencies ?? {})) {
					if (dependency.startsWith('@sveltejs/adapter-')) delete data.devDependencies[dependency];
					if (options.starter === 'examples' && demoDependencies.includes(dependency)) {
						delete data.devDependencies[dependency];
					}
				}
			})
		);
		sv.devDependency('@skgo/sveltekit-adapter', adapterVersion);
		// VitePlus preserves this native pnpm project policy when it adds its
		// catalog. It applies only to the generated project's installs; the
		// separate Storybook dlx environment receives its own explicit flag.
		sv.file('pnpm-workspace.yaml', pnpm.allowBuilds('esbuild'));
		sv.file('.gitignore', (content) => {
			const outputRule = '\n/build\n';
			if (content.includes('!/build/.gitkeep')) return false;
			if (!content.includes(outputRule)) {
				throw new Error('skgo expected sv to ignore the SvelteKit build directory');
			}
			return content.replace(outputRule, '\n/build/*\n!/build/.gitkeep\n');
		});

		svelteConfig.edit({ sv, cwd }, ({ ast, override, js }) => {
			const adapterImport = ast.body
				.filter((node) => node.type === 'ImportDeclaration')
				.find(
					(node) =>
						typeof node.source.value === 'string' &&
						node.source.value.startsWith('@sveltejs/adapter-') &&
						node.importKind === 'value'
				);
			let adapterName = 'skgo';
			if (adapterImport) {
				adapterImport.source.value = '@skgo/sveltekit-adapter';
				adapterImport.source.raw = undefined;
				const defaultSpecifier = adapterImport.specifiers?.find(
					(specifier) => specifier.type === 'ImportDefaultSpecifier'
				);
				if (defaultSpecifier) adapterName = defaultSpecifier.local.name;
			} else {
				js.imports.addDefault(ast, { from: '@skgo/sveltekit-adapter', as: adapterName });
			}
			override(
				{
					adapter: js.functions.createCall({ name: adapterName, args: [], useIdentifiers: true }),
					compilerOptions: { experimental: { async: true } },
					experimental: { remoteFunctions: true }
				},
				{ dropLeadingComments: ['adapter'] }
			);
		});

		if (options.starter === 'examples') {
			if (fs.existsSync(path.resolve(cwd, 'src/routes/Header.svelte'))) {
				// The demo's layout and stylesheet are also where other add-ons put
				// theirs (Tailwind's import), so they are reduced, not removed.
				let stylesheet = false;
				sv.file('src/routes/layout.css', (content) => {
					const kept = content
						.split('\n')
						.filter((line) => /^@(import|plugin)\s+['"](tailwindcss|@tailwindcss\/)/.test(line));
					stylesheet = kept.length > 0;
					return stylesheet ? kept.join('\n') + '\n' : false;
				});
				if (!stylesheet) fs.rmSync(path.resolve(cwd, 'src/routes/layout.css'), { force: true });
				sv.file(
					'src/routes/+layout.svelte',
					() =>
						`<script${language === 'ts' ? ' lang="ts"' : ''}>\n` +
						(stylesheet ? "\timport './layout.css';\n\n" : '') +
						'\tlet { children } = $props();\n</script>\n\n{@render children()}\n'
				);
			}
			for (const demoPath of demoPaths) {
				fs.rmSync(path.resolve(cwd, demoPath), { recursive: true, force: true });
			}
			sv.file('src/routes/+page.svelte', () => examplesPage(language === 'ts'));
		}
	},
	nextSteps: () => []
});
