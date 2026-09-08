// The globals kit's runtime and Svelte's renderer reach that a bare
// ECMAScript engine does not have and Go does not bind.
//
// This file is the SSR bundle's banner rather than a module of it. The fold
// that makes the bundle evaluates kit's shared chunk before the entry, so a
// polyfill imported as a module would arrive after the code that needs it —
// which the single-pass build only got away with by module order.
//
// URL, URLSearchParams, TextEncoder, TextDecoder, btoa and atob are not here:
// Go binds those natively when it creates a runtime, before this text runs.
// What is left is the three classes kit compares against with `instanceof`,
// which a Go-backed constructor cannot satisfy.

// Svelte's no-AsyncLocalStorage fallback is gated on exactly this check
// (svelte/src/internal/server/render-context.js), and kit reads the same flag
// (kit/src/constants.js) to stop nulling its synchronous request store. It is
// Svelte's own supported path; the cost is one render per runtime at a time,
// which the pool in Go pays.
globalThis.process = { versions: { webcontainer: 'skgo' } };

// Headers is reached by kit's own Redirect constructor
// (exports/internal/shared.js), which builds one to reject a location that
// could not survive an HTTP header — and does it inside a try/catch, so an
// absent Headers would be reported as an invalid location rather than as a
// missing global. Only the construction and the validation it performs are
// needed; nothing in the engine sends a request.
if (typeof globalThis.Headers === 'undefined') {
	// RFC 9110 field-name and field-value rules, which is all the real
	// constructor checks that matters here.
	const NAME = /^[A-Za-z0-9!#$%&'*+.^_|~-]+$/;
	const BAD_VALUE = /[\u0000\r\n]/;

	globalThis.Headers = class Headers {
		constructor(init) {
			this._ = new Map();
			if (init instanceof Headers) {
				for (const [k, v] of init._) this._.set(k, v);
			} else if (Array.isArray(init)) {
				for (const [k, v] of init) this.set(k, v);
			} else if (init && typeof init === 'object') {
				for (const k of Object.keys(init)) this.set(k, init[k]);
			}
		}
		set(name, value) {
			const n = String(name);
			const v = String(value).trim();
			if (!NAME.test(n)) throw new TypeError('Invalid header name: ' + n);
			if (BAD_VALUE.test(v)) throw new TypeError('Invalid header value');
			this._.set(n.toLowerCase(), v);
		}
		append(name, value) {
			const existing = this.get(name);
			this.set(name, existing === null ? value : existing + ', ' + value);
		}
		get(name) {
			const k = String(name).toLowerCase();
			return this._.has(k) ? this._.get(k) : null;
		}
		has(name) {
			return this._.has(String(name).toLowerCase());
		}
		delete(name) {
			this._.delete(String(name).toLowerCase());
		}
		forEach(fn, thisArg) {
			for (const [k, v] of this._) fn.call(thisArg, v, k, this);
		}
		keys() {
			return this._.keys();
		}
		values() {
			return this._.values();
		}
		entries() {
			return this._.entries();
		}
		[Symbol.iterator]() {
			return this._.entries();
		}
	};
}

// Blob and File are named by kit's form-field proxy, which asks whether a
// field's value is a File before it decides how to describe it to the markup.
// Nothing here ever holds one — an uploaded file's bytes are Go's, and they
// never enter the engine — so these exist to be compared against.
if (typeof globalThis.Blob === 'undefined') {
	globalThis.Blob = class Blob {
		constructor(parts = [], options = {}) {
			this._parts = parts;
			this.type = options.type ?? '';
			this.size = 0;
		}
	};
}

if (typeof globalThis.File === 'undefined') {
	globalThis.File = class File extends globalThis.Blob {
		constructor(parts = [], name = '', options = {}) {
			super(parts, options);
			this.name = String(name);
			this.lastModified = options.lastModified ?? 0;
		}
	};
}
