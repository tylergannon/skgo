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
// What is left is what a Go-backed constructor could not stand in for: the
// classes kit compares against with `instanceof` (Headers, Blob, File) and the
// two a render's own `event.fetch` builds and hands back (Request, Response),
// which are string and Map bookkeeping over a value that never leaves the
// engine except through `__skgo_fetch`.

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
// missing global. It is also what a render's fetch response carries, so it
// follows the Fetch standard where kit's universal fetch can see it: names are
// lowercased and iterate sorted, a repeated name is joined with ", ", and
// Set-Cookie alone is kept as a list (getSetCookie) and iterates once per value.
if (typeof globalThis.Headers === 'undefined') {
	// RFC 9110 field-name and field-value rules, which is all the real
	// constructor checks that matters here.
	const NAME = /^[A-Za-z0-9!#$%&'*+.^_|~-]+$/;
	const BAD_VALUE = /[\u0000\r\n]/;

	globalThis.Headers = class Headers {
		constructor(init) {
			this._ = new Map();
			this._cookies = [];
			if (init instanceof Headers) {
				for (const [k, v] of init._) this._.set(k, v);
				this._cookies = init._cookies.slice();
			} else if (Array.isArray(init)) {
				for (const [k, v] of init) this.append(k, v);
			} else if (init && typeof init === 'object') {
				for (const k of Object.keys(init)) this.append(k, init[k]);
			}
		}
		set(name, value) {
			const n = String(name);
			const v = String(value).trim();
			if (!NAME.test(n)) throw new TypeError('Invalid header name: ' + n);
			if (BAD_VALUE.test(v)) throw new TypeError('Invalid header value');
			const k = n.toLowerCase();
			if (k === 'set-cookie') this._cookies = [v];
			else this._.set(k, v);
		}
		append(name, value) {
			const n = String(name);
			const v = String(value).trim();
			if (!NAME.test(n)) throw new TypeError('Invalid header name: ' + n);
			if (BAD_VALUE.test(v)) throw new TypeError('Invalid header value');
			const k = n.toLowerCase();
			if (k === 'set-cookie') this._cookies.push(v);
			else this._.set(k, this._.has(k) ? this._.get(k) + ', ' + v : v);
		}
		get(name) {
			const k = String(name).toLowerCase();
			if (k === 'set-cookie') return this._cookies.length ? this._cookies.join(', ') : null;
			return this._.has(k) ? this._.get(k) : null;
		}
		getSetCookie() {
			return this._cookies.slice();
		}
		has(name) {
			const k = String(name).toLowerCase();
			return k === 'set-cookie' ? this._cookies.length > 0 : this._.has(k);
		}
		delete(name) {
			const k = String(name).toLowerCase();
			if (k === 'set-cookie') this._cookies = [];
			else this._.delete(k);
		}
		forEach(fn, thisArg) {
			for (const [k, v] of this.entries()) fn.call(thisArg, v, k, this);
		}
		keys() {
			return Array.from(this.entries(), ([k]) => k)[Symbol.iterator]();
		}
		values() {
			return Array.from(this.entries(), ([, v]) => v)[Symbol.iterator]();
		}
		entries() {
			const all = Array.from(this._);
			for (const cookie of this._cookies) all.push(['set-cookie', cookie]);
			// A stable sort keeps the Set-Cookie values in the order they arrived.
			return all.sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0)[Symbol.iterator]();
		}
		[Symbol.iterator]() {
			return this.entries();
		}
	};
}

