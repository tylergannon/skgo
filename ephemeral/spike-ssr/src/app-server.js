// SPIKE. The `$app/server` the SSR bundle sees.
//
// Kit's own `query`/`command`/`form` wrappers are kept — the argument-keyed
// cache, `state.remote.implicit`, the `<id>/<payload>` key, the "no arguments
// without a validator" rule and the resource shape are all kit's. The only
// substitution is the user function body: instead of the generated stub that
// throws, it calls back into Go.
//
// Chosen over rewriting the generated `.remote.ts` bodies because the wrappers
// (and therefore the hydration keys the browser will look for) stay kit's.
import * as real from 'skgo:kit/remote';
import { stringify_remote_arg } from 'skgo:kit/shared';

export { getRequestEvent } from '@sveltejs/kit/internal/server';

/**
 * Calls the Go host. Synchronous: Go has the answer in-process, so there is
 * nothing for an event loop to wait on.
 * @param {string} id  `<hash>/<name>`, as kit's vite plugin assigns it
 * @param {string} payload  `stringify_remote_arg(arg)`
 */
function host(id, payload) {
	const raw = globalThis.__skgo_remote(id, payload);
	const res = JSON.parse(raw);
	if (res.e) {
		const err = new Error(res.e.message);
		// @ts-ignore
		err.status = res.e.status ?? 500;
		throw err;
	}
	return res.v;
}

export function query(validate_or_fn, maybe_fn) {
	/** @param {any} arg the validated argument */
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.query(validate_or_fn, fn) : real.query(fn);
	return wrapper;
}

query.batch = (validate_or_fn, maybe_fn) => {
	const fn = (args) => {
		const results = args.map((arg) => host(wrapper.__.id, stringify_remote_arg(arg)));
		return (_arg, i) => results[i];
	};
	const wrapper = maybe_fn ? real.query.batch(validate_or_fn, fn) : real.query.batch(fn);
	return wrapper;
};

query.live = (validate_or_fn, maybe_fn) => {
	// Not part of the spike: a live query is a stream, and nothing here drives
	// one. Kept so `query.live(...)` at module scope does not explode; awaiting
	// it during render is what would fail.
	const fn = function* () {
		throw new Error('skgo spike: query.live is not implemented in-engine');
	};
	return maybe_fn ? real.query.live(validate_or_fn, fn) : real.query.live(fn);
};

export function command(validate_or_fn, maybe_fn) {
	const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
	const wrapper = maybe_fn ? real.command(validate_or_fn, fn) : real.command(fn);
	return wrapper;
}

export const form = real.form;
export const prerender = real.prerender;
export const requested = real.requested;
