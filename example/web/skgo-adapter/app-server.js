// The `$app/server` the SSR bundle sees.
//
// It is a module of the `goja` environment and of no other: the adapter's
// plugin resolves `$app/server` to this file only while that environment is
// building, so kit's `ssr` build — the one that prerenders — keeps kit's real
// module. Substituting at source level is the whole reason the bundle is an
// environment of kit's build rather than a pass over its output: once kit's
// graph is linked, the boundary this file replaces no longer exists.
//
// Kit's own `query`/`command`/`form` wrappers are kept: the argument-keyed
// cache, `state.remote.implicit`, the `<id>/<payload>` key, the rule that an
// argument needs a validator, the refusal to run a command during a render and
// the derived event whose `url`, `params` and `route` throw inside a query are
// all kit's, unchanged. The only substitution is the user function body —
// the generated stub that throws — which becomes a call into Go.
//
// That inversion is the proof: the generated `.remote.ts` still throws, and the
// host binding is the only path by which a value can reach the engine.
//
// Everything this file does not name is kit's, by `export *`: `read`,
// `getRequestEvent`, `prerender` and `requested` come from kit's own module,
// and an export kit adds arrives without an edit here. An explicit export
// below shadows the star, which is how the four substitutions take effect.
import * as real from 'skgo:kit/remote';
import { stringify_remote_arg } from 'skgo:kit/shared';
import { HttpError, Redirect } from '@sveltejs/kit/internal/server';
import { parse } from 'skgo:kit/transport';

export * from 'skgo:kit/app-server';

/**
 * Calls Go. Synchronous: Go has the answer in this process, so there is nothing
 * for an event loop to wait on.
 *
 * A refusal comes back as one of kit's own control objects rather than a bare
 * Error, because everything downstream classifies by type: handle_error
 * keeps an HttpError's body and replaces anything else with Internal Error,
 * and transformError rethrows a Redirect so that the whole document becomes
 * the 3xx kit answers with.
 *
 * The answer is devalue's flat form, the same bytes Go would have sent the
 * browser from /_app/remote/..., and it is read back with kit's own parse —
 * the app's transport decoders. So a query answering with a custom type hands
 * the component an instance of the app's class, and a method call on it during
 * a render works for the same reason it works in the browser.
 */
function host(id, payload) {
	const raw = globalThis.__skgo_remote(id, payload);
	const res = JSON.parse(raw);
	if (res.r) {
		throw new Redirect(res.r.status, res.r.location);
	}
	if (res.e) {
		throw new HttpError({ status: res.e.status ?? 500, message: res.e.message });
	}
	return parse(res.v);
}

export function query(validate_or_fn, maybe_fn) {
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.query(validate_or_fn, fn) : real.query(fn);
	return wrapper;
}

/**
 * Calls Go once for a whole batch. Kit's own enqueue has already collected
 * every call made to this function in one macrotask and deduplicated them by
 * payload, so args is the batch — and the app's Go function is invoked once
 * for all of it, which is the only thing that makes a batch query different
 * from a query.
 *
 * The payloads travel as a JSON array in the place a single payload would
 * occupy, and the answer is {n: [...]} — one envelope per payload, in the
 * order they were sent, which is the order kit's client matches results to
 * arguments by.
 */
function host_batch(id, args) {
	const payloads = args.map((arg) => stringify_remote_arg(arg));
	const raw = globalThis.__skgo_remote(id, JSON.stringify(payloads));
	const res = JSON.parse(raw);
	if (res.r) {
		throw new Redirect(res.r.status, res.r.location);
	}
	if (res.e) {
		throw new HttpError({ status: res.e.status ?? 500, message: res.e.message });
	}
	const nodes = res.n ?? [];
	return (_arg, i) => {
		const node = nodes[i];
		if (!node) {
			throw new HttpError({ status: 500, message: 'Internal Error' });
		}
		if (node.e) {
			throw new HttpError({ status: node.e.status ?? 500, message: node.e.message });
		}
		return parse(node.v);
	};
}

query.batch = (validate_or_fn, maybe_fn) => {
	const fn = (args) => host_batch(wrapper.__.id, args);
	const wrapper = maybe_fn ? real.query.batch(validate_or_fn, fn) : real.query.batch(fn);
	return wrapper;
};

query.live = (validate_or_fn, maybe_fn) => {
	// A live query awaited during a render resolves to its first value, which is
	// what kit's get_first_value takes from the generator before closing it.
	// Go drives the producer far enough to yield once and answers with that, so
	// the value is in the markup before any script runs; the stream itself is
	// the browser's, and it opens after hydration against the same endpoint.
	//
	// A plain iterator rather than a generator: to_iterator accepts anything
	// with a next method, and there is nothing to suspend on — Go has the
	// value in this process.
	const fn = (arg) => {
		const payload = stringify_remote_arg(arg);
		let taken = false;
		return {
			next: () => {
				if (taken) return { value: undefined, done: true };
				taken = true;
				return { value: host(wrapper.__.id, payload), done: false };
			},
			return: () => ({ value: undefined, done: true })
		};
	};
	const wrapper = maybe_fn ? real.query.live(validate_or_fn, fn) : real.query.live(fn);
	return wrapper;
};

export function command(validate_or_fn, maybe_fn) {
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.command(validate_or_fn, fn) : real.command(fn);
	return wrapper;
}

/**
 * Kit's own form wrapper, with the instance kept so that a submission the
 * browser posted without JavaScript can be put back where the instance reads
 * it.
 *
 * The generated stub still throws — Go runs the handler, not this module — so
 * the register below never runs a form body. It only records which object is
 * which, because the internals' id is assigned by the epilogue the bundler
 * appends after this module has been evaluated, and that id is the only handle
 * Go has.
 */
export function form(validate_or_fn, maybe_fn) {
	const wrapper = maybe_fn === undefined ? real.form(validate_or_fn) : real.form(validate_or_fn, maybe_fn);
	(globalThis.__skgo_forms ??= []).push(wrapper);
	return wrapper;
}
