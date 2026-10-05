import fs from 'node:fs';
import path from 'node:path';
import { createHash } from 'node:crypto';
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
	.add('origin', {
		type: 'string',
		question: 'What is the Go application origin?',
		default: 'http://127.0.0.1:8080'
	})
	.build();

const propertyName = (property) => {
	if (property.type !== 'Property' || property.computed) return undefined;
	if (property.key.type === 'Identifier') return property.key.name;
	if (property.key.type === 'Literal' || property.key.type === 'StringLiteral') return property.key.value;
	return undefined;
};

const getProperty = (object, name) =>
	object?.type === 'ObjectExpression'
		? object.properties.find((property) => propertyName(property) === name)
		: undefined;

const hasSpread = (object) =>
	object?.type === 'ObjectExpression' &&
	object.properties.some((property) => property.type === 'SpreadElement');

const hasComputedProperty = (object) =>
	object?.type === 'ObjectExpression' &&
	object.properties.some((property) => property.type === 'Property' && property.computed);

const hasDuplicateProperties = (object, names) =>
	object?.type === 'ObjectExpression' &&
	names.some((name) => object.properties.filter((property) => propertyName(property) === name).length > 1);

const setProperty = (object, name, value, js) => {
	const property = getProperty(object, name);
	if (property) {
		property.value = js.common.parseExpression(JSON.stringify(value));
		return;
	}
	js.object.overrideProperties(object, { [name]: value });
};

const setExpressionProperty = (object, name, expression, js) => {
	const property = getProperty(object, name);
	if (property) {
		property.value = js.common.parseExpression(expression);
		return;
	}
	const added = js.common.parseExpression(`({ ${name}: ${expression} })`).properties[0];
	object.properties.push(added);
};

const findDefaultConfig = (ast, js) => {
	const { value } = js.exports.createDefault(ast, { fallback: null });
	if (value?.type === 'ObjectExpression') return value;
	if (
		value?.type === 'CallExpression' &&
		value.arguments.length === 1 &&
		value.arguments[0]?.type === 'ObjectExpression'
	) {
		const callee = value.callee;
		if (callee?.type !== 'Identifier') return null;
		const fromPlaywright = ast.body.some((node) =>
			node.type === 'ImportDeclaration' &&
			node.source.value === '@playwright/test' &&
			node.specifiers.some((specifier) =>
				specifier.type === 'ImportSpecifier' &&
				(specifier.imported?.name ?? specifier.imported?.value) === 'defineConfig' &&
				specifier.local?.name === callee.name
			)
		);
		if (fromPlaywright) return value.arguments[0];
	}
	return null;
};

const repairPlaywright = (sv, cwd, language, origin) => {
	const file = `playwright.config.${language}`;
	if (!fs.existsSync(path.resolve(cwd, file))) return;
	sv.file(file, transforms.script(({ ast, js, content }) => {
		const config = findDefaultConfig(ast, js);
		if (!config) {
			if (content.includes('npm run build && npm run preview')) {
				throw new Error(`skgo cannot repair ${file}: the upstream Playwright launcher is present, but its default export is not a direct object or a single-argument imported defineConfig(object); rewrite the export before adding skgo`);
			}
			return false;
		}
		const webServer = getProperty(config, 'webServer')?.value;
		const command = getProperty(webServer, 'command')?.value;
		const port = getProperty(webServer, 'port')?.value;
		if (command?.type !== 'Literal' && command?.type !== 'StringLiteral') {
			if (content.includes('npm run build && npm run preview')) {
				throw new Error(`skgo cannot repair ${file}: the upstream Playwright launcher is present outside a literal webServer object; rewrite that configuration before adding skgo`);
			}
			return false;
		}
		if (command.value !== 'npm run build && npm run preview') return false;
		const usePropertyCount = config.properties.filter((property) => propertyName(property) === 'use').length;
		if (
			hasSpread(config) ||
			hasComputedProperty(config) ||
			hasDuplicateProperties(config, ['webServer', 'use']) ||
			hasSpread(webServer) ||
			hasComputedProperty(webServer) ||
			hasDuplicateProperties(webServer, ['command', 'port', 'url']) ||
			usePropertyCount > 1
		) {
			throw new Error(`skgo cannot repair ${file}: the upstream Playwright launcher has a spread, computed, or duplicate launcher property, so its effective server settings are ambiguous; simplify that configuration before adding skgo`);
		}
		if (webServer?.type !== 'ObjectExpression' || port?.value !== 4173) {
			throw new Error(`skgo cannot repair ${file}: the upstream Playwright launcher command is present, but its server configuration is not the stock npm/4173 shape`);
		}

		setProperty(webServer, 'command', 'cd .. && just serve', js);
		webServer.properties = webServer.properties.filter((property) => propertyName(property) !== 'port');
		const selectedOrigin = `Reflect.get(globalThis, 'process')?.env?.ORIGIN || ${JSON.stringify(origin)}`;
		setExpressionProperty(webServer, 'url', selectedOrigin, js);
		let use = getProperty(config, 'use')?.value;
		if (!use) {
			use = js.common.parseExpression('defineConfig({})').arguments[0];
			js.object.overrideProperties(config, { use });
		}
		if (
			use.type !== 'ObjectExpression' ||
			hasSpread(use) ||
			hasComputedProperty(use) ||
			hasDuplicateProperties(use, ['baseURL'])
		) {
			throw new Error(`skgo cannot repair ${file}: its "use" configuration is not a plain object or has a spread/computed/duplicate baseURL; simplify it before adding skgo`);
		}
		setExpressionProperty(use, 'baseURL', selectedOrigin, js);
	}));
};

