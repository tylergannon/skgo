// The entry point. It mirrors the one region of kit's `render_response` that
// executes code (packages/kit/src/runtime/server/page/render.js): build the
// `Props` linked list, call `render(Root, ...)` inside kit's request store, and
// hand back `{ head, body }`. Everything on either side of that call — the boot
// script, the hydration array, the remote data, the head buckets, the template
// — is string assembly Go does with data Go already has.
//
// It is the `goja` environment's input, so kit's own build compiles what it
// reaches: the app's components through vite-plugin-svelte, its `.remote.ts`
// modules through kit's remote plugin, its TypeScript through vite. The
// `skgo:` specifiers are the kit internals `$app/*` does not name; the
// adapter's plugin resolves them to files inside kit itself.
import { render } from 'svelte/server';
import Root from 'skgo:kit/root';
import { Props, RenderNode } from 'skgo:kit/props';
import {
	with_request_store,
	HttpError,
	Redirect,
	SvelteKitError,
	ValidationError
} from '@sveltejs/kit/internal/server';
import { HandledHttpError } from '@sveltejs/kit/internal';
import * as devalue from 'skgo:devalue';
import { decoders, init_transport, parse } from 'skgo:kit/transport';
import { components, universalLoads } from 'skgo:nodes';
import { transport } from 'skgo:hooks';
import { serialize_data } from 'skgo:kit/serialize-data';
import { create_universal_fetch } from 'skgo:kit/load-data';

/**
 * The app's transport hook, installed the way kit installs it
 * (runtime/server/index.js: init_transport(module.transport ?? {})).
 *
 * Kit does it per request because it loads the hooks with a dynamic import;
 * the hooks are static and this bundle has no top-level await, so it happens
 * once per runtime instead. From here on, parse() is the app's own decoders
 * and a value Go serialized under a transport key comes back as an instance
 * of the class src/hooks.ts declares.
 */
init_transport(transport ?? {});

/**
 * kit's handle_error_and_jsonify (runtime/server/errors.js) with no
 * handleError hook: an HttpError keeps the body the app chose, a framework
 * error keeps its status and text, a validation failure is a Bad Request, and
 * anything else is Internal Error with the real cause left on the server.
 * skgo has no handleError hook, so there is nothing here to await or merge.
 */
function handle_error(error) {
	if (error instanceof HandledHttpError) return error.body;
	if (error instanceof HttpError) return error.body;
	if (error instanceof SvelteKitError) return { status: error.status, message: error.text };
	if (error instanceof ValidationError) return { status: 400, message: 'Bad Request' };
	return { status: 500, message: 'Internal Error' };
}

/**
 * A RequestState (packages/kit/src/types/internal.d.ts). Only the fields
 * kit's remote wrappers read during a render are populated. is_in_render is
 * what makes kit refuse a command.
 */