// ReadableStream is what a Response body is on the server too: kit's universal
// fetch tees `response.body` to record what a load read, and builds a Response
// over a stream to turn that copy into bytes. This is the part of the standard
// that path touches — a default reader, tee and cancel — over chunks that are
// already in memory, because Go hands a render a finished body and the engine
// holds no socket to stream from.
if (typeof globalThis.ReadableStream === 'undefined') {
	class ReadableStreamDefaultReader {
		constructor(stream) {
			if (stream._locked) throw new TypeError('ReadableStream is locked');
			stream._locked = true;
			this._stream = stream;
		}
		read() {
			return this._stream._read();
		}
		releaseLock() {
			this._stream._locked = false;
		}
		cancel(reason) {
			return this._stream.cancel(reason, true);
		}
	}

	globalThis.ReadableStream = class ReadableStream {
		constructor(source = {}) {
			this._source = source;
			this._queue = [];
			this._state = 'readable';
			this._error = undefined;
			this._waiters = [];
			this._locked = false;
			this._disturbed = false;
			const wake = () => this._waiters.splice(0).forEach((resolve) => resolve());
			this._controller = {
				enqueue: (chunk) => {
					if (this._state !== 'readable') throw new TypeError('The stream is not readable');
					this._queue.push(chunk);
					wake();
				},
				close: () => {
					if (this._state === 'readable') this._state = 'closed';
					wake();
				},
				error: (reason) => {
					if (this._state === 'readable') {
						this._state = 'errored';
						this._error = reason;
						this._queue = [];
					}
					wake();
				},
				desiredSize: 1
			};
			this._started = Promise.resolve().then(() => source.start?.(this._controller));
		}
		get locked() {
			return this._locked;
		}
		getReader() {
			return new ReadableStreamDefaultReader(this);
		}
		async _read() {
			this._disturbed = true;
			await this._started;
			for (;;) {
				if (this._queue.length) return { value: this._queue.shift(), done: false };
				if (this._state === 'closed') return { value: undefined, done: true };
				if (this._state === 'errored') throw this._error;
				if (this._source.pull) {
					await this._source.pull(this._controller);
					if (this._queue.length || this._state !== 'readable') continue;
				}
				await new Promise((resolve) => this._waiters.push(resolve));
			}
		}
		async cancel(reason, viaReader = false) {
			if (this._locked && !viaReader) throw new TypeError('ReadableStream is locked');
			this._disturbed = true;
			if (this._state === 'readable') {
				this._state = 'closed';
				this._queue = [];
				this._waiters.splice(0).forEach((resolve) => resolve());
				await this._source.cancel?.(reason);
			}
		}
		tee() {
			const reader = this.getReader();
			const branches = [];
			let chain = Promise.resolve();
			const pump = () => (chain = chain.then(async () => {
				const result = await reader.read().catch((error) => ({ error }));
				if (result.error !== undefined) branches.forEach((branch) => branch._controller.error(result.error));
				else if (result.done) branches.forEach((branch) => branch._controller.close());
				else branches.forEach((branch) => branch._state === 'readable' && branch._controller.enqueue(result.value));
			}));
			for (let i = 0; i < 2; i++) branches.push(new ReadableStream({ pull: pump }));
			return branches;
		}
		[Symbol.asyncIterator]() {
			const reader = this.getReader();
			return { next: () => reader.read(), [Symbol.asyncIterator]() { return this; } };
		}
	};
}

// What a body of any of the kinds `Request` and `Response` take comes to as
// bytes: text, an ArrayBuffer or a view over one, URLSearchParams (which is
// text), or a ReadableStream of those.
const body_bytes = async (body) => {
	if (body === null || body === undefined) return new Uint8Array(0);
	if (typeof body === 'string') return new TextEncoder().encode(body);
	if (typeof URLSearchParams !== 'undefined' && body instanceof URLSearchParams) {
		return new TextEncoder().encode(body.toString());
	}
	if (body instanceof ArrayBuffer) return new Uint8Array(body.slice(0));
	if (ArrayBuffer.isView(body)) {
		return new Uint8Array(body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength));
	}
	if (body instanceof ReadableStream) {
		const chunks = [];
		let length = 0;
		const reader = body.getReader();
		for (;;) {
			const { value, done } = await reader.read();
			if (done) break;
			const bytes = await body_bytes(value);
			chunks.push(bytes);
			length += bytes.length;
		}
		const out = new Uint8Array(length);
		let at = 0;
		for (const chunk of chunks) {
			out.set(chunk, at);
			at += chunk.length;
		}
		return out;
	}
	return new TextEncoder().encode(String(body));
};
globalThis.__skgo_body_bytes = body_bytes;