const repairMCP = (sv, cwd, file, serversKey) => {
	if (!fs.existsSync(path.resolve(cwd, file))) return;
	sv.file(file, transforms.json(({ data }) => {
		const servers = data[serversKey];
		const svelte = servers?.svelte;
		if (
			svelte?.command !== 'npx' ||
			!Array.isArray(svelte.args) ||
			svelte.args.length !== 2 ||
			svelte.args[0] !== '-y' ||
			svelte.args[1] !== '@sveltejs/mcp' ||
			svelte.url
		) return;
		svelte.command = './node_modules/.bin/vp';
		svelte.args = ['dlx', '@sveltejs/mcp'];
	}));
};

const repairSvelteFallbacks = (sv, cwd) => {
	// Fingerprints are computed from the untouched SV 1.1.0 output. They keep
	// this repair off edited skill and agent instructions.
	const upstream = {
		skill: 'ccd435bc524f1bdc38f7ca34a3ff279de1e855d3e5165151345d08b85706e95f',
		agent: '50d8ad485802d3ca15a2cf17e50effb1d8707f59edc4466cda5944441b0e181a'
	};
	const files = [
		'.claude/skills/svelte-code-writer/SKILL.md',
		'.cursor/skills/svelte-code-writer/SKILL.md',
		'.gemini/skills/svelte-code-writer/SKILL.md',
		'.github/skills/svelte-code-writer/SKILL.md',
		'.claude/agents/svelte-file-editor.md',
		'.cursor/agents/svelte-file-editor.md',
		'.gemini/agents/svelte-file-editor.md',
		'.github/agents/svelte-file-editor.agent.md'
	];
	for (const file of files) {
		if (!fs.existsSync(path.resolve(cwd, file))) continue;
		const expected = file.includes('/skills/') ? upstream.skill : upstream.agent;
		sv.file(file, (content) => {
			if (!content?.includes('npx @sveltejs/mcp')) return false;
			const digest = createHash('sha256').update(content).digest('hex');
			if (digest !== expected) return false;
			return content
				.replaceAll('npx @sveltejs/mcp', './node_modules/.bin/vp dlx @sveltejs/mcp')
				.replace('Use these commands via `npx`:', 'Use these commands via `./node_modules/.bin/vp dlx`:')
				.replace(/(\/node_modules\/\.bin\/vp dlx @sveltejs\/mcp@latest) -y(?=\s)/g, '$1');
		});
	}
};

