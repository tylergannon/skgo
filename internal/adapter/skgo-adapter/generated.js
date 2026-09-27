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