function make_state(req) {
	return {
		getClientAddress: () => __skgo_client_address(),
		error: false,
		rerouted_url: null,
		depth: req.is_sub_request ? 1 : 0,
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

const bytes_to_base64 = (bytes) => {
	let binary = '';
	for (let i = 0; i < bytes.length; i += 0x8000) {
		binary += String.fromCharCode.apply(null, bytes.subarray(i, i + 0x8000));
	}
	return btoa(binary);
};

const base64_to_bytes = (encoded) => {
	const binary = atob(encoded);
	const bytes = new Uint8Array(binary.length);
	for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
	return bytes;
};

// Statuses a real Response cannot carry a body for (runtime/server/constants.js).
const NULL_BODY = [101, 204, 205, 304];

/**
 * Every response header a fetch answered with, and whether the app's
 * `filterSerializedResponseHeaders` lets Go replay it into the document. Go
 * evaluates that filter, so the one kit hands its serializer answers from what
 * Go said: the same name and value always get the same answer.
 *
 * @param {Set<string>} serialized
 */
function resolve_options(serialized) {
	return {
		filterSerializedResponseHeaders: (name, value) => serialized.has(name + ':' + value)
	};
}

/**
 * Kit's `event.fetch` (runtime/server/fetch.js) with its whole body — the
 * handleFetch hook, credential inheritance, internal dispatch, cookie
 * write-back — done by Go. What stays here is what the engine owns: turning
 * what a component passed into a Request, and the bytes Go answers with into a
 * Response.
 *
 * @param {object} req
 * @param {URL} url
 * @param {Set<string>} serialized
 */
function create_fetch(req, url, serialized) {
	return async function (input, init) {
		const request = input instanceof Request ? input : new Request(new URL(input, url).href, init);
		const target = new URL(request.url, url);
		const headers = {};
		request.headers.forEach((value, name) => { headers[name] = value; });
		const envelope = {
			method: request.method || 'GET', url: target.href, headers,
			credentials: request.credentials, mode: request.mode
		};
		if (request.body !== null) {
			const bytes = new Uint8Array(await request._consume());
			if (bytes.length) envelope.bodyBase64 = bytes_to_base64(bytes);
		}
		const answer = JSON.parse(await globalThis.__skgo_fetch(JSON.stringify(envelope)));
		if (answer.error) throw new TypeError(answer.error);
		const value = answer.response;
		for (const [name, header, allowed] of value.headers ?? []) {
			if (allowed) serialized.add(name + ':' + header);
		}
		let body = null;
		if (!NULL_BODY.includes(value.status) && request.method !== 'HEAD') {
			body = value.bodyBase64 ? base64_to_bytes(value.bodyBase64) : value.body ?? '';
		}
		return new Response(body, {
			status: value.status,
			statusText: value.statusText,
			headers: (value.headers ?? []).map(([name, header]) => [name, header])
		});
	};
}

/**
 * A RequestEvent stand-in. run_remote_function spreads it and derives the
 * event a query actually sees, which is where kit makes url, params and
 * route throw. Go owns the real request; the only I/O anything here performs
 * is `fetch`'s in-process call back into Go for one of the app's own routes.
 */
function make_event(req, url, serialized) {
	return {
		cookies: {
			get: (name) => (req.cookies ?? {})[name],
			getAll: () => Object.entries(req.cookies ?? {}).map(([name, value]) => ({ name, value })),
			set: () => {},
			delete: () => {},
			serialize: () => ''
		},
		fetch: create_fetch(req, url, serialized),
		getClientAddress: () => __skgo_client_address(),
		locals: {},
		params: req.params ?? {},
		platform: undefined,
		request: { headers: new Headers(req.headers), method: req.method ?? 'GET' },
		route: { id: req.route_id ?? null },
		setHeaders: () => {},
		url,
		isDataRequest: !!req.is_data_request,
		isSubRequest: !!req.is_sub_request,
		isRemoteRequest: !!req.is_remote_request,
		tracing: { enabled: false }
	};
}

/**
 * One node's load result. Go sends devalue's flat form — the same bytes it
 * sends the client in __data.json, produced by the same encoders — and the
 * app's decoders read it back, so the component renders against the instance
 * the browser is about to hold rather than the object its fields travelled in.
 *
 * A value the load promised is in those bytes as kit's own placeholder, and it
 * is read back the way kit's client reads it (process_stream in client.js): a
 * Promise reviver alongside the app's decoders. So a promise is found wherever
 * the load left one, at any depth, which is where devalue's reducer put it.
 *
 * The promise it becomes never settles. Svelte's server renderer does not await
 * an await block — it pushes the block marker and renders the pending branch
 * (svelte/src/internal/server/index.js, await_block) — so the document leaves Go
 * with the loading state already in it, and the value follows it down as a chunk
 * Go appends. That is exactly what kit does, which hands its renderer the
 * promise itself.
 */
function node_data(node) {
	if (!node.data) return null;
	return devalue.parse(node.data, {
		...decoders,
		Promise: () => new Promise(() => {})
	});
}

async function build_props(req, url, event, state, fetched, serialized) {
	const form = req.form ? devalue.parse(req.form, decoders) : null;
	const page = {
		error: req.error ?? null,
		params: req.params ?? {},
		route: { id: req.route_id ?? null },
		status: req.status ?? 200,
		url,
		data: {},
		form,
		shallow: null,
		state: {}
	};

	const branch = req.branch ?? [];
	const error_components = (req.error_components ?? []).map((i) =>
		i == null ? undefined : components[i]
	);

	const props = new Props({
		page,
		tree: new RenderNode(components[branch[0].node], undefined),
		form,
		error: req.error ?? undefined
	});

	let current_node = props.tree;
	let data = props.page.data;

	for (let i = 0; i < branch.length; i += 1) {
		const server_data = node_data(branch[i]);
		const load = universalLoads[branch[i].node];
		const parent_data = data;
		const result = load
			? await with_request_store({ event, state }, () =>
				load.call(null, {
					url,
					params: req.params ?? {},
					data: server_data,
					route: { id: req.route_id ?? null },
					parent: async () => parent_data,
					depends: () => {},
					untrack: (fn) => fn(),
					fetch: create_universal_fetch(event, undefined, fetched, req.csr, resolve_options(serialized)),
					setHeaders: () => {
						throw new Error('skgo: universal load setHeaders is not supported during rendering');
					}
				})
			)
			: server_data;
		if (result != null && Object.getPrototypeOf(result) !== Object.prototype) {
			throw new Error('skgo: universal load must return a plain object');
		}
		data = { ...data, ...result };
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
 * Puts a non-enhanced submission's outcome where kit's form instance reads
 * it: the request's remote cache, under the instance's own internals object
 * and the empty-string key.
 *
 * That is the last thing kit's form wrapper does after it runs a submission
 * (runtime/app/server/remote/form.js: get_cache(__, state)[''] ??= output),
 * and it is what makes myForm.result, myForm.fields.x.issues() and the value
 * of every control render the submission. Nothing here runs a form body — Go
 * already ran it — so the stubs still throw and a rendered result is still
 * proof that Go answered.
 *
 * The output arrives in devalue's flat form and is read back with the app's
 * own decoders, for the same reason a load's data is: a result carrying a
 * transported type has to reach the component as an instance of its class.
 *
 * A keyed instance, one created by calling for(key) on a form, needs one more
 * step than an unkeyed one. Go's req.form_action.id is kit's own composite
 * id: the base hash/name, or that plus a slash and the key's JSON text — the
 * same string kit's server files a form's output under in the page's remote
 * data — and only the base half is registered anywhere: __skgo_forms holds
 * the module-level instance kit's form factory created, with no key at all.
 * That instance's own for method is kit's own code
 * (runtime/app/server/remote/form.js) for turning a key into the actual
 * per-key instance the page's own call to for(key) will return — it caches
 * what it creates in the request's form cache, keyed by the base id and the
 * key's JSON text together, so calling it here, before the page component
 * runs, makes the page's later call resolve to the very instance seeded below
 * rather than a fresh, empty one. Calling for reaches into the request
 * store, which is why this function now has to run inside with_request_store
 * rather than before it.
 */
function seed_form(req, state) {
	const seed = req.form_action;
	if (!seed) return;

	const first_slash = seed.id.indexOf('/');
	const second_slash = seed.id.indexOf('/', first_slash + 1);
	const base_id = second_slash === -1 ? seed.id : seed.id.slice(0, second_slash);
	const key_json = second_slash === -1 ? undefined : seed.id.slice(second_slash + 1);

	for (const instance of globalThis.__skgo_forms ?? []) {
		if (!instance.__ || instance.__.id !== base_id) continue;

		const target = key_json === undefined ? instance : instance.for(JSON.parse(key_json));
		(state.remote.data ??= new Map()).set(target.__, { '': parse(seed.output) });
		return;
	}

	throw new Error('skgo: no form is registered as ' + seed.id);
}

/**
 * Renders one page. The result object is filled in as the promise chain
 * settles; the host drains the job queue when this call returns, so done is
 * true by then or the render never finished — which is a Go error, not a
 * partial document.
 */
globalThis.__skgo_render = function (req_json) {
	const result = {
		done: false,
		// The message of a render that threw with nothing to catch it. Go turns
		// it into a Go error and answers the request some other way; it is never
		// part of a document.
		failure: '',
		// A redirect thrown during the render — by a remote function, say. Kit
		// answers the whole document with a bare 3xx (render_page's catch), and
		// so does Go.
		redirect: null,
		// The status and error the document is answered with. They start as the
		// ones Go asked for and are replaced by transformError if a boundary
		// catches something, which is where kit sets them too.
		status: 200,
		error: null,
		head: '',
		body: '',
		fetched: ''
	};

	try {
		const req = JSON.parse(req_json);
		const url = new URL(req.url);
		const state = make_state(req);
		const serialized = new Set();
		const event = make_event(req, url, serialized);
		const fetched = [];
		const props_promise = build_props(req, url, event, state, fetched, serialized);
		const promise = props_promise.then((props) => {
			result.status = props.page.status;
			result.error = req.error ?? null;
			const options = {
				context: new Map([['__request__', { page: props.page }]]),
				// kit's own \`csp.script_needs_nonce ? { nonce: csp.nonce } : {
				// hash: csp.script_needs_hash }\` (page/render.js:198), passed to
				// Svelte's own renderer so the one inline script Svelte can still
				// emit on its own — its hydratable-async-block script
				// (internal/server/renderer.js's #hydratable_block, for a
				// component's own top-level await) — carries the same nonce or
				// hash decision the boot script gets from Go (csp.go). Go decided
				// which branch this request is in before the engine ever ran;
				// req.csp carries only the answer.
				csp: req.csp.nonce ? { nonce: req.csp.nonce } : { hash: !!req.csp.hash },
				// kit's own (page/render.js): the transform every error boundary's
				// error passes through on its way to the failed snippet. It is
				// what makes page.status and page.error inside a rendering
				// component the values kit would give, and what turns an
				// unexpected throw into Internal Error rather than a stack trace on
				// the page. A redirect is rethrown, because a redirect is an answer
				// for the whole document rather than for one boundary.
				transformError: (e) => {
					if (e instanceof Redirect) throw e;
					const handled = handle_error(e);
					result.error = handled;
					result.status = handled.status;
					props.page.error = handled;
					props.page.status = handled.status;
					return handled;
				}
			};
			// seed_form runs inside the request store rather than before it: a
			// keyed submission's call to for(key) needs the request store, the
			// same as the page component's own call to it does.
			return with_request_store({ event, state }, () => {
				seed_form(req, state);
				return render(Root, { ...options, props });
			});
		});

		Promise.resolve(promise).then(
			(rendered) => {
				result.head = rendered.head;
				result.body = rendered.body;
				result.fetched = fetched
					.map((item) => serialize_data(item, resolve_options(serialized).filterSerializedResponseHeaders))
					.join("\n\t\t\t");
				result.done = true;
			},
			(err) => {
				if (err instanceof Redirect) {
					result.redirect = { status: err.status, location: err.location };
				} else if (err instanceof HandledHttpError) {
					result.handled_error = err.body;
				} else {
					result.failure = (err && (err.stack || err.message)) || String(err);
				}
				result.done = true;
			}
		);
	} catch (err) {
		if (err instanceof Redirect) {
			result.redirect = { status: err.status, location: err.location };
		} else if (err instanceof HandledHttpError) {
			result.handled_error = err.body;
		} else {
			result.failure = (err && (err.stack || err.message)) || String(err);
		}
		result.done = true;
	}

	return result;
};

// A liveness check Go calls once, when it puts a fresh runtime into the pool.
globalThis.__skgo_ping = function () {
	return 'ok';
};