const examplesPage = (ts) => `<script${ts ? ' lang="ts"' : ''}>
	import { record, status } from './example.remote';

	let name = $state('Svelte developer');
	let saving = $state(false);
	let greeting = $state('');

	${ts ? '' : '/** @param {SubmitEvent} event */\n\t'}
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
		{#snippet failed(error)}<p class="error">{${ts ? '(error as Error).message' : 'error instanceof Error ? error.message : String(error)'}}</p>{/snippet}
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
		runsAfter('playwright');
		runsAfter('ai-tools');
	},
	run: ({ sv, file, cwd, options, language }) => {
		const adapterVersion = decodeURIComponent(options.adapter);
		const applicationName = decodeURIComponent(options.name);
		const origin = decodeURIComponent(options.origin || 'http://127.0.0.1:8080');
		if (!adapterVersion) throw new Error('skgo requires an explicit adapter version');

		// VitePlus resolves the test runtime from its own dependencies. sv's
		// standalone Vitest add-on must share that version after it is added.
		const vitePlusPackage = path.resolve(cwd, 'node_modules/vite-plus/package.json');
		const vitePlus = fs.existsSync(vitePlusPackage)
			? JSON.parse(fs.readFileSync(vitePlusPackage, 'utf8'))
			: undefined;
		if (vitePlus && vitePlus.version !== '1.0.0') {
			throw new Error(`skgo qualifies VitePlus 1.0.0; found ${vitePlus.version}. Install the qualified version before adding skgo.`);
		}
		const vitestVersion = vitePlus?.dependencies?.vitest;

		sv.file(
			file.package,
			transforms.json(({ data }) => {
				data.name = applicationName;
				// Vitest's upstream add-on writes `npm run`, but VitePlus records
				// pnpm as the only valid package manager in devEngines.
				data.scripts.test = 'pnpm run test:unit --run';
				if (vitestVersion) {
					if (data.devDependencies?.['vitest-browser-svelte']) {
						data.devDependencies['vitest-browser-svelte'] = '3.1.0';
					}
					for (const dependency of Object.keys(data.devDependencies ?? {})) {
						if (dependency === 'vitest' || dependency.startsWith('@vitest/')) {
							data.devDependencies[dependency] = vitestVersion;
						}
					}
				}

				for (const dependency of Object.keys(data.devDependencies ?? {})) {
					if (dependency.startsWith('@sveltejs/adapter-')) delete data.devDependencies[dependency];
					if (options.starter === 'examples' && demoDependencies.includes(dependency)) {
						delete data.devDependencies[dependency];
					}
				}
			})
		);
		sv.devDependency('@skgo/sveltekit-adapter', adapterVersion);
		if (language !== 'ts' && fs.existsSync(path.resolve(cwd, 'src/lib/vitest-examples/greet.js'))) {
			// sv's Vitest fixture has an untyped parameter under a JS app's
			// strict checkJs config. Keep the app's initial check green.
			sv.file('src/lib/vitest-examples/greet.js', (content) =>
				content.replace('export function greet(name) {', '/** @param {string} name */\nexport function greet(name) {')
			);
		}
		// VitePlus preserves this native pnpm project policy when it adds its
		// catalog. It applies only to the generated project's installs; the
		// separate Storybook dlx environment receives its own explicit flag.
		sv.file('pnpm-workspace.yaml', pnpm.allowBuilds({ cwd, packages: ['esbuild'] }));
		if (vitePlus?.version === '1.0.0') {
			// Storybook 10.6.1's optional peer predates VitePlus 1.0. The
			// generated Storybook build is qualified with this exact pair.
			sv.file('pnpm-workspace.yaml', transforms.yaml(({ data }) => {
				const rules = data.get('peerDependencyRules');
				const value = rules?.toJSON?.() ?? rules ?? {};
				value.allowedVersions ??= {};
				value.allowedVersions['storybook@10.6.1>vite-plus'] = '1.0.0';
				data.set('peerDependencyRules', value);
			}));
		}

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

		repairPlaywright(sv, cwd, language, origin);
		repairMCP(sv, cwd, '.mcp.json', 'mcpServers');
		repairMCP(sv, cwd, '.cursor/mcp.json', 'mcpServers');
		repairMCP(sv, cwd, '.gemini/settings.json', 'mcpServers');
		repairMCP(sv, cwd, '.vscode/mcp.json', 'servers');
		repairSvelteFallbacks(sv, cwd);
	},
	nextSteps: () => []
});