// The part of Request and Response that is the same on both: one body, readable
// once, as text, JSON, bytes or a stream.
const Body = class {
	get bodyUsed() {
		return this._used || (this._stream !== undefined && this._stream._disturbed);
	}
	get body() {
		if (this._source === null || this._source === undefined) return null;
		if (this._stream === undefined) {
			this._stream = this._source instanceof ReadableStream
				? this._source
				: new ReadableStream({
					start: async (controller) => {
						const bytes = await body_bytes(this._source);
						if (bytes.length) controller.enqueue(bytes);
						controller.close();
					}
				});
		}
		return this._stream;
	}
	async _consume() {
		if (this.bodyUsed) throw new TypeError('Body is unusable: Body has already been read');
		if (this._source === null || this._source === undefined) {
			this._used = true;
			return new Uint8Array(0);
		}
		if (this._stream === undefined && !(this._source instanceof ReadableStream)) {
			this._used = true;
			return body_bytes(this._source);
		}
		const stream = this.body;
		this._used = true;
		return body_bytes(stream);
	}
	async text() {
		return new TextDecoder().decode(await this._consume());
	}
	async json() {
		return JSON.parse(await this.text());
	}
	async arrayBuffer() {
		const bytes = await this._consume();
		return bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength);
	}
	_cloneSource() {
		if (this.bodyUsed) throw new TypeError('Body is unusable: Body has already been read');
		if (this._source === null || this._source === undefined) return null;
		if (this._source instanceof ReadableStream || this._stream !== undefined) {
			const [kept, copy] = this.body.tee();
			this._stream = kept;
			return copy;
		}
		return this._source;
	}
};

// Request and Response back a render's `event.fetch`. Kit's own
// `normalize_fetch_input` (runtime/server/fetch.js) turns whatever a component
// passed into a real Request before deciding what to do with it, and the
// answer a render's fetch gets back has to be a real Response — `await (await
// event.fetch(...)).json()` is what a page actually writes. Nothing here sends
// bytes anywhere: building one is bookkeeping, and the one call that leaves
// the engine is `__skgo_fetch` itself.
if (typeof globalThis.Request === 'undefined') {
	globalThis.Request = class Request extends Body {
		constructor(input, init = {}) {
			super();
			const from = input instanceof Request ? input : null;
			this.url = from ? from.url : String(input);
			this.method = String(init.method ?? from?.method ?? 'GET').toUpperCase();
			this.headers = new Headers(init.headers ?? from?.headers);
			this.credentials = init.credentials ?? from?.credentials ?? 'same-origin';
			this.mode = init.mode ?? from?.mode ?? 'cors';
			this.cache = init.cache ?? from?.cache ?? 'default';
			this.signal = init.signal ?? from?.signal ?? undefined;
			this._used = false;
			const own = init.body !== undefined && init.body !== null;
			this._source = own ? init.body : from ? from._cloneSource() : null;
			if (this._source !== null && (this.method === 'GET' || this.method === 'HEAD')) {
				throw new TypeError('Request with GET/HEAD method cannot have body.');
			}
			if (own && !this.headers.has('content-type')) {
				if (typeof init.body === 'string') {
					this.headers.set('content-type', 'text/plain;charset=UTF-8');
				} else if (typeof URLSearchParams !== 'undefined' && init.body instanceof URLSearchParams) {
					this.headers.set('content-type', 'application/x-www-form-urlencoded;charset=UTF-8');
				}
			}
		}
		clone() {
			return new Request(this);
		}
	};
}

if (typeof globalThis.Response === 'undefined') {
	globalThis.Response = class Response extends Body {
		constructor(body, init = {}) {
			super();
			this._source = body ?? null;
			this._used = false;
			this.status = init.status ?? 200;
			this.statusText = init.statusText ?? '';
			this.headers = new Headers(init.headers);
			this.ok = this.status >= 200 && this.status < 300;
			this.url = '';
			this.redirected = false;
			this.type = 'default';
		}
		clone() {
			return new Response(this._cloneSource(), {
				status: this.status,
				statusText: this.statusText,
				headers: this.headers
			});
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
