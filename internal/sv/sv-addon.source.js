import { defineAddon, defineAddonOptions } from 'sv';
import { svelteConfig, transforms } from '@sveltejs/sv-utils';

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

const examplesPage = `<script lang="ts">
	import { record, status } from './example.remote';

	let name = $state('Svelte developer');
	let saving = $state(false);
	let greeting = $state('');

	async function submit(event: SubmitEvent) {
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
		{#snippet failed(error)}<p class="error">{(error as Error).message}</p>{/snippet}
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

export default defineAddon({
	id: 'skgo',
	shortDescription: 'Go application server integration',
	homepage: 'https://github.com/tylergannon/skgo',
	options,
	setup: ({ isKit, unsupported, runsAfter }) => {
		if (!isKit) unsupported('Requires SvelteKit');
		runsAfter('vitest');
	},
	run: ({ sv, file, cwd, options }) => {
		const adapterVersion = decodeURIComponent(options.adapter);
		const applicationName = decodeURIComponent(options.name);
		if (!adapterVersion) throw new Error('skgo requires an explicit adapter version');

		sv.file(
			file.package,
			transforms.json(({ data }) => {
				data.name = applicationName;
				// Vitest's upstream add-on writes `npm run`, but VitePlus records
				// pnpm as the only valid package manager in devEngines.
				data.scripts.test = 'pnpm run test:unit -- --run';
				for (const dependency of Object.keys(data.devDependencies ?? {})) {
					if (dependency.startsWith('@sveltejs/adapter-')) delete data.devDependencies[dependency];
				}
			})
		);
		sv.devDependency('@skgo/sveltekit-adapter', adapterVersion);
		// VitePlus preserves this native pnpm project policy when it adds its
		// catalog. It applies only to the generated project's installs; the
		// separate Storybook dlx environment receives its own explicit flag.
		sv.file('pnpm-workspace.yaml', (content) => {
			if (content.trim()) {
				throw new Error('skgo expected sv to create pnpm-workspace.yaml after add-ons run');
			}
			return 'allowBuilds:\n  esbuild: true\n';
		});
		sv.file('.gitignore', (content) => {
			const outputRule = '\n/build\n';
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
			sv.file('src/routes/+page.svelte', () => examplesPage);
		}
	},
	nextSteps: () => []
});
