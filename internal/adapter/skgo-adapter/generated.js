/**
 * The shape `skgo generate` promises in `skgo.remotes.json`, checked without a
 * SvelteKit build around it.
 *
 * The adapter refuses a manifest it cannot read before it copies anything into
 * the build, and the Go module refuses a build whose manifest does not match
 * what it answers. The checks live here, apart from the adapter entry and its
 * kit imports, so they can be exercised directly: the paths they validate are
 * `.ts` in a TypeScript app and `.js` in a JavaScript one, and both are
 * first-class.
 *
 * @param {any} parsed the parsed contents of `skgo.remotes.json`
 * @returns {{ remotes: string[], loads: string[], actions: string[], endpoints: Record<string, string[]> }}
 */
export function validateGenerated(parsed) {
	if (!Array.isArray(parsed.remotes)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `remotes` array. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const id of parsed.remotes) {
		if (typeof id !== 'string' || !/^[^/]+\/[^/]+$/.test(id)) {
			throw new Error(
				`skgo: skgo.remotes.json lists ${JSON.stringify(id)}, which is not a <hash>/<name> id.`
			);
		}
	}
	if (!Array.isArray(parsed.loads)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `loads` array. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const module of parsed.loads) {
		if (typeof module !== 'string' || !/\/\+(page|layout)\.server\.(ts|js)$/.test(module)) {
			throw new Error(
				`skgo: skgo.remotes.json lists ${JSON.stringify(module)}, which is not a +page.server.(ts|js) or +layout.server.(ts|js) path.`
			);
		}
	}
	if (!Array.isArray(parsed.actions)) {
		throw new Error('skgo: skgo.remotes.json has no `actions` array. Run `go generate ./...` before building the frontend.');
	}
	for (const module of parsed.actions) {
		if (typeof module !== 'string' || !/\/\+page\.server\.(ts|js)$/.test(module)) {
			throw new Error(`skgo: skgo.remotes.json lists ${JSON.stringify(module)}, which is not a +page.server.(ts|js) action path.`);
		}
	}
	if (typeof parsed.endpoints !== 'object' || parsed.endpoints === null || Array.isArray(parsed.endpoints)) {
		throw new Error(
			'skgo: skgo.remotes.json has no `endpoints` object. Run `go generate ./...` before building the frontend.'
		);
	}
	for (const [id, methods] of Object.entries(parsed.endpoints)) {
		if (!id.startsWith('/') || !Array.isArray(methods) || methods.length === 0) {
			throw new Error(
				`skgo: skgo.remotes.json maps ${JSON.stringify(id)} to ${JSON.stringify(methods)}, which is not a route id and its methods.`
			);
		}
	}
	return { remotes: parsed.remotes, loads: parsed.loads, actions: parsed.actions, endpoints: parsed.endpoints };
}

/**
 * The check for server routes, against the one place kit reports what it
 * compiled: `builder.routes[].api.methods`, which kit derives by importing each
 * built `+server.js` and reading its exports
 * (packages/kit/src/core/postbuild/analyse.js, `analyse_endpoint`). A `fallback`
 * export travels there as `'*'`, and `skgo generate` writes the same spelling,
 * so the two lists are compared literally.
 *
 * It lives here rather than in the adapter entry so it can be exercised without
 * a SvelteKit build around it, exactly as `validateGenerated` is. That is what
 * lets a test prove it accepts `QUERY` and `'*'` and still refuses a mismatch.
 *
 * This is the check that makes a hand-written `+server.ts` fail the build rather
 * than 404 in the browser: kit would compile it, Go would never have been told
 * about it, and the route would answer nothing.
 *
 * @param {{ routes: Array<{ id: string, api: { methods: string[] } }> }} builder
 * @param {Record<string, string[]>} declared
 * @returns {Map<string, string[]>} the methods kit compiled, per route id
 */
export function checkEndpoints(builder, declared) {
	/** @type {Map<string, string[]>} */
	const built = new Map();
	for (const route of builder.routes) {
		if (route.api.methods.length > 0) built.set(route.id, [...route.api.methods].sort());
	}

	/** @type {string[]} */
	const problems = [];
	for (const [id, methods] of Object.entries(declared)) {
		const compiled = built.get(id);
		if (!compiled) {
			problems.push(`  generated but not compiled: ${methods.join(', ')} ${id}`);
			continue;
		}
		const want = [...methods].sort().join(', ');
		const got = compiled.join(', ');
		if (want !== got) {
			problems.push(`  ${id}: Go answers ${want}, the built +server module exports ${got}`);
		}
	}
	for (const [id, methods] of built) {
		if (!(id in declared)) {
			problems.push(`  compiled but not generated: ${methods.join(', ')} ${id}`);
		}
	}

	if (problems.length) {
		throw new Error(
			'skgo: skgo.remotes.json does not describe the server routes kit just compiled.\n' +
				problems.join('\n') +
				'\n  Every server route is written in Go. Run `go generate ./...`.'
		);
	}
	return built;
}
