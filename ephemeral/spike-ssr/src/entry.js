// SPIKE. The only JavaScript skgo would run at request time: kit's `Root` with
// kit's `Props`/`RenderNode`, through `render()` from `svelte/server`.
//
// It mirrors packages/kit/src/runtime/server/page/render.js:142-261 and nothing
// else. Every string the browser gets around this (`__sveltekit_*`, the
// devalue'd data, CSP, app.html substitution) is Go's job.
import './polyfill.js';
import { render } from 'svelte/server';
import Root from 'skgo:kit/root';
import { Props, RenderNode } from 'skgo:kit/props';
import { with_request_store, try_get_request_store } from '@sveltejs/kit/internal/server';
import { create_remote_key } from 'skgo:kit/shared';
import { components } from 'skgo:nodes';

/**
 * A `RequestState`, per packages/kit/src/types/internal.d.ts:659. Only the
 * fields kit's remote wrappers read during a render are populated.
 */
function make_state() {
	return {
		getClientAddress: () => '127.0.0.1',
		error: false,
		rerouted_url: null,
		depth: 0,
		remote: {
			data: null,
			implicit: null,
			explicit: null,
			forms: null,
			requested: null,
			batches: null,
			live_iterators: null
		},
		is_in_remote_function: false,
		is_in_render: true
	};
}

/**
 * A `RequestEvent` stand-in. `run_remote_function` spreads it and wraps
 * `cookies`; `refresh()` reads `isRemoteRequest`. Go owns the real request, so
 * the URL arrives already parsed rather than being reconstructed here (goja has
 * no `URL`).
 */
function make_event(req) {
	return {
		cookies: {
			get: (name) => req.cookies?.[name],
			getAll: () => Object.entries(req.cookies ?? {}).map(([name, value]) => ({ name, value })),
			set: () => {},
			delete: () => {},
			serialize: () => ''
		},
		fetch: () => {
			throw new Error('skgo spike: fetch during render is not available');
		},
		getClientAddress: () => '127.0.0.1',
		locals: {},
		params: req.params ?? {},
		platform: undefined,
		request: { headers: { get: () => null }, method: 'GET' },
		route: { id: req.route_id ?? null },
		setHeaders: () => {},
		url: req.url,
		isDataRequest: false,
		isSubRequest: false,
		isRemoteRequest: false,
		tracing: { enabled: false }
	};
}

/**
 * @param {{
 *   url: object,               parsed by Go
 *   route_id: string | null,
 *   params: Record<string,string>,
 *   status: number,
 *   error: { message: string, status?: number } | null,
 *   form: any,
 *   branch: Array<{ node: number, data: Record<string, any> | null }>,
 *   error_components: Array<number | null>
 * }} req
 */
function build_props(req) {
	const page = {
		error: req.error ?? null,
		params: req.params ?? {},
		route: { id: req.route_id ?? null },
		status: req.status ?? 200,
		url: req.url,
		data: {},
		form: req.form ?? null,
		shallow: null,
		state: {}
	};

	const branch = req.branch;
	const error_components = (req.error_components ?? []).map((i) =>
		i == null ? undefined : components[i]
	);

	const props = new Props({
		page,
		tree: new RenderNode(components[branch[0].node], undefined),
		form: req.form ?? null,
		error: req.error ?? undefined
	});

	let current_node = props.tree;
	let data = props.page.data;

	for (let i = 0; i < branch.length; i += 1) {
		data = { ...data, ...branch[i].data };
		current_node.data = data;

		if (i < branch.length - 1) {
			current_node = current_node.child = new RenderNode(
				components[branch[i + 1].node],
				error_components[i + 1]
			);
		}
	}

	props.page.data = data;
	return props;
}

/**
 * Everything `await`ed during the render that kit would serialise into
 * `__sveltekit.data`, keyed the way the client's query cache will look it up:
 * `create_remote_key(internals.id, payload)` === `<hash>/<name>/<payload>`.
 * (packages/kit/src/runtime/server/remote-functions.js:433-484)
 */
function collect(state) {
	const out = {};
	if (!state.remote.implicit) return out;
	for (const [internals, record] of state.remote.implicit) {
		if (!internals.id) continue;
		for (const key in record) {
			const remote_key =
				internals.type === 'form' ? key : create_remote_key(internals.id, key);
			const type = internals.type === 'query_live' ? 'l' : internals.type[0];
			const promise = state.remote.data?.get(internals)?.[key] ?? record[key]();
			(out[type] ??= {})[remote_key] = { pending: promise };
		}
	}
	return out;
}

globalThis.__skgo_render = function (req_json) {
	const req = JSON.parse(req_json);
	const props = build_props(req);
	const state = make_state();
	const event = make_event(req);

	const options = {
		context: new Map([['__request__', { page: props.page }]])
	};

	const result = { done: false, error: null, head: '', body: '', data: {} };

	const promise = with_request_store({ event, state }, () => render(Root, { ...options, props }));

	Promise.resolve(promise).then(
		(rendered) => {
			result.head = rendered.head;
			result.body = rendered.body;
			const collected = collect(state);
			const settled = {};
			const waits = [];
			for (const type in collected) {
				for (const key in collected[type]) {
					waits.push(
						Promise.resolve(collected[type][key].pending).then(
							(v) => ((settled[type] ??= {})[key] = { v }),
							(e) => ((settled[type] ??= {})[key] = { e: { message: String(e && e.message) } })
						)
					);
				}
			}
			return Promise.all(waits).then(() => {
				result.data = settled;
				result.done = true;
			});
		},
		(err) => {
			result.error = (err && (err.stack || err.message)) || String(err);
			result.done = true;
		}
	);

	return result;
};

// A cheap liveness check Go can call before trusting anything else.
globalThis.__skgo_ping = function () {
	return 'ok';
};

// Evidence, not plumbing. After a render, kit's request store should be gone.
// If it is still there, whatever `run_remote_function` last put in it has
// leaked past the end of the render — which is what a shim that never restores
// the previous store necessarily does, and what kit itself does in its own
// no-AsyncLocalStorage mode (event.js:82, `if (!IN_WEBCONTAINER)`).
globalThis.__skgo_leaked_store = function () {
	const store = try_get_request_store();
	if (!store) return JSON.stringify({ leaked: false });
	return JSON.stringify({
		leaked: true,
		is_in_remote_function: !!store.state?.is_in_remote_function,
		is_in_remote_query: !!store.state?.is_in_remote_query
	});
};
