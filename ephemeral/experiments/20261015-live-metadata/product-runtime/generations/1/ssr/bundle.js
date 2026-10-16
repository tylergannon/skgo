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
globalThis.process = { versions: { webcontainer: "skgo" } };
// Headers is reached by kit's own Redirect constructor
// (exports/internal/shared.js), which builds one to reject a location that
// could not survive an HTTP header — and does it inside a try/catch, so an
// absent Headers would be reported as an invalid location rather than as a
// missing global. It is also what a render's fetch response carries, so it
// follows the Fetch standard where kit's universal fetch can see it: names are
// lowercased and iterate sorted, a repeated name is joined with ", ", and
// Set-Cookie alone is kept as a list (getSetCookie) and iterates once per value.
if (typeof globalThis.Headers === "undefined") {
	// RFC 9110 field-name and field-value rules, which is all the real
	// constructor checks that matters here.
	const NAME = /^[A-Za-z0-9!#$%&'*+.^_|~-]+$/;
	const BAD_VALUE = /[\u0000\r\n]/;
	globalThis.Headers = class Headers {
		constructor(init) {
			this._ = /* @__PURE__ */ new Map();
			this._cookies = [];
			if (init instanceof Headers) {
				for (const [k, v] of init._) this._.set(k, v);
				this._cookies = init._cookies.slice();
			} else if (Array.isArray(init)) for (const [k, v] of init) this.append(k, v);
			else if (init && typeof init === "object") for (const k of Object.keys(init)) this.append(k, init[k]);
		}
		set(name, value) {
			const n = String(name);
			const v = String(value).trim();
			if (!NAME.test(n)) throw new TypeError("Invalid header name: " + n);
			if (BAD_VALUE.test(v)) throw new TypeError("Invalid header value");
			const k = n.toLowerCase();
			if (k === "set-cookie") this._cookies = [v];
			else this._.set(k, v);
		}
		append(name, value) {
			const n = String(name);
			const v = String(value).trim();
			if (!NAME.test(n)) throw new TypeError("Invalid header name: " + n);
			if (BAD_VALUE.test(v)) throw new TypeError("Invalid header value");
			const k = n.toLowerCase();
			if (k === "set-cookie") this._cookies.push(v);
			else this._.set(k, this._.has(k) ? this._.get(k) + ", " + v : v);
		}
		get(name) {
			const k = String(name).toLowerCase();
			if (k === "set-cookie") return this._cookies.length ? this._cookies.join(", ") : null;
			return this._.has(k) ? this._.get(k) : null;
		}
		getSetCookie() {
			return this._cookies.slice();
		}
		has(name) {
			const k = String(name).toLowerCase();
			return k === "set-cookie" ? this._cookies.length > 0 : this._.has(k);
		}
		delete(name) {
			const k = String(name).toLowerCase();
			if (k === "set-cookie") this._cookies = [];
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
			for (const cookie of this._cookies) all.push(["set-cookie", cookie]);
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
if (typeof globalThis.ReadableStream === "undefined") {
	class ReadableStreamDefaultReader {
		constructor(stream) {
			if (stream._locked) throw new TypeError("ReadableStream is locked");
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
			this._state = "readable";
			this._error = void 0;
			this._waiters = [];
			this._locked = false;
			this._disturbed = false;
			const wake = () => this._waiters.splice(0).forEach((resolve) => resolve());
			this._controller = {
				enqueue: (chunk) => {
					if (this._state !== "readable") throw new TypeError("The stream is not readable");
					this._queue.push(chunk);
					wake();
				},
				close: () => {
					if (this._state === "readable") this._state = "closed";
					wake();
				},
				error: (reason) => {
					if (this._state === "readable") {
						this._state = "errored";
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
				if (this._queue.length) return {
					value: this._queue.shift(),
					done: false
				};
				if (this._state === "closed") return {
					value: void 0,
					done: true
				};
				if (this._state === "errored") throw this._error;
				if (this._source.pull) {
					await this._source.pull(this._controller);
					if (this._queue.length || this._state !== "readable") continue;
				}
				await new Promise((resolve) => this._waiters.push(resolve));
			}
		}
		async cancel(reason, viaReader = false) {
			if (this._locked && !viaReader) throw new TypeError("ReadableStream is locked");
			this._disturbed = true;
			if (this._state === "readable") {
				this._state = "closed";
				this._queue = [];
				this._waiters.splice(0).forEach((resolve) => resolve());
				await this._source.cancel?.(reason);
			}
		}
		tee() {
			const reader = this.getReader();
			const branches = [];
			let chain = Promise.resolve();
			const pump = () => chain = chain.then(async () => {
				const result = await reader.read().catch((error) => ({ error }));
				if (result.error !== void 0) branches.forEach((branch) => branch._controller.error(result.error));
				else if (result.done) branches.forEach((branch) => branch._controller.close());
				else branches.forEach((branch) => branch._state === "readable" && branch._controller.enqueue(result.value));
			});
			for (let i = 0; i < 2; i++) branches.push(new ReadableStream({ pull: pump }));
			return branches;
		}
		[Symbol.asyncIterator]() {
			const reader = this.getReader();
			return {
				next: () => reader.read(),
				[Symbol.asyncIterator]() {
					return this;
				}
			};
		}
	};
}
// What a body of any of the kinds `Request` and `Response` take comes to as
// bytes: text, an ArrayBuffer or a view over one, URLSearchParams (which is
// text), or a ReadableStream of those.
const body_bytes = async (body) => {
	if (body === null || body === void 0) return /* @__PURE__ */ new Uint8Array(0);
	if (typeof body === "string") return new TextEncoder().encode(body);
	if (typeof URLSearchParams !== "undefined" && body instanceof URLSearchParams) return new TextEncoder().encode(body.toString());
	if (body instanceof ArrayBuffer) return new Uint8Array(body.slice(0));
	if (ArrayBuffer.isView(body)) return new Uint8Array(body.buffer.slice(body.byteOffset, body.byteOffset + body.byteLength));
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
		return this._used || this._stream !== void 0 && this._stream._disturbed;
	}
	get body() {
		if (this._source === null || this._source === void 0) return null;
		if (this._stream === void 0) this._stream = this._source instanceof ReadableStream ? this._source : new ReadableStream({ start: async (controller) => {
			const bytes = await body_bytes(this._source);
			if (bytes.length) controller.enqueue(bytes);
			controller.close();
		} });
		return this._stream;
	}
	async _consume() {
		if (this.bodyUsed) throw new TypeError("Body is unusable: Body has already been read");
		if (this._source === null || this._source === void 0) {
			this._used = true;
			return /* @__PURE__ */ new Uint8Array(0);
		}
		if (this._stream === void 0 && !(this._source instanceof ReadableStream)) {
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
		if (this.bodyUsed) throw new TypeError("Body is unusable: Body has already been read");
		if (this._source === null || this._source === void 0) return null;
		if (this._source instanceof ReadableStream || this._stream !== void 0) {
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
if (typeof globalThis.Request === "undefined") globalThis.Request = class Request extends Body {
	constructor(input, init = {}) {
		super();
		const from = input instanceof Request ? input : null;
		this.url = from ? from.url : String(input);
		this.method = String(init.method ?? from?.method ?? "GET").toUpperCase();
		this.headers = new Headers(init.headers ?? from?.headers);
		this.credentials = init.credentials ?? from?.credentials ?? "same-origin";
		this.mode = init.mode ?? from?.mode ?? "cors";
		this.cache = init.cache ?? from?.cache ?? "default";
		this.signal = init.signal ?? from?.signal ?? void 0;
		this._used = false;
		const own = init.body !== void 0 && init.body !== null;
		this._source = own ? init.body : from ? from._cloneSource() : null;
		if (this._source !== null && (this.method === "GET" || this.method === "HEAD")) throw new TypeError("Request with GET/HEAD method cannot have body.");
		if (own && !this.headers.has("content-type")) {
			if (typeof init.body === "string") this.headers.set("content-type", "text/plain;charset=UTF-8");
			else if (typeof URLSearchParams !== "undefined" && init.body instanceof URLSearchParams) this.headers.set("content-type", "application/x-www-form-urlencoded;charset=UTF-8");
		}
	}
	clone() {
		return new Request(this);
	}
};
if (typeof globalThis.Response === "undefined") globalThis.Response = class Response extends Body {
	constructor(body, init = {}) {
		super();
		this._source = body ?? null;
		this._used = false;
		this.status = init.status ?? 200;
		this.statusText = init.statusText ?? "";
		this.headers = new Headers(init.headers);
		this.ok = this.status >= 200 && this.status < 300;
		this.url = "";
		this.redirected = false;
		this.type = "default";
	}
	clone() {
		return new Response(this._cloneSource(), {
			status: this.status,
			statusText: this.statusText,
			headers: this.headers
		});
	}
};
// Blob and File are named by kit's form-field proxy, which asks whether a
// field's value is a File before it decides how to describe it to the markup.
// Nothing here ever holds one — an uploaded file's bytes are Go's, and they
// never enter the engine — so these exist to be compared against.
if (typeof globalThis.Blob === "undefined") globalThis.Blob = class Blob {
	constructor(parts = [], options = {}) {
		this._parts = parts;
		this.type = options.type ?? "";
		this.size = 0;
	}
};
if (typeof globalThis.File === "undefined") globalThis.File = class File extends globalThis.Blob {
	constructor(parts = [], name = "", options = {}) {
		super(parts, options);
		this.name = String(name);
		this.lastModified = options.lastModified ?? 0;
	}
};
(function() {
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/app-server-CGYkqIcl.js
	var __create = Object.create;
	var __defProp = Object.defineProperty;
	var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
	var __getOwnPropNames = Object.getOwnPropertyNames;
	var __getProtoOf = Object.getPrototypeOf;
	var __hasOwnProp = Object.prototype.hasOwnProperty;
	var __esmMin = (fn, res, err) => () => {
		if (err) throw err[0];
		try {
			return fn && (res = fn(fn = 0)), res;
		} catch (e) {
			throw err = [e], e;
		}
	};
	var __commonJSMin = (cb, mod) => () => (mod || (cb((mod = { exports: {} }).exports, mod), cb = null), mod.exports);
	var __exportAll = (all, no_symbols) => {
		let target = {};
		for (var name in all) __defProp(target, name, {
			get: all[name],
			enumerable: true
		});
		if (!no_symbols) __defProp(target, Symbol.toStringTag, { value: "Module" });
		return target;
	};
	var __copyProps = (to, from, except, desc) => {
		if (from && typeof from === "object" || typeof from === "function") for (var keys = __getOwnPropNames(from), i = 0, n = keys.length, key; i < n; i++) {
			key = keys[i];
			if (!__hasOwnProp.call(to, key) && key !== except) __defProp(to, key, {
				get: ((k) => from[k]).bind(null, key),
				enumerable: !(desc = __getOwnPropDesc(from, key)) || desc.enumerable
			});
		}
		return to;
	};
	var __toESM = (mod, isNodeMode, target) => (target = mod != null ? __create(__getProtoOf(mod)) : {}, __copyProps(isNodeMode || !mod || !mod.__esModule || !__hasOwnProp.call(mod, "default") ? __defProp(target, "default", {
		value: mod,
		enumerable: true
	}) : target, mod));
	function _OverloadYield(e, d) {
		this.v = e, this.k = d;
	}
	function _awaitAsyncGenerator(e) {
		return new _OverloadYield(e, 0);
	}
	function _wrapAsyncGenerator(e) {
		return function() {
			return new AsyncGenerator(e.apply(this, arguments));
		};
	}
	function AsyncGenerator(e) {
		var r, t;
		function resume(r, t) {
			try {
				var n = e[r](t), o = n.value, u = o instanceof _OverloadYield;
				Promise.resolve(u ? o.v : o).then(function(t) {
					if (u) {
						var i = "return" === r ? "return" : "next";
						if (!o.k || t.done) return resume(i, t);
						t = e[i](t).value;
					}
					settle(n.done ? "return" : "normal", t);
				}, function(e) {
					resume("throw", e);
				});
			} catch (e) {
				settle("throw", e);
			}
		}
		function settle(e, n) {
			switch (e) {
				case "return":
					r.resolve({
						value: n,
						done: !0
					});
					break;
				case "throw":
					r.reject(n);
					break;
				default: r.resolve({
					value: n,
					done: !1
				});
			}
			(r = r.next) ? resume(r.key, r.arg) : t = null;
		}
		this._invoke = function(e, n) {
			return new Promise(function(o, u) {
				var i = {
					key: e,
					arg: n,
					resolve: o,
					reject: u,
					next: null
				};
				t ? t = t.next = i : (r = t = i, resume(e, n));
			});
		}, "function" != typeof e["return"] && (this["return"] = void 0);
	}
	AsyncGenerator.prototype["function" == typeof Symbol && Symbol.asyncIterator || "@@asyncIterator"] = function() {
		return this;
	}, AsyncGenerator.prototype.next = function(e) {
		return this._invoke("next", e);
	}, AsyncGenerator.prototype["throw"] = function(e) {
		return this._invoke("throw", e);
	}, AsyncGenerator.prototype["return"] = function(e) {
		return this._invoke("return", e);
	};
	var require__skgo_skgo_missing = /* @__PURE__ */ __commonJSMin((() => {
		throw new Error("skgo: node:async_hooks is unavailable in the SSR engine");
	}));
	var MAX_ARRAY_LEN = 2 ** 32 - 1;
	var MAX_ARRAY_INDEX = MAX_ARRAY_LEN - 1;
	/** @type {Record<string, string>} */
	var escaped = {
		"<": "\\u003C",
		"\\": "\\\\",
		"\b": "\\b",
		"\f": "\\f",
		"\n": "\\n",
		"\r": "\\r",
		"	": "\\t",
		"\u2028": "\\u2028",
		"\u2029": "\\u2029"
	};
	var DevalueError = class extends Error {
		/**
		* @param {string} message
		* @param {string[]} keys
		* @param {any} [value] - The value that failed to be serialized
		* @param {any} [root] - The root value being serialized
		*/
		constructor(message, keys, value, root) {
			super(message);
			this.name = "DevalueError";
			this.path = keys.join("");
			this.value = value;
			this.root = root;
		}
	};
	/** @param {any} thing */
	function is_primitive(thing) {
		return thing === null || typeof thing !== "object" && typeof thing !== "function";
	}
	var object_proto_names$1 = /* @__PURE__ */ Object.getOwnPropertyNames(Object.prototype).sort().join("\0");
	/** @param {any} thing */
	function is_plain_object$1(thing) {
		const proto = Object.getPrototypeOf(thing);
		return proto === Object.prototype || proto === null || Object.getPrototypeOf(proto) === null || Object.getOwnPropertyNames(proto).sort().join("\0") === object_proto_names$1;
	}
	/** @param {any} thing */
	function get_type(thing) {
		return Object.prototype.toString.call(thing).slice(8, -1);
	}
	/** @param {any} thing */
	function is_buffer(thing) {
		return typeof Buffer !== "undefined" && Buffer.isBuffer(thing);
	}
	/** @param {string} char */
	function get_escaped_char(char) {
		switch (char) {
			case "\"": return "\\\"";
			case "<": return "\\u003C";
			case "\\": return "\\\\";
			case "\n": return "\\n";
			case "\r": return "\\r";
			case "	": return "\\t";
			case "\b": return "\\b";
			case "\f": return "\\f";
			case "\u2028": return "\\u2028";
			case "\u2029": return "\\u2029";
			default: return char < " " ? `\\u${char.charCodeAt(0).toString(16).padStart(4, "0")}` : "";
		}
	}
	/** @param {string} str */
	function stringify_string(str) {
		let result = "";
		let last_pos = 0;
		const len = str.length;
		for (let i = 0; i < len; i += 1) {
			const char = str[i];
			const replacement = get_escaped_char(char);
			if (replacement) {
				result += str.slice(last_pos, i) + replacement;
				last_pos = i + 1;
			}
		}
		return `"${last_pos === 0 ? str : result + str.slice(last_pos)}"`;
	}
	/** @param {Record<string | symbol, any>} object */
	function enumerable_symbols(object) {
		return Object.getOwnPropertySymbols(object).filter((symbol) => Object.getOwnPropertyDescriptor(object, symbol).enumerable);
	}
	var is_identifier = /^[a-zA-Z_$][a-zA-Z_$0-9]*$/;
	/** @param {string} key */
	function stringify_key(key) {
		return is_identifier.test(key) ? "." + key : "[" + JSON.stringify(key) + "]";
	}
	/** @param {number} n */
	function is_valid_array_index(n) {
		if (!Number.isInteger(n)) return false;
		if (n < 0) return false;
		if (n > MAX_ARRAY_INDEX) return false;
		return true;
	}
	/** @param {number} n */
	function is_valid_array_len(n) {
		if (!Number.isInteger(n)) return false;
		if (n < 0) return false;
		if (n > MAX_ARRAY_LEN) return false;
		return true;
	}
	/** @param {string} s */
	function is_valid_array_index_string(s) {
		if (s.length === 0) return false;
		if (s.length > 1 && s.charCodeAt(0) === 48) return false;
		for (let i = 0; i < s.length; i++) {
			const c = s.charCodeAt(i);
			if (c < 48 || c > 57) return false;
		}
		return is_valid_array_index(+s);
	}
	/**
	* Returns the length of the leading run of valid array indices in `keys`.
	* @param {readonly string[]} keys
	*/
	function array_index_cut(keys) {
		for (var i = keys.length - 1; i >= 0; i--) if (is_valid_array_index_string(keys[i])) break;
		return i + 1;
	}
	/**
	* Finds the populated indices of an array.
	* @param {unknown[]} array
	*/
	function valid_array_indices(array) {
		const keys = Object.keys(array);
		keys.length = array_index_cut(keys);
		return keys;
	}
	var chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_$";
	var MIN_STRING_LENGTH = 128;
	var unsafe_chars = /[<\b\f\n\r\t\0\u2028\u2029]/g;
	var reserved = /^(?:do|if|in|for|int|let|new|try|var|byte|case|char|else|enum|goto|long|this|void|with|await|break|catch|class|const|final|float|short|super|throw|while|yield|delete|double|export|import|native|return|switch|throws|typeof|boolean|default|extends|finally|package|private|abstract|continue|debugger|function|volatile|interface|protected|transient|implements|instanceof|synchronized)$/;
	/**
	* Turn a value into the JavaScript that creates an equivalent value
	* @param {any} value
	* @param {(value: any, uneval: (value: any) => string) => string | void} [replacer]
	*/
	function uneval$1(value, replacer) {
		const counts = /* @__PURE__ */ new Map();
		/** @type {string[]} */
		const keys = [];
		const custom = /* @__PURE__ */ new Map();
		/** @type {Map<string | bigint, string> | undefined} */
		let primitives;
		/** @param {any} thing */
		function stringify_cached_primitive(thing) {
			if (typeof thing === "string" && thing.length >= MIN_STRING_LENGTH || typeof thing === "bigint") {
				primitives ??= /* @__PURE__ */ new Map();
				let literal = primitives.get(thing);
				if (literal === void 0) {
					literal = stringify_primitive$1(thing);
					primitives.set(thing, literal);
				}
				return literal;
			}
			return stringify_primitive$1(thing);
		}
		/** @param {any} thing */
		function walk(thing) {
			if (!is_primitive(thing)) {
				if (counts.has(thing)) {
					counts.set(thing, counts.get(thing) + 1);
					return;
				}
				counts.set(thing, 1);
				if (replacer) {
					const str = replacer(thing, (value) => uneval$1(value, replacer));
					if (typeof str === "string") {
						custom.set(thing, str);
						return;
					}
				}
				if (typeof thing === "function") throw new DevalueError(`Cannot stringify a function`, keys, thing, value);
				switch (get_type(thing)) {
					case "BigInt":
					case "String":
						walk(thing.valueOf());
						return;
					case "Number":
					case "Boolean":
					case "Date":
					case "RegExp":
					case "URL":
					case "URLSearchParams": return;
					case "Array":
						for (const i of valid_array_indices(thing)) {
							keys.push(`[${i}]`);
							walk(thing[i]);
							keys.pop();
						}
						break;
					case "Set":
						Array.from(thing).forEach(walk);
						break;
					case "Map":
						for (const [key, value] of thing) {
							keys.push(`.get(${is_primitive(key) ? stringify_cached_primitive(key) : "..."})`);
							walk(key);
							walk(value);
							keys.pop();
						}
						break;
					case "Int8Array":
					case "Uint8Array":
					case "Uint8ClampedArray":
					case "Int16Array":
					case "Uint16Array":
					case "Float16Array":
					case "Int32Array":
					case "Uint32Array":
					case "Float32Array":
					case "Float64Array":
					case "BigInt64Array":
					case "BigUint64Array":
					case "DataView":
						if (!is_buffer(thing)) walk(thing.buffer);
						return;
					case "ArrayBuffer": return;
					case "Temporal.Duration":
					case "Temporal.Instant":
					case "Temporal.PlainDate":
					case "Temporal.PlainTime":
					case "Temporal.PlainDateTime":
					case "Temporal.PlainMonthDay":
					case "Temporal.PlainYearMonth":
					case "Temporal.ZonedDateTime": return;
					default:
						if (!is_plain_object$1(thing)) throw new DevalueError(`Cannot stringify arbitrary non-POJOs`, keys, thing, value);
						if (enumerable_symbols(thing).length > 0) throw new DevalueError(`Cannot stringify POJOs with symbolic keys`, keys, thing, value);
						for (const key of Object.keys(thing)) {
							if (key === "__proto__") throw new DevalueError(`Cannot stringify objects with __proto__ keys`, keys, thing, value);
							keys.push(stringify_key(key));
							walk(thing[key]);
							keys.pop();
						}
				}
			} else if (typeof thing === "symbol") throw new DevalueError(`Cannot stringify a Symbol primitive`, keys, thing, value);
			else if (typeof thing === "string" && thing.length >= MIN_STRING_LENGTH || typeof thing === "bigint") counts.set(thing, (counts.get(thing) || 0) + 1);
		}
		walk(value);
		const names = /* @__PURE__ */ new Map();
		/** @type {Array<[any, number]>} */
		const repeated = [];
		counts.forEach((count, thing) => {
			if (count > 1) repeated.push([thing, count]);
		});
		repeated.sort((a, b) => b[1] - a[1]).forEach(([thing, count]) => {
			const name = get_name(names.size);
			if (is_primitive(thing)) {
				const length = stringify_cached_primitive(thing).length;
				if (length * count <= length + (count + 1) * name.length + 25) return;
			}
			names.set(thing, name);
		});
		/**
		* @param {any} thing
		* @returns {string}
		*/
		function stringify(thing) {
			if (names.has(thing)) return names.get(thing);
			if (is_primitive(thing)) return stringify_primitive$1(thing);
			if (custom.has(thing)) return custom.get(thing);
			const type = get_type(thing);
			switch (type) {
				case "Number":
				case "String":
				case "Boolean":
				case "BigInt": return `Object(${stringify(thing.valueOf())})`;
				case "RegExp":
					const { source, flags } = thing;
					return flags ? `new RegExp(${stringify_string(source)},"${flags}")` : `new RegExp(${stringify_string(source)})`;
				case "Date": return `new Date(${thing.getTime()})`;
				case "URL": return `new URL(${stringify_string(thing.toString())})`;
				case "URLSearchParams": return `new URLSearchParams(${stringify_string(thing.toString())})`;
				case "Array": {
					let has_holes = false;
					let result = "[";
					for (let i = 0; i < thing.length; i += 1) {
						if (i > 0) result += ",";
						if (Object.hasOwn(thing, i)) result += stringify(thing[i]);
						else if (!has_holes) {
							const populated_keys = valid_array_indices(thing);
							const population = populated_keys.length;
							const d = String(thing.length).length;
							const array = stringify_sparse_array(thing.length);
							if (thing.length + 2 > array.length + 18 + population * (d + 2)) return `Object.assign(${array},{${populated_keys.map((k) => `${k}:${stringify(thing[k])}`).join(",")}})`;
							has_holes = true;
						}
					}
					const tail = thing.length === 0 || Object.hasOwn(thing, thing.length - 1) ? "" : ",";
					return result + tail + "]";
				}
				case "Set":
				case "Map": return `new ${type}([${Array.from(thing).map(stringify).join(",")}])`;
				case "Int8Array":
				case "Uint8Array":
				case "Uint8ClampedArray":
				case "Int16Array":
				case "Uint16Array":
				case "Float16Array":
				case "Int32Array":
				case "Uint32Array":
				case "Float32Array":
				case "Float64Array":
				case "BigInt64Array":
				case "BigUint64Array": {
					if (is_buffer(thing)) thing = new Uint8Array(thing);
					let str = `new ${type}`;
					if (!names.has(thing.buffer)) str += `([${stringify_typed_array_elements(type, thing.buffer)}])`;
					else str += `(${stringify(thing.buffer)})`;
					if (thing.byteLength !== thing.buffer.byteLength) {
						const start = thing.byteOffset / thing.BYTES_PER_ELEMENT;
						const end = start + thing.length;
						str += `.subarray(${start},${end})`;
					}
					return str;
				}
				case "DataView": {
					let str = `new DataView`;
					if (!names.has(thing.buffer)) str += `(new Uint8Array([${new Uint8Array(thing.buffer)}]).buffer`;
					else str += `(${stringify(thing.buffer)}`;
					if (thing.byteLength !== thing.buffer.byteLength) str += `,${thing.byteOffset},${thing.byteLength}`;
					return str + ")";
				}
				case "ArrayBuffer": return `new Uint8Array([${new Uint8Array(thing).toString()}]).buffer`;
				case "Temporal.Duration":
				case "Temporal.Instant":
				case "Temporal.PlainDate":
				case "Temporal.PlainTime":
				case "Temporal.PlainDateTime":
				case "Temporal.PlainMonthDay":
				case "Temporal.PlainYearMonth":
				case "Temporal.ZonedDateTime": return `${type}.from(${stringify_string(thing.toString())})`;
				default:
					const keys = Object.keys(thing);
					const obj = keys.map((key) => `${safe_key(key)}:${stringify(thing[key])}`).join(",");
					if (Object.getPrototypeOf(thing) === null) return keys.length > 0 ? `{${obj},__proto__:null}` : `{__proto__:null}`;
					return `{${obj}}`;
			}
		}
		const str = stringify(value);
		if (names.size) {
			/** @type {string[]} */
			const params = [];
			/** @type {string[]} */
			const statements = [];
			/** @type {string[]} */
			const values = [];
			/** @type {string[]} */
			const reconstructions = [];
			names.forEach((name, thing) => {
				params.push(name);
				if (custom.has(thing)) {
					values.push(custom.get(thing));
					return;
				}
				if (is_primitive(thing)) {
					values.push(stringify_cached_primitive(thing));
					return;
				}
				const type = get_type(thing);
				switch (type) {
					case "Number":
					case "String":
					case "Boolean":
					case "BigInt": {
						const primitive = thing.valueOf();
						if (names.has(primitive)) {
							values.push("{}");
							reconstructions.push(`${name}=Object(${stringify(primitive)})`);
						} else values.push(`Object(${stringify(primitive)})`);
						break;
					}
					case "RegExp":
						const { source, flags } = thing;
						const regexp = flags ? `new RegExp(${stringify_string(source)},"${flags}")` : `new RegExp(${stringify_string(source)})`;
						values.push(regexp);
						break;
					case "Date":
						values.push(`new Date(${thing.getTime()})`);
						break;
					case "URL":
						values.push(`new URL(${stringify_string(thing.toString())})`);
						break;
					case "URLSearchParams":
						values.push(`new URLSearchParams(${stringify_string(thing.toString())})`);
						break;
					case "Array": {
						const populated_keys = valid_array_indices(thing);
						values.push(thing.length > 32 + 2 * populated_keys.length ? stringify_sparse_array(thing.length) : `Array(${thing.length})`);
						for (const i of populated_keys) statements.push(`${name}[${i}]=${stringify(thing[i])}`);
						break;
					}
					case "Set": {
						values.push(`new Set`);
						const adds = Array.from(thing).map((v) => `.add(${stringify(v)})`);
						if (adds.length > 0) statements.push(name + adds.join(""));
						break;
					}
					case "Map": {
						values.push(`new Map`);
						const sets = Array.from(thing).map(([k, v]) => `.set(${stringify(k)}, ${stringify(v)})`);
						if (sets.length > 0) statements.push(name + sets.join(""));
						break;
					}
					case "Int8Array":
					case "Uint8Array":
					case "Uint8ClampedArray":
					case "Int16Array":
					case "Uint16Array":
					case "Float16Array":
					case "Int32Array":
					case "Uint32Array":
					case "Float32Array":
					case "Float64Array":
					case "BigInt64Array":
					case "BigUint64Array": {
						if (is_buffer(thing)) thing = new Uint8Array(thing);
						let str = `new ${type}`;
						if (!names.has(thing.buffer)) str += `([${stringify_typed_array_elements(type, thing.buffer)}])`;
						else str += `(${stringify(thing.buffer)})`;
						if (thing.byteLength !== thing.buffer.byteLength) {
							const start = thing.byteOffset / thing.BYTES_PER_ELEMENT;
							const end = start + thing.length;
							str += `.subarray(${start},${end})`;
						}
						values.push(`{}`);
						reconstructions.push(`${name}=${str}`);
						break;
					}
					case "DataView": {
						let str = `new DataView`;
						if (!names.has(thing.buffer)) str += `(new Uint8Array([${new Uint8Array(thing.buffer)}]).buffer`;
						else str += `(${stringify(thing.buffer)}`;
						if (thing.byteLength !== thing.buffer.byteLength) str += `,${thing.byteOffset},${thing.byteLength}`;
						str += ")";
						values.push(`{}`);
						reconstructions.push(`${name}=${str}`);
						break;
					}
					case "ArrayBuffer":
						values.push(`new Uint8Array([${new Uint8Array(thing)}]).buffer`);
						break;
					case "Temporal.Duration":
					case "Temporal.Instant":
					case "Temporal.PlainDate":
					case "Temporal.PlainTime":
					case "Temporal.PlainDateTime":
					case "Temporal.PlainMonthDay":
					case "Temporal.PlainYearMonth":
					case "Temporal.ZonedDateTime":
						values.push(`${type}.from(${stringify_string(thing.toString())})`);
						break;
					default:
						values.push(Object.getPrototypeOf(thing) === null ? "Object.create(null)" : "{}");
						Object.keys(thing).forEach((key) => {
							statements.push(`${name}${safe_prop(key)}=${stringify(thing[key])}`);
						});
				}
			});
			statements.push(`return ${str}`);
			const body = [...reconstructions, ...statements].join(";");
			if (params.length > 65534) return `(function(){var[${params.join(",")}]=arguments[0];${body}}([${values.join(",")}]))`;
			return `(function(${params.join(",")}){${body}}(${values.join(",")}))`;
		} else return str;
	}
	/**
	* Emit an array whose storage is not proportional to its declared length.
	* As in the default parse operations, touching and deleting the largest valid
	* index forces V8 into dictionary-elements mode before setting the length.
	* Merely starting with [] and assigning .length still eagerly allocates.
	* @param {number} length
	*/
	function stringify_sparse_array(length) {
		return `(function(a){a[${MAX_ARRAY_INDEX}]=0;delete a[${MAX_ARRAY_INDEX}];a.length=${length};return a}([]))`;
	}
	/**
	* Serialize the elements of `buffer`, read as `type`, as a comma-separated list.
	* The view is created from `type` rather than from the serialized value's own
	* constructor, which may be a subclass like Node's `Buffer` whose `toString`
	* decodes the bytes instead of listing them.
	* `BigInt64Array`/`BigUint64Array` elements are bigints and must be written
	* with an `n` suffix, otherwise the emitted `new BigInt64Array([...])` throws.
	* @param {string} type
	* @param {ArrayBufferLike} buffer
	*/
	function stringify_typed_array_elements(type, buffer) {
		const array = new globalThis[type](buffer);
		if (type === "BigInt64Array" || type === "BigUint64Array") return Array.from(array, (element) => `${element}n`).join(",");
		if (array instanceof Float32Array || array instanceof Float64Array || typeof Float16Array !== "undefined" && array instanceof Float16Array) return Array.from(array, (element) => Object.is(element, -0) ? "-0" : `${element}`).join(",");
		return array.toString();
	}
	/** @param {number} num */
	function get_name(num) {
		let name = "";
		do {
			name = chars[num % 54] + name;
			num = ~~(num / 54) - 1;
		} while (num >= 0);
		return reserved.test(name) ? `${name}0` : name;
	}
	/** @param {string} c */
	function escape_unsafe_char(c) {
		return escaped[c] || c;
	}
	/** @param {string} str */
	function escape_unsafe_chars(str) {
		return str.replace(unsafe_chars, escape_unsafe_char);
	}
	/** @param {string} key */
	function safe_key(key) {
		return /^[_$a-zA-Z][_$a-zA-Z0-9]*$/.test(key) ? key : escape_unsafe_chars(JSON.stringify(key));
	}
	/** @param {string} key */
	function safe_prop(key) {
		return /^[_$a-zA-Z][_$a-zA-Z0-9]*$/.test(key) ? `.${key}` : `[${escape_unsafe_chars(JSON.stringify(key))}]`;
	}
	/** @param {any} thing */
	function stringify_primitive$1(thing) {
		const type = typeof thing;
		if (type === "string") return stringify_string(thing);
		if (thing === void 0) return "void 0";
		if (thing === 0 && 1 / thing < 0) return "-0";
		const str = String(thing);
		if (type === "number") return str.replace(/^(-)?0\./, "$1.");
		if (type === "bigint") return thing + "n";
		return str;
	}
	/**	@type {(array_buffer: ArrayBuffer) => string} */
	function encode_native(array_buffer) {
		return new Uint8Array(array_buffer).toBase64();
	}
	/**	@type {(base64: string) => ArrayBuffer} */
	function decode_native(base64) {
		return Uint8Array.fromBase64(base64).buffer;
	}
	/** @type {(array_buffer: ArrayBuffer) => string} */
	function encode_buffer(array_buffer) {
		return Buffer.from(array_buffer).toString("base64");
	}
	/**	@type {(base64: string) => ArrayBuffer} */
	function decode_buffer(base64) {
		return Uint8Array.from(Buffer.from(base64, "base64")).buffer;
	}
	/** @type {(array_buffer: ArrayBuffer) => string} */
	function encode_legacy(array_buffer) {
		const array = new Uint8Array(array_buffer);
		let binary = "";
		const chunk_size = 32768;
		for (let i = 0; i < array.length; i += chunk_size) {
			const chunk = array.subarray(i, i + chunk_size);
			binary += String.fromCharCode.apply(null, chunk);
		}
		return btoa(binary);
	}
	/**	@type {(base64: string) => ArrayBuffer} */
	function decode_legacy(base64) {
		const binary_string = atob(base64);
		const len = binary_string.length;
		const array = new Uint8Array(len);
		for (let i = 0; i < len; i++) array[i] = binary_string.charCodeAt(i);
		return array.buffer;
	}
	var native = typeof Uint8Array.fromBase64 === "function";
	var buffer = typeof process === "object" && process.versions?.node !== void 0;
	var encode64 = native ? encode_native : buffer ? encode_buffer : encode_legacy;
	var decode64 = native ? decode_native : buffer ? decode_buffer : decode_legacy;
	/**
	* Merges caller-provided operation overrides over the defaults. Iterating the
	* default keys (rather than the override's own keys) means nullish members
	* fall back to the default, and inherited members — e.g. from a class
	* instance — are picked up.
	*
	* @template {Record<string, any>} T
	* @param {T} defaults
	* @param {Partial<T> | undefined} overrides
	* @returns {T}
	*/
	function merge_operations(defaults, overrides) {
		if (!overrides) return defaults;
		const merged = {};
		for (const key of Object.keys(defaults)) merged[key] = overrides[key] ?? defaults[key];
		return merged;
	}
	/** @type {{ kind: 'not-plain' }} */
	var NOT_PLAIN = /* @__PURE__ */ Object.freeze({ kind: "not-plain" });
	/** @type {{ kind: 'symbol-keys' }} */
	var SYMBOL_KEYS = /* @__PURE__ */ Object.freeze({ kind: "symbol-keys" });
	var default_stringify_operations = /* @__PURE__ */ Object.freeze({
		identify: (value) => value,
		typeOf: (value) => value === null ? "null" : typeof value,
		toPrimitive: (value) => value,
		tagOf: (value) => get_type(value),
		isThenable: (value) => typeof value.then === "function",
		toPromise: (thenable) => Promise.resolve(thenable),
		unbox: (boxed) => boxed.valueOf(),
		toISOString: (date) => isNaN(date.getDate()) ? "" : date.toISOString(),
		toStringValue: (value) => value.toString(),
		regExpInfo: (regexp) => ({
			source: regexp.source,
			flags: regexp.flags
		}),
		valuesOf: (set) => set,
		entriesOf: (map) => map,
		viewInfo: (view) => {
			if (is_buffer(view)) view = new Uint8Array(view);
			return {
				buffer: view.buffer,
				byteOffset: view.byteOffset,
				byteLength: view.byteLength,
				length: view.length,
				bufferByteLength: view.buffer.byteLength
			};
		},
		toArrayBuffer: (buffer) => buffer,
		lengthOf: (array) => array.length,
		hasOwn: (value, key) => Object.hasOwn(value, key),
		indicesOf: (array) => valid_array_indices(array),
		shapeOf: (value) => {
			if (!is_plain_object$1(value)) return NOT_PLAIN;
			if (enumerable_symbols(value).length > 0) return SYMBOL_KEYS;
			return {
				kind: Object.getPrototypeOf(value) === null ? "null-proto" : "plain",
				keys: Object.keys(value)
			};
		},
		get: (value, key) => value[key]
	});
	var array_buffer_byte_length = Object.getOwnPropertyDescriptor(ArrayBuffer.prototype, "byteLength").get;
	var shared_array_buffer_byte_length = typeof SharedArrayBuffer === "undefined" ? void 0 : Object.getOwnPropertyDescriptor(SharedArrayBuffer.prototype, "byteLength").get;
	var default_parse_operations = /* @__PURE__ */ Object.freeze({
		fromPrimitive: (primitive) => primitive,
		fromISOString: (iso) => new Date(iso),
		fromStringValue: (tag, text) => {
			if (tag === "URL") return new URL(text);
			if (tag === "URLSearchParams") return new URLSearchParams(text);
			return Temporal[tag.slice(9)].from(text);
		},
		fromArrayBuffer: (buffer) => buffer,
		fromRegExpInfo: (source, flags) => new RegExp(source, flags),
		fromViewInfo: (tag, buffer, byteOffset, length) => {
			try {
				array_buffer_byte_length.call(buffer);
			} catch (error) {
				if (!shared_array_buffer_byte_length) throw error;
				shared_array_buffer_byte_length.call(buffer);
			}
			const Constructor = globalThis[tag];
			return byteOffset !== void 0 ? new Constructor(buffer, byteOffset, length) : new Constructor(buffer);
		},
		box: (value) => Object(value),
		createArray: (length) => new Array(length),
		createSparseArray: (length) => {
			/** @type {any[]} */
			const array = [];
			array[MAX_ARRAY_INDEX] = void 0;
			delete array[MAX_ARRAY_INDEX];
			array.length = length;
			return array;
		},
		createObject: () => ({}),
		createNullPrototypeObject: () => Object.create(null),
		createSet: () => /* @__PURE__ */ new Set(),
		createMap: () => /* @__PURE__ */ new Map(),
		set: (target, key, value) => {
			target[key] = value;
		},
		addValue: (set, value) => {
			set.add(value);
		},
		addEntry: (map, key, value) => {
			map.set(key, value);
		}
	});
	/**
	* Revive a value serialized with `devalue.stringify`
	* @param {string} serialized
	* @param {Record<string, (value: any) => any>} [revivers]
	* @param {import('./types.js').ParseOptions} [options]
	*/
	function parse$1(serialized, revivers, options) {
		return unflatten(JSON.parse(serialized), revivers, options);
	}
	/**
	* Revive a value flattened with `devalue.stringify`
	* @param {number | any[]} parsed
	* @param {Record<string, (value: any) => any>} [revivers]
	* @param {import('./types.js').ParseOptions} [options]
	*/
	function unflatten(parsed, revivers, options) {
		/** @type {import('./types.js').ParseOperations} */
		const ops = merge_operations(default_parse_operations, options?.operations);
		if (typeof parsed === "number") return hydrate(parsed, true);
		if (!Array.isArray(parsed) || parsed.length === 0) throw new Error("Invalid input");
		const values = parsed;
		const hydrated = Array(values.length);
		/**
		* A set of values currently being hydrated with custom revivers,
		* used to detect invalid cyclical dependencies
		* @type {Set<number> | null}
		*/
		let hydrating = null;
		/**
		* @param {number} index
		* @returns {any}
		*/
		function hydrate(index, standalone = false) {
			if (index === -1) return ops.fromPrimitive(void 0);
			if (index === -3) return ops.fromPrimitive(NaN);
			if (index === -4) return ops.fromPrimitive(Infinity);
			if (index === -5) return ops.fromPrimitive(-Infinity);
			if (index === -6) return ops.fromPrimitive(-0);
			if (standalone || typeof index !== "number") throw new Error(`Invalid input`);
			if (index in hydrated) return hydrated[index];
			if (index >= values.length) throw new Error(`Invalid input`);
			const value = values[index];
			if (!value || typeof value !== "object") hydrated[index] = ops.fromPrimitive(value);
			else if (Array.isArray(value)) {
				if (typeof value[0] === "string") {
					const type = value[0];
					const reviver = revivers && Object.hasOwn(revivers, type) ? revivers[type] : void 0;
					if (reviver) {
						let i = value[1];
						if (typeof i !== "number") i = values.push(value[1]) - 1;
						if (Object.hasOwn(hydrated, i)) return hydrated[index] = reviver(hydrated[i]);
						hydrating ??= /* @__PURE__ */ new Set();
						if (hydrating.has(i)) throw new Error("Invalid circular reference");
						hydrating.add(i);
						hydrated[index] = reviver(hydrate(i));
						hydrating.delete(i);
						return hydrated[index];
					}
					switch (type) {
						case "Date":
							hydrated[index] = ops.fromISOString(value[1]);
							break;
						case "Set":
							const set = ops.createSet();
							hydrated[index] = set;
							for (let i = 1; i < value.length; i += 1) ops.addValue(set, hydrate(value[i]));
							break;
						case "Map":
							const map = ops.createMap();
							hydrated[index] = map;
							for (let i = 1; i < value.length; i += 2) ops.addEntry(map, hydrate(value[i]), hydrate(value[i + 1]));
							break;
						case "RegExp":
							hydrated[index] = ops.fromRegExpInfo(value[1], value[2]);
							break;
						case "Object": {
							const wrapped_index = value[1];
							if (typeof values[wrapped_index] === "object" && values[wrapped_index][0] !== "BigInt") throw new Error("Invalid input");
							hydrated[index] = ops.box(hydrate(wrapped_index));
							break;
						}
						case "BigInt":
							hydrated[index] = ops.fromPrimitive(BigInt(value[1]));
							break;
						case "null":
							const obj = ops.createNullPrototypeObject();
							hydrated[index] = obj;
							for (let i = 1; i < value.length; i += 2) {
								const key = value[i];
								if (typeof key !== "string") throw new Error("Cannot parse an object with a non-string key");
								if (key === "__proto__") throw new Error("Cannot parse an object with a `__proto__` property");
								ops.set(obj, key, hydrate(value[i + 1]));
							}
							break;
						case "Int8Array":
						case "Uint8Array":
						case "Uint8ClampedArray":
						case "Int16Array":
						case "Uint16Array":
						case "Float16Array":
						case "Int32Array":
						case "Uint32Array":
						case "Float32Array":
						case "Float64Array":
						case "BigInt64Array":
						case "BigUint64Array":
						case "DataView": {
							if (values[value[1]][0] !== "ArrayBuffer") throw new Error("Invalid data");
							const buffer = hydrate(value[1]);
							hydrated[index] = ops.fromViewInfo(type, buffer, value[2], value[3]);
							break;
						}
						case "ArrayBuffer": {
							const base64 = value[1];
							if (typeof base64 !== "string") throw new Error("Invalid ArrayBuffer encoding");
							hydrated[index] = ops.fromArrayBuffer(decode64(base64));
							break;
						}
						case "URL":
						case "URLSearchParams":
						case "Temporal.Duration":
						case "Temporal.Instant":
						case "Temporal.PlainDate":
						case "Temporal.PlainTime":
						case "Temporal.PlainDateTime":
						case "Temporal.PlainMonthDay":
						case "Temporal.PlainYearMonth":
						case "Temporal.ZonedDateTime":
							hydrated[index] = ops.fromStringValue(type, value[1]);
							break;
						default: throw new Error(`Unknown type ${type}`);
					}
				} else if (value[0] === -7) {
					const len = value[1];
					if (!is_valid_array_len(len)) throw new Error("Invalid input");
					const array = ops.createSparseArray(len);
					hydrated[index] = array;
					for (let i = 2; i < value.length; i += 2) {
						const idx = value[i];
						if (!is_valid_array_index(idx) || idx >= len) throw new Error("Invalid input");
						ops.set(array, idx, hydrate(value[i + 1]));
					}
				} else {
					const array = ops.createArray(value.length);
					hydrated[index] = array;
					for (let i = 0; i < value.length; i += 1) {
						const n = value[i];
						if (n === -2) continue;
						ops.set(array, i, hydrate(n));
					}
				}
			} else {
				const object = ops.createObject();
				hydrated[index] = object;
				for (const key of Object.keys(value)) {
					if (key === "__proto__") throw new Error("Cannot parse an object with a `__proto__` property");
					ops.set(object, key, hydrate(value[key]));
				}
			}
			return hydrated[index];
		}
		return hydrate(0);
	}
	/**
	* Turn a value into a JSON string that can be parsed with `devalue.parse`
	* @param {any} value
	* @param {Record<string, (value: any) => any>} [reducers]
	* @param {import('./types.js').StringifyOptions} [options]
	*/
	function stringify$1$1(value, reducers, options) {
		const stringified = run$1(false, value, reducers, options);
		return typeof stringified === "string" ? stringified : `[${stringified.join(",")}]`;
	}
	/**
	* @param {boolean} async
	* @param {any} value
	* @param {Record<string, (value: any) => any>} [reducers]
	* @param {import('./types.js').StringifyOptions} [options]
	*/
	function run$1(async, value, reducers, options) {
		const ops = merge_operations(default_stringify_operations, options?.operations);
		/** @type {any[]} */
		const stringified = [];
		/** @type {Map<any, number>} */
		const indexes = /* @__PURE__ */ new Map();
		/** @type {Array<{ key: string, fn: (value: any) => any }>} */
		const custom = [];
		if (reducers) for (const key of Object.getOwnPropertyNames(reducers)) custom.push({
			key,
			fn: reducers[key]
		});
		/** @type {string[]} */
		const keys = [];
		let p = 0;
		/**
		* @param {any} thing
		* @param {number} [index]
		*/
		function flatten(thing, index) {
			const type = ops.typeOf(thing);
			if (type === "undefined") return -1;
			/** @type {number | undefined} */
			let number;
			if (type === "number") {
				number = ops.toPrimitive(thing);
				if (Number.isNaN(number)) return -3;
				if (number === Infinity) return -4;
				if (number === -Infinity) return -5;
				if (number === 0 && 1 / number < 0) return -6;
			}
			const id = ops.identify(thing);
			if (indexes.has(id)) return indexes.get(id);
			index ??= p++;
			indexes.set(id, index);
			for (const { key, fn } of custom) {
				const value = fn(thing);
				if (value) {
					stringified[index] = `["${key}",${flatten(value)}]`;
					return index;
				}
			}
			if (type === "function") throw new DevalueError(`Cannot stringify a function`, keys, thing, value);
			else if (type === "symbol") throw new DevalueError(`Cannot stringify a Symbol primitive`, keys, thing, value);
			/** @type {string | Promise<any>} */
			let str = "";
			if (type !== "object") str = stringify_primitive(type === "number" ? number : ops.toPrimitive(thing));
			else if (ops.isThenable(thing)) {
				if (!async) throw new DevalueError(`Cannot stringify a Promise or thenable — use stringifyAsync instead`, keys, thing, value);
				str = ops.toPromise(thing).then((value) => {
					const i = flatten(value, index);
					if (i < 0) stringified[index] = i;
				});
				str.catch(() => {});
			} else {
				const tag = ops.tagOf(thing);
				switch (tag) {
					case "Number":
					case "String":
					case "Boolean":
					case "BigInt":
						str = `["Object",${flatten(ops.unbox(thing))}]`;
						break;
					case "Date":
						str = `["Date","${ops.toISOString(thing)}"]`;
						break;
					case "URL":
						str = `["URL",${stringify_string(ops.toStringValue(thing))}]`;
						break;
					case "URLSearchParams":
						str = `["URLSearchParams",${stringify_string(ops.toStringValue(thing))}]`;
						break;
					case "RegExp":
						const { source, flags } = ops.regExpInfo(thing);
						str = flags ? `["RegExp",${stringify_string(source)},"${flags}"]` : `["RegExp",${stringify_string(source)}]`;
						break;
					case "Array": {
						let mostly_dense = false;
						const length = ops.lengthOf(thing);
						str = "[";
						for (let i = 0; i < length; i += 1) {
							if (i > 0) str += ",";
							if (ops.hasOwn(thing, i)) {
								keys.push(`[${i}]`);
								str += flatten(ops.get(thing, i));
								keys.pop();
							} else if (mostly_dense) str += -2;
							else {
								const populated_keys = ops.indicesOf(thing);
								const population = populated_keys.length;
								const d = String(length).length;
								if ((length - population) * 3 > 4 + d + population * (d + 1)) {
									str = "[-7," + length;
									for (let j = 0; j < populated_keys.length; j++) {
										const key = populated_keys[j];
										keys.push(`[${key}]`);
										str += "," + key + "," + flatten(ops.get(thing, key));
										keys.pop();
									}
									break;
								} else {
									mostly_dense = true;
									str += -2;
								}
							}
						}
						str += "]";
						break;
					}
					case "Set":
						str = "[\"Set\"";
						for (const value of ops.valuesOf(thing)) str += `,${flatten(value)}`;
						str += "]";
						break;
					case "Map":
						str = "[\"Map\"";
						for (const [key, value] of ops.entriesOf(thing)) {
							const key_type = ops.typeOf(key);
							const key_is_primitive = key_type !== "object" && key_type !== "function" && key_type !== "symbol";
							keys.push(`.get(${key_is_primitive ? stringify_primitive(ops.toPrimitive(key)) : "..."})`);
							str += `,${flatten(key)},${flatten(value)}`;
							keys.pop();
						}
						str += "]";
						break;
					case "Int8Array":
					case "Uint8Array":
					case "Uint8ClampedArray":
					case "Int16Array":
					case "Uint16Array":
					case "Float16Array":
					case "Int32Array":
					case "Uint32Array":
					case "Float32Array":
					case "Float64Array":
					case "BigInt64Array":
					case "BigUint64Array": {
						const info = ops.viewInfo(thing);
						str = "[\"" + tag + "\"," + flatten(info.buffer);
						if (info.byteLength !== info.bufferByteLength) str += `,${info.byteOffset},${info.length}`;
						str += "]";
						break;
					}
					case "DataView": {
						const info = ops.viewInfo(thing);
						str = "[\"" + tag + "\"," + flatten(info.buffer);
						if (info.byteLength !== info.bufferByteLength) str += `,${info.byteOffset},${info.byteLength}`;
						str += "]";
						break;
					}
					case "ArrayBuffer":
						str = `["ArrayBuffer","${encode64(ops.toArrayBuffer(thing))}"]`;
						break;
					case "Temporal.Duration":
					case "Temporal.Instant":
					case "Temporal.PlainDate":
					case "Temporal.PlainTime":
					case "Temporal.PlainDateTime":
					case "Temporal.PlainMonthDay":
					case "Temporal.PlainYearMonth":
					case "Temporal.ZonedDateTime":
						str = `["${tag}",${stringify_string(ops.toStringValue(thing))}]`;
						break;
					default: {
						const shape = ops.shapeOf(thing);
						if (shape.kind === "not-plain") throw new DevalueError(`Cannot stringify arbitrary non-POJOs`, keys, thing, value);
						if (shape.kind === "symbol-keys") throw new DevalueError(`Cannot stringify POJOs with symbolic keys`, keys, thing, value);
						if (shape.kind === "null-proto") {
							str = "[\"null\"";
							for (const key of shape.keys) {
								if (key === "__proto__") throw new DevalueError(`Cannot stringify objects with __proto__ keys`, keys, thing, value);
								keys.push(stringify_key(key));
								str += `,${stringify_string(key)},${flatten(ops.get(thing, key))}`;
								keys.pop();
							}
							str += "]";
						} else {
							str = "{";
							let started = false;
							for (const key of shape.keys) {
								if (key === "__proto__") throw new DevalueError(`Cannot stringify objects with __proto__ keys`, keys, thing, value);
								if (started) str += ",";
								started = true;
								keys.push(stringify_key(key));
								str += `${stringify_string(key)}:${flatten(ops.get(thing, key))}`;
								keys.pop();
							}
							str += "}";
						}
					}
				}
			}
			stringified[index] = str;
			return index;
		}
		const index = flatten(value);
		if (index < 0) return `${index}`;
		return stringified;
	}
	/**
	* @param {any} thing
	* @returns {string}
	*/
	function stringify_primitive(thing) {
		const type = typeof thing;
		if (type === "string") return stringify_string(thing);
		if (thing === void 0) return (-1).toString();
		if (thing === 0 && 1 / thing < 0) return (-6).toString();
		if (type === "bigint") return `["BigInt","${thing}"]`;
		return String(thing);
	}
	/**
	* @typedef {{ cause?: unknown; stackless?: boolean }} ServerThrowOptions
	*/
	/**
	* Throws the full diagnostic. Unlike shared errors, server-only errors keep their full text in production
	* @param {string} code
	* @param {string} message
	* @param {ServerThrowOptions | undefined} options
	* @param {Function} caller The generated helper, which is omitted from the stack along with this function
	* @returns {never}
	*/
	function throw_error$1(code, message, options, caller) {
		const error = new Error(`${code}\n${message}\nhttps://svelte.dev/e/kit/${code}`, options?.cause === void 0 ? void 0 : { cause: options.cause });
		error.name = "SvelteKit error";
		if (options?.stackless) error.stack = "";
		else Error.captureStackTrace?.(error, caller);
		throw error;
	}
	/**
	* Returns the error thrown by `fn`, for the few places that pass an error on (for example to
	* `handleError` or a later `console.error`) rather than throwing it. Its stack starts inside `fn`
	* @param {() => never} fn A function that calls a generated `server-errors` helper
	* @returns {Error}
	*/
	function capture_error(fn) {
		try {
			fn();
		} catch (error) {
			return error;
		}
	}
	/** @import { ServerThrowOptions } from './internal/server.js' */
	/**
	* The `handleError` hook failed
	* @param {void} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function handle_error_hook_failed(_values, options) {
		throw_error$1("handle_error_hook_failed", `The \`handleError\` hook failed`, options, handle_error_hook_failed);
	}
	/**
	* CORS error: %reason% `Access-Control-Allow-Origin` header is present on the requested resource
	* @param {{ "reason": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function load_fetch_cors(_values, options) {
		throw_error$1("load_fetch_cors", `CORS error: ${_values.reason} \`Access-Control-Allow-Origin\` header is present on the requested resource`, options, load_fetch_cors);
	}
	/**
	* Failed to get response header `%name%` — it must be included by the `filterSerializedResponseHeaders` option: https://svelte.dev/docs/kit/hooks#handle (at `%id%`)
	* @param {{ "name": string; "id": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function load_response_header_not_serialized(_values, options) {
		throw_error$1("load_response_header_not_serialized", `Failed to get response header \`${_values.name}\` — it must be included by the \`filterSerializedResponseHeaders\` option: https://svelte.dev/docs/kit/hooks#handle (at \`${_values.id}\`)`, options, load_response_header_not_serialized);
	}
	/**
	* Cannot call a command (`%name%`) from a `%method%` handler
	* @param {{ "name": string; "method": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_command_method(_values, options) {
		throw_error$1("remote_command_method", `Cannot call a command (\`${_values.name}\`) from a \`${_values.method}\` handler`, options, remote_command_method);
	}
	/**
	* Cannot call a command (`%name%`) inside a query or prerender function
	* @param {{ "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_command_readonly(_values, options) {
		throw_error$1("remote_command_readonly", `Cannot call a command (\`${_values.name}\`) inside a query or prerender function`, options, remote_command_readonly);
	}
	/**
	* Cannot call a command (`%name%`) during server-side rendering
	* @param {{ "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_command_render(_values, options) {
		throw_error$1("remote_command_render", `Cannot call a command (\`${_values.name}\`) during server-side rendering`, options, remote_command_render);
	}
	/**
	* Cannot %operation% cookies in `query` or `prerender` functions
	* @param {{ "operation": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_cookie_forbidden(_values, options) {
		throw_error$1("remote_cookie_forbidden", `Cannot ${_values.operation} cookies in \`query\` or \`prerender\` functions`, options, remote_cookie_forbidden);
	}
	/**
	* Cookies %operation% in remote functions must have an absolute path
	* @param {{ "operation": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_cookie_path_relative(_values, options) {
		throw_error$1("remote_cookie_path_relative", `Cookies ${_values.operation} in remote functions must have an absolute path`, options, remote_cookie_path_relative);
	}
	/**
	* `setHeaders` is not allowed in remote functions
	* @param {void} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_headers_forbidden(_values, options) {
		throw_error$1("remote_headers_forbidden", `\`setHeaders\` is not allowed in remote functions`, options, remote_headers_forbidden);
	}
	/**
	* Invalid validator passed to remote function. Expected `'unchecked'` or a Standard Schema (https://standardschema.dev)
	* @param {void} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_invalid_validator(_values, options) {
		throw_error$1("remote_invalid_validator", `Invalid validator passed to remote function. Expected \`'unchecked'\` or a Standard Schema (https://standardschema.dev)`, options, remote_invalid_validator);
	}
	/**
	* Cannot export `default` from a remote module (`%file%`) — please use named exports instead
	* @param {{ "file": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_module_default_export(_values, options) {
		throw_error$1("remote_module_default_export", `Cannot export \`default\` from a remote module (\`${_values.file}\`) — please use named exports instead`, options, remote_module_default_export);
	}
	/**
	* `%name%` exported from `%file%` is invalid — all exports from this file must be remote functions
	* @param {{ "name": string; "file": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_module_invalid_export(_values, options) {
		throw_error$1("remote_module_invalid_export", `\`${_values.name}\` exported from \`${_values.file}\` is invalid — all exports from this file must be remote functions`, options, remote_module_invalid_export);
	}
	/**
	* `query.live` `%name%` did not yield a value
	* @param {{ "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_query_live_no_value(_values, options) {
		throw_error$1("remote_query_live_no_value", `\`query.live\` \`${_values.name}\` did not yield a value`, options, remote_query_live_no_value);
	}
	/**
	* `query.live` `%name%` must return an `Iterator`, `Iterable`, `AsyncIterator` or `AsyncIterable`
	* @param {{ "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_query_live_not_iterable(_values, options) {
		throw_error$1("remote_query_live_not_iterable", `\`query.live\` \`${_values.name}\` must return an \`Iterator\`, \`Iterable\`, \`AsyncIterator\` or \`AsyncIterable\``, options, remote_query_live_not_iterable);
	}
	/**
	* Cannot call `%type%` `%name%` while prerendering, as prerendered pages need static data. Use `prerender` from `$app/server` instead
	* @param {{ "type": string; "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_query_prerender(_values, options) {
		throw_error$1("remote_query_prerender", `Cannot call \`${_values.type}\` \`${_values.name}\` while prerendering, as prerendered pages need static data. Use \`prerender\` from \`$app/server\` instead`, options, remote_query_prerender);
	}
	/**
	* Cannot access `event.%property%` in a query. Pass the value as an argument to the query instead
	* @param {{ "property": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function remote_request_property(_values, options) {
		throw_error$1("remote_request_property", `Cannot access \`event.${_values.property}\` in a query. Pass the value as an argument to the query instead`, options, remote_request_property);
	}
	/**
	* Could not get the request store. In environments without `AsyncLocalStorage`, the request store (used by e.g. remote functions) must be accessed synchronously, not after an `await`. If it was accessed synchronously then this is an internal error.
	* @param {void} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function request_store_after_await(_values, options) {
		throw_error$1("request_store_after_await", `Could not get the request store. In environments without \`AsyncLocalStorage\`, the request store (used by e.g. remote functions) must be accessed synchronously, not after an \`await\`. If it was accessed synchronously then this is an internal error.`, options, request_store_after_await);
	}
	/**
	* Cannot call `%name%` on the server
	* @param {{ "name": string }} _values
	* @param {ServerThrowOptions} [options]
	* @returns {never}
	*/
	function server_api_unavailable(_values, options) {
		throw_error$1("server_api_unavailable", `Cannot call \`${_values.name}\` on the server`, options, server_api_unavailable);
	}
	function noop$2() {}
	var MUTATIVE_METHODS = [
		"POST",
		"PUT",
		"PATCH",
		"DELETE"
	];
	[...MUTATIVE_METHODS];
	({}).dirname;
	var IN_WEBCONTAINER = !!globalThis.process?.versions?.webcontainer;
	/** @import { RequestEvent } from '@sveltejs/kit' */
	/** @import { RequestStore } from 'types' */
	/** @import { AsyncLocalStorage } from 'node:async_hooks' */
	/** @type {RequestStore | null} */
	var sync_store = null;
	/** @type {AsyncLocalStorage<RequestStore | null> | null} */
	var als$1;
	Promise.resolve().then(() => /* @__PURE__ */ __toESM(require__skgo_skgo_missing(), 1)).then((hooks) => als$1 = new hooks.AsyncLocalStorage()).catch(() => {});
	function get_request_store() {
		const result = try_get_request_store();
		if (!result) {
			if (als$1) throw new Error("Could not get the request store. This is an internal error.");
			request_store_after_await();
		}
		return result;
	}
	function try_get_request_store() {
		return sync_store ?? als$1?.getStore() ?? null;
	}
	/**
	* @template T
	* @param {RequestStore | null} store
	* @param {() => T} fn
	*/
	function with_request_store(store, fn) {
		try {
			sync_store = store;
			return als$1 ? als$1.run(store, fn) : fn();
		} finally {
			if (!IN_WEBCONTAINER) sync_store = null;
		}
	}
	/** @import { RemoteInternals } from 'types' */
	/** @type {RemoteInternals['type'][]} */
	var types = [
		"command",
		"form",
		"prerender",
		"query",
		"query_batch",
		"query_live"
	];
	/**
	* @param {Record<string, any>} module
	* @param {string} file
	* @param {string} hash
	*/
	function init_remote_functions(module, file, hash) {
		if (module.default) remote_module_default_export({ file });
		for (const [name, fn] of Object.entries(module)) {
			if (!types.includes(fn?.__?.type)) remote_module_invalid_export({
				name,
				file
			});
			fn.__.id = `${hash}/${name}`;
			fn.__.name = name;
		}
	}
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	var HttpError = class {
		/**
		* @param {App.Error} error
		*/
		constructor(error) {
			this.status = error.status;
			this.body = error;
		}
		toString() {
			return JSON.stringify(this.body);
		}
	};
	/**
	* An `HttpError` whose body is already in its final, user-facing form — either produced by the
	* `handleError` hook on the server and reconstructed here from the response, or authored directly
	* by the client runtime. Unlike a plain `HttpError` (which represents a fresh `error(...)` call
	* that the hook has yet to see), `handleError` must not run on it.
	* @extends HttpError
	*/
	var HandledHttpError = class extends HttpError {};
	var Redirect = class {
		/**
		* @param {300 | 301 | 302 | 303 | 304 | 305 | 306 | 307 | 308} status
		* @param {string} location
		*/
		constructor(status, location) {
			try {
				new Headers({ location });
			} catch {
				throw new Error(`Invalid redirect location ${JSON.stringify(location)}: this string contains characters that cannot be used in HTTP headers`);
			}
			this.status = status;
			this.location = location;
		}
	};
	/**
	* An error that was thrown from within the SvelteKit runtime that is not fatal and doesn't result in a 500, such as a 404.
	* `SvelteKitError` goes through `handleError`.
	* @extends Error
	*/
	var SvelteKitError = class extends Error {
		/**
		* @param {number} status
		* @param {string} text
		* @param {string} message
		*/
		constructor(status, text, message) {
			super(message);
			this.status = status;
			this.text = text;
		}
	};
	/**
	* Error thrown when form validation fails imperatively
	*/
	var ValidationError = class extends Error {
		/**
		* @param {StandardSchemaV1.Issue[]} issues
		*/
		constructor(issues) {
			super("Validation failed");
			this.name = "ValidationError";
			this.issues = issues;
		}
	};
	/** @type {(data: any) => string} */
	var stringify$2 = () => {
		throw new Error("");
	};
	/** @type {(data: string) => any} */
	var parse$2 = () => {
		throw new Error("");
	};
	/** @type {Record<string, (data: any) => any>} */
	var encoders = {};
	/** @type {Record<string, (data: any) => any>} */
	var decoders = {};
	/**
	*
	* @param {Transport} transport
	*/
	function init_transport(transport) {
		const transporters = Object.entries(transport);
		transporters.length;
		encoders = Object.fromEntries(transporters.map(([k, v]) => [k, v.encode]));
		decoders = Object.fromEntries(transporters.map(([k, v]) => [k, v.decode]));
		stringify$2 = (data) => stringify$1$1(data, encoders);
		parse$2 = (data) => parse$1(data, decoders);
	}
	var prerendering = false;
	var DATA_SUFFIX = "/__data.json";
	var HTML_DATA_SUFFIX = ".html__data.json";
	/** @param {string} pathname */
	function add_data_suffix(pathname) {
		if (pathname.endsWith(".html")) return pathname.replace(/\.html$/, HTML_DATA_SUFFIX);
		return pathname.replace(/\/$/, "") + DATA_SUFFIX;
	}
	/**
	* Whether shared errors include their full text on the server even though `DEV` is false. This is
	* enabled while the app is being built (analysed or prerendered)
	*/
	var verbose = false;
	/**
	* Throws the full diagnostic. Generated helpers throw their URL-only production error inline, so
	* bundlers can remove both this call and the message text from production browser builds
	* @param {string} code
	* @param {string} message
	* @param {Function} caller The generated helper, which is omitted from the stack along with this function
	* @returns {never}
	*/
	function throw_error(code, message, caller) {
		const error = /* @__PURE__ */ new Error(`${code}\n${message}\nhttps://svelte.dev/e/kit/${code}`);
		error.name = "SvelteKit error";
		Error.captureStackTrace?.(error, caller);
		throw error;
	}
	/**
	* Invalid array key `%key%`
	* @param {{ "key": string }} _values
	* @returns {never}
	*/
	function form_field_array_conflict(_values) {
		if (verbose) throw_error("form_field_array_conflict", `Invalid array key \`${_values.key}\``, form_field_array_conflict);
		throw new Error("https://svelte.dev/e/kit/form_field_array_conflict");
	}
	/**
	* Invalid key `%key%`: This key is not allowed to prevent prototype pollution.
	* @param {{ "key": string }} _values
	* @returns {never}
	*/
	function form_field_forbidden_key(_values) {
		if (verbose) throw_error("form_field_forbidden_key", `Invalid key \`${_values.key}\`: This key is not allowed to prevent prototype pollution.`, form_field_forbidden_key);
		throw new Error("https://svelte.dev/e/kit/form_field_forbidden_key");
	}
	/**
	* Invalid field name `%name%`: field names are written in JS object notation, so keys that would need quoting are not supported. See https://svelte.dev/docs/kit/remote-functions#form-Fields
	* @param {{ "name": string }} _values
	* @returns {never}
	*/
	function form_field_invalid_name(_values) {
		if (verbose) throw_error("form_field_invalid_name", `Invalid field name \`${_values.name}\`: field names are written in JS object notation, so keys that would need quoting are not supported. See https://svelte.dev/docs/kit/remote-functions#form-Fields`, form_field_invalid_name);
		throw new Error("https://svelte.dev/e/kit/form_field_invalid_name");
	}
	/**
	* Form contained a field that wasn't created with `form.fields.as(...)`: `%name%`
	* @param {{ "name": string }} _values
	* @returns {never}
	*/
	function form_field_unbound(_values) {
		if (verbose) throw_error("form_field_unbound", `Form contained a field that wasn't created with \`form.fields.as(...)\`: \`${_values.name}\``, form_field_unbound);
		throw new Error("https://svelte.dev/e/kit/form_field_unbound");
	}
	/**
	* HTTP error status codes must be between 400 and 599 — %status% is invalid
	* @param {{ "status": string }} _values
	* @returns {never}
	*/
	function invalid_error_status(_values) {
		if (verbose) throw_error("invalid_error_status", `HTTP error status codes must be between 400 and 599 — ${_values.status} is invalid`, invalid_error_status);
		throw new Error("https://svelte.dev/e/kit/invalid_error_status");
	}
	/**
	* %type% are not valid remote function arguments
	* @param {{ "type": string }} _values
	* @returns {never}
	*/
	function remote_argument_unsupported(_values) {
		if (verbose) throw_error("remote_argument_unsupported", `${_values.type} are not valid remote function arguments`, remote_argument_unsupported);
		throw new Error("https://svelte.dev/e/kit/remote_argument_unsupported");
	}
	/**
	* Missing params for dynamic route ID `%id%`
	* @param {{ "id": string }} _values
	* @returns {never}
	*/
	function resolve_params_missing(_values) {
		if (verbose) throw_error("resolve_params_missing", `Missing params for dynamic route ID \`${_values.id}\``, resolve_params_missing);
		throw new Error("https://svelte.dev/e/kit/resolve_params_missing");
	}
	/**
	* Missing parameter `%name%` in route `%id%`
	* @param {{ "name": string; "id": string }} _values
	* @returns {never}
	*/
	function route_param_missing(_values) {
		if (verbose) throw_error("route_param_missing", `Missing parameter \`${_values.name}\` in route \`${_values.id}\``, route_param_missing);
		throw new Error("https://svelte.dev/e/kit/route_param_missing");
	}
	/**
	* Parameter `%name%` in route `%id%` cannot start or end with a slash — this would cause an invalid route like `foo/bar`
	* @param {{ "name": string; "id": string }} _values
	* @returns {never}
	*/
	function route_param_slash(_values) {
		if (verbose) throw_error("route_param_slash", `Parameter \`${_values.name}\` in route \`${_values.id}\` cannot start or end with a slash — this would cause an invalid route like \`foo/bar\``, route_param_slash);
		throw new Error("https://svelte.dev/e/kit/route_param_slash");
	}
	/**
	* Parameter `%name%` in route `%id%` must be a string, number, boolean, or bigint
	* @param {{ "name": string; "id": string }} _values
	* @returns {never}
	*/
	function route_param_value_invalid(_values) {
		if (verbose) throw_error("route_param_value_invalid", `Parameter \`${_values.name}\` in route \`${_values.id}\` must be a string, number, boolean, or bigint`, route_param_value_invalid);
		throw new Error("https://svelte.dev/e/kit/route_param_value_invalid");
	}
	new URL("a://");
	/**
	* Decode pathname excluding %25 to prevent further double decoding of params
	* @param {string} pathname
	*/
	function decode_pathname(pathname) {
		return pathname.split("%25").map(decodeURI).join("%25");
	}
	new TextEncoder();
	/**
	* Throws an error with a HTTP status code and an optional message.
	* When called during request handling, this will cause SvelteKit to
	* return an error response; the error will be passed to `handleError` as an _expected_ error.
	* Make sure you're not catching the thrown error, which would prevent SvelteKit from handling it.
	* @param {number} status The [HTTP status code](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses). Must be in the range 400-599.
	* @param {string} [message] The error message.
	* @overload
	* @param {{ status: number; message: string } extends App.Error ? number : never} status
	* @param {{ status: number; message: string } extends App.Error ? string : never} [message]
	* @return {never}
	* @throws {import('./public.js').HttpError} This error instructs SvelteKit to initiate HTTP error handling.
	* @throws {Error} If the provided status is invalid (not between 400 and 599).
	*/
	/**
	* Throws an error with a HTTP status code and an optional message.
	* When called during request handling, this will cause SvelteKit to
	* return an error response; the error will be passed to `handleError` as an _expected_ error.
	* Make sure you're not catching the thrown error, which would prevent SvelteKit from handling it.
	* @param {number} status The [HTTP status code](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses). Must be in the range 400-599.
	* @param {string} message The error message.
	* @param {keyof Omit<App.Error, 'status' | 'message'> extends never ? never : Omit<App.Error, 'status' | 'message'>} properties Additional properties of the App.Error type.
	* @overload
	* @param {number} status
	* @param {string} message
	* @param {keyof Omit<App.Error, 'status' | 'message'> extends never ? never : Omit<App.Error, 'status' | 'message'>} properties
	* @return {never}
	* @throws {import('./public.js').HttpError} This error instructs SvelteKit to initiate HTTP error handling.
	* @throws {Error} If the provided status is invalid (not between 400 and 599).
	*/
	/**
	* Throws an error with a HTTP status code and an optional message.
	* When called during request handling, this will cause SvelteKit to
	* return an error response; the error will be passed to `handleError` as an _expected_ error.
	* Make sure you're not catching the thrown error, which would prevent SvelteKit from handling it.
	* @deprecated Passing an `App.Error` body as the second argument is deprecated — pass the `message` as the second argument, and any additional properties as the third
	* @param {number} status The [HTTP status code](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses). Must be in the range 400-599.
	* @param {Omit<App.Error, 'status'> & { status?: App.Error['status'] }} body An object that conforms to the App.Error type. If a string is passed, it will be used as the message property.
	* @overload
	* @param {number} status
	* @param {Omit<App.Error, 'status'> & { status?: App.Error['status'] }} properties
	* @return {never}
	* @throws {import('./public.js').HttpError} This error instructs SvelteKit to initiate HTTP error handling.
	* @throws {Error} If the provided status is invalid (not between 400 and 599).
	*/
	/**
	* Throws an error with a HTTP status code and an optional message.
	* When called during request handling, this will cause SvelteKit to
	* return an error response; the error will be passed to `handleError` as an _expected_ error.
	* Make sure you're not catching the thrown error, which would prevent SvelteKit from handling it.
	* @param {any} status The [HTTP status code](https://developer.mozilla.org/en-US/docs/Web/HTTP/Status#client_error_responses). Must be in the range 400-599.
	* @param {any} [message] A string, or (deprecated) a partial App.Error object
	* @param {any} [properties] Additional properties of the App.Error type when passing a string message.
	* @return {never}
	* @throws {import('./public.js').HttpError} This error instructs SvelteKit to initiate HTTP error handling.
	* @throws {Error} If the provided status is invalid (not between 400 and 599).
	*/
	function error(status, message, properties) {
		if (isNaN(status) || status < 400 || status > 599) invalid_error_status({ status: String(status) });
		if (message !== void 0 && typeof message !== "string") ({message, ...properties} = message);
		throw new HttpError({
			...properties,
			status,
			message: message ?? `Error: ${status}`
		});
	}
	function _typeof(o) {
		"@babel/helpers - typeof";
		return _typeof = "function" == typeof Symbol && "symbol" == typeof Symbol.iterator ? function(o) {
			return typeof o;
		} : function(o) {
			return o && "function" == typeof Symbol && o.constructor === Symbol && o !== Symbol.prototype ? "symbol" : typeof o;
		}, _typeof(o);
	}
	function toPrimitive(t, r) {
		if ("object" != _typeof(t) || !t) return t;
		var e = t[Symbol.toPrimitive];
		if (void 0 !== e) {
			var i = e.call(t, r || "default");
			if ("object" != _typeof(i)) return i;
			throw new TypeError("@@toPrimitive must return a primitive value.");
		}
		return ("string" === r ? String : Number)(t);
	}
	function toPropertyKey(t) {
		var i = toPrimitive(t, "string");
		return "symbol" == _typeof(i) ? i : i + "";
	}
	function _defineProperty(e, r, t) {
		return (r = toPropertyKey(r)) in e ? Object.defineProperty(e, r, {
			value: t,
			enumerable: !0,
			configurable: !0,
			writable: !0
		}) : e[r] = t, e;
	}
	function ownKeys(e, r) {
		var t = Object.keys(e);
		if (Object.getOwnPropertySymbols) {
			var o = Object.getOwnPropertySymbols(e);
			r && (o = o.filter(function(r) {
				return Object.getOwnPropertyDescriptor(e, r).enumerable;
			})), t.push.apply(t, o);
		}
		return t;
	}
	function _objectSpread2(e) {
		for (var r = 1; r < arguments.length; r++) {
			var t = null != arguments[r] ? arguments[r] : {};
			r % 2 ? ownKeys(Object(t), !0).forEach(function(r) {
				_defineProperty(e, r, t[r]);
			}) : Object.getOwnPropertyDescriptors ? Object.defineProperties(e, Object.getOwnPropertyDescriptors(t)) : ownKeys(Object(t)).forEach(function(r) {
				Object.defineProperty(e, r, Object.getOwnPropertyDescriptor(t, r));
			});
		}
		return e;
	}
	/** @import { RequestEvent } from '@sveltejs/kit' */
	/** @import { MaybePromise, RequestState, RemoteInternals, RequestStore, RemoteLiveQueryUserFunctionReturnType } from 'types' */
	/**
	* @param {any} validate_or_fn
	* @param {((arg?: any) => any) | undefined} [maybe_fn]
	* @returns {(arg?: any) => MaybePromise<any>}
	*/
	function create_validator(validate_or_fn, maybe_fn) {
		if (!maybe_fn) return (arg) => {
			if (arg !== void 0) error(400, "Bad Request");
		};
		if (validate_or_fn === "unchecked") return (arg) => arg;
		if ("~standard" in validate_or_fn) return async (arg) => {
			const result = await validate_or_fn["~standard"].validate(arg);
			if (result.issues) throw new ValidationError(result.issues);
			return result.value;
		};
		remote_invalid_validator();
	}
	/**
	* In case of a single remote function call, just returns the result.
	*
	* In case of a full page reload, returns the response for a remote function call,
	* either from the cache or by invoking the function.
	* Also saves an uneval'ed version of the result for later HTML inlining for hydration.
	*
	* @template {MaybePromise<any>} T
	* @param {RemoteInternals} internals
	* @param {string} payload — the stringified raw argument (i.e. the cache key the client will use)
	* @param {RequestState} state
	* @param {() => Promise<T>} get_result
	* @returns {Promise<T>}
	*/
	async function get_response(internals, payload, state, get_result) {
		var _cache$payload;
		await 0;
		const cache = get_cache(internals, state);
		if (!state.is_in_remote_query) get_implicit_lookup(internals, state)[payload] = get_result;
		return (_cache$payload = cache[payload]) !== null && _cache$payload !== void 0 ? _cache$payload : cache[payload] = get_result();
	}
	/**
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {boolean} allow_cookies
	* @returns {RequestStore}
	*/
	function derive_remote_function_event(event, state, allow_cookies) {
		/** @type {RequestEvent} */
		const derived = _objectSpread2(_objectSpread2({}, event), {}, {
			setHeaders: () => {
				remote_headers_forbidden();
			},
			cookies: _objectSpread2(_objectSpread2({}, event.cookies), {}, {
				set: (name, value, opts) => {
					if (!allow_cookies) remote_cookie_forbidden({ operation: "set" });
					if ((opts === null || opts === void 0 ? void 0 : opts.path) && !opts.path.startsWith("/")) remote_cookie_path_relative({ operation: "set" });
					return event.cookies.set(name, value, opts);
				},
				delete: (name, opts) => {
					if (!allow_cookies) remote_cookie_forbidden({ operation: "delete" });
					if ((opts === null || opts === void 0 ? void 0 : opts.path) && !opts.path.startsWith("/")) remote_cookie_path_relative({ operation: "deleted" });
					return event.cookies.delete(name, opts);
				}
			})
		});
		if (state.is_in_remote_query) for (const property of [
			"url",
			"params",
			"route"
		]) Object.defineProperty(derived, property, {
			enumerable: false,
			get() {
				return remote_request_property({ property });
			}
		});
		return {
			event: derived,
			state: _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_function: true })
		};
	}
	/**
	* Like `with_event` but removes things from `event` you cannot see/call in remote functions, such as `setHeaders`.
	* @template T
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {boolean} allow_cookies
	* @param {() => any} get_input
	* @param {(arg?: any) => T} fn
	*/
	async function run_remote_function(event, state, allow_cookies, get_input, fn) {
		const store = derive_remote_function_event(event, state, allow_cookies);
		const input = await with_request_store(store, get_input);
		return with_request_store(store, () => fn(input));
	}
	/**
	* Like `with_event` but removes things from `event` you cannot see/call in remote functions, such as `setHeaders`.
	* @template T
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {boolean} allow_cookies
	* @param {() => any} get_input
	* @param {(arg?: any) => RemoteLiveQueryUserFunctionReturnType<T>} fn
	* @param {string} name
	*/
	function run_remote_generator(_x, _x2, _x3, _x4, _x5, _x6) {
		return _run_remote_generator.apply(this, arguments);
	}
	function _run_remote_generator() {
		_run_remote_generator = _wrapAsyncGenerator(function* (event, state, allow_cookies, get_input, fn, name) {
			const store = derive_remote_function_event(event, state, allow_cookies);
			const input = yield _awaitAsyncGenerator(with_request_store(store, get_input));
			const iterator = to_iterator(yield _awaitAsyncGenerator(with_request_store(store, () => fn(input))), name);
			let done = false;
			try {
				while (true) {
					const result = yield _awaitAsyncGenerator(with_request_store(store, () => iterator.next()));
					if (result.done) {
						done = true;
						return result.value;
					}
					yield result.value;
				}
			} finally {
				if (!done && typeof iterator.return === "function") yield _awaitAsyncGenerator(with_request_store(store, () => {
					var _iterator$return;
					return (_iterator$return = iterator.return) === null || _iterator$return === void 0 ? void 0 : _iterator$return.call(iterator, void 0);
				}));
			}
		});
		return _run_remote_generator.apply(this, arguments);
	}
	/**
	* @template T
	* @param {Awaited<RemoteLiveQueryUserFunctionReturnType<T>>} source
	* @param {string} name
	* @returns {Iterator<T> | AsyncIterator<T>}
	*/
	function to_iterator(source, name) {
		if ("next" in source && typeof source.next === "function") return source;
		if (Symbol.asyncIterator in source && typeof source[Symbol.asyncIterator] === "function") return source[Symbol.asyncIterator]();
		if (Symbol.iterator in source && typeof source[Symbol.iterator] === "function") return source[Symbol.iterator]();
		remote_query_live_not_iterable({ name });
	}
	/**
	* Note that `state` is deliberately not optional: resources that capture the request
	* state at creation must pass it explicitly, because reading it from the request store
	* at call time is only equivalent on runtimes with `AsyncLocalStorage` support.
	* Callers without a captured state (such as the module-level `form` instance getters)
	* should pass `get_request_store().state` themselves.
	* @param {RemoteInternals} internals
	* @param {RequestState} state
	*/
	function get_cache(internals, state) {
		var _state$remote$data;
		let cache = (_state$remote$data = state.remote.data) === null || _state$remote$data === void 0 ? void 0 : _state$remote$data.get(internals);
		if (cache === void 0) {
			var _state$remote, _state$remote$data2;
			cache = {};
			((_state$remote$data2 = (_state$remote = state.remote).data) !== null && _state$remote$data2 !== void 0 ? _state$remote$data2 : _state$remote.data = /* @__PURE__ */ new Map()).set(internals, cache);
		}
		return cache;
	}
	/**
	* @param {RemoteInternals} internals
	* @param {RequestState} state
	*/
	function get_implicit_lookup(internals, state) {
		var _state$remote$implici;
		let cache = (_state$remote$implici = state.remote.implicit) === null || _state$remote$implici === void 0 ? void 0 : _state$remote$implici.get(internals);
		if (cache === void 0) {
			var _state$remote2, _state$remote2$implic;
			cache = {};
			((_state$remote2$implic = (_state$remote2 = state.remote).implicit) !== null && _state$remote2$implic !== void 0 ? _state$remote2$implic : _state$remote2.implicit = /* @__PURE__ */ new Map()).set(internals, cache);
		}
		return cache;
	}
	/** @import { RemoteCommand } from '$app/server' */
	/** @import { MaybePromise, RemoteCommandInternals } from 'types' */
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	/**
	* Creates a remote command. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#command) for full documentation.
	*
	* @template Output
	* @overload
	* @param {() => MaybePromise<Output>} fn
	* @returns {RemoteCommand<void, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote command. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#command) for full documentation.
	*
	* @template Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(arg: Input) => MaybePromise<Output>} fn
	* @returns {RemoteCommand<Input, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote command. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#command) for full documentation.
	*
	* @template {StandardSchemaV1} Schema
	* @template Output
	* @overload
	* @param {Schema} validate
	* @param {(arg: StandardSchemaV1.InferOutput<Schema>) => MaybePromise<Output>} fn
	* @returns {RemoteCommand<StandardSchemaV1.InferInput<Schema>, Output>}
	* @since 2.27
	*/
	/**
	* @template Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {(arg?: Input) => MaybePromise<Output>} [maybe_fn]
	* @returns {RemoteCommand<Input, Output>}
	* @since 2.27
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function command$1(validate_or_fn, maybe_fn) {
		/** @type {(arg?: Input) => MaybePromise<Output>} */
		const fn = maybe_fn ?? validate_or_fn;
		/** @type {(arg?: any) => MaybePromise<Input>} */
		const validate = create_validator(validate_or_fn, maybe_fn);
		/** @type {RemoteCommandInternals} */
		const __ = {
			type: "command",
			id: "",
			name: ""
		};
		/** @type {RemoteCommand<Input, Output> & { __: RemoteCommandInternals }} */
		const wrapper = (arg) => {
			const { event, state } = get_request_store();
			if (!MUTATIVE_METHODS.includes(event.request.method) || state.is_in_remote_query || state.is_in_remote_prerender) {
				if (state.is_in_remote_query || state.is_in_remote_prerender) remote_command_readonly({ name: __.name });
				remote_command_method({
					name: __.name,
					method: event.request.method
				});
			}
			if (state.is_in_render) remote_command_render({ name: __.name });
			const promise = Promise.resolve(run_remote_function(event, state, true, () => validate(arg), fn));
			promise.updates = () => {
				server_api_unavailable({ name: `${__.name}(...).updates(...)` });
			};
			return promise;
		};
		Object.defineProperty(wrapper, "__", { value: __ });
		Object.defineProperty(wrapper, "pending", { get: () => 0 });
		return wrapper;
	}
	function _asyncIterator(r) {
		var n, t, o, e = 2;
		for ("undefined" != typeof Symbol && (t = Symbol.asyncIterator, o = Symbol.iterator); e--;) {
			if (t && null != (n = r[t])) return n.call(r);
			if (o && null != (n = r[o])) return new AsyncFromSyncIterator(n.call(r));
			t = "@@asyncIterator", o = "@@iterator";
		}
		throw new TypeError("Object is not async iterable");
	}
	function AsyncFromSyncIterator(r) {
		function AsyncFromSyncIteratorContinuation(r) {
			if (Object(r) !== r) return Promise.reject(/* @__PURE__ */ new TypeError(r + " is not an object."));
			var n = r.done;
			return Promise.resolve(r.value).then(function(r) {
				return {
					value: r,
					done: n
				};
			});
		}
		return AsyncFromSyncIterator = function AsyncFromSyncIterator(r) {
			this.s = r, this.n = r.next;
		}, AsyncFromSyncIterator.prototype = {
			s: null,
			n: null,
			next: function next() {
				return AsyncFromSyncIteratorContinuation(this.n.apply(this.s, arguments));
			},
			"return": function _return(r) {
				var n = this.s["return"];
				return void 0 === n ? Promise.resolve({
					value: r,
					done: !0
				}) : AsyncFromSyncIteratorContinuation(n.apply(this.s, arguments));
			},
			"throw": function _throw(r) {
				var n = this.s["return"];
				return void 0 === n ? Promise.reject(r) : AsyncFromSyncIteratorContinuation(n.apply(this.s, arguments));
			}
		}, new AsyncFromSyncIterator(r);
	}
	var text_encoder$1 = new TextEncoder();
	new TextDecoder();
	/**
	* @param {Uint8Array} bytes
	* @returns {string}
	*/
	function base64_encode$1(bytes) {
		if (globalThis.Buffer) return globalThis.Buffer.from(bytes).toString("base64");
		let binary = "";
		for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
		return btoa(binary);
	}
	/** @import { BinaryFormMeta, InternalRemoteFormIssue } from 'types' */
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	/**
	* Separates a form field's path from the metadata encoded in its name.
	* @param {string} form_id
	* @param {string} key
	* @returns {{ name: string; type: 'number' | 'boolean' | null; is_array: boolean }}
	*/
	function parse_form_key(form_id, key) {
		const suffix = "/" + form_id;
		let name = key;
		let image_coordinate = "";
		if (name.startsWith("i:") && (name.endsWith(suffix + ".x") || name.endsWith(suffix + ".y"))) {
			image_coordinate = name[name.length - 1];
			name = name.slice(0, -2);
		}
		if (!name.endsWith(suffix)) form_field_unbound({ name });
		name = name.slice(0, -suffix.length);
		/** @type {'number' | 'boolean' | null} */
		let type = null;
		if (name.startsWith("n:")) {
			name = name.slice(2);
			type = "number";
		} else if (name.startsWith("b:")) {
			name = name.slice(2);
			type = "boolean";
		} else if (name.startsWith("i:")) {
			name = name.slice(2);
			type = "number";
		}
		const is_array = name.endsWith("[]");
		if (is_array) name = name.slice(0, -2);
		if (image_coordinate) name += "." + image_coordinate;
		return {
			name,
			type,
			is_array
		};
	}
	/** Pass this to set_nested_value to delete the last part of the given path */
	var DELETE_KEY = {};
	var path_regex = /^[a-zA-Z_$]\w*(\.[a-zA-Z_$]\w*|\[\d+\])*$/;
	/**
	* @param {string} path
	*/
	function split_path(path) {
		if (!path_regex.test(path)) form_field_invalid_name({ name: path });
		return path.split(/\.|\[|\]/).filter(Boolean);
	}
	/**
	* Check if a property key is dangerous and could lead to prototype pollution
	* @param {string} key
	*/
	function check_prototype_pollution(key) {
		if (key === "__proto__" || key === "constructor" || key === "prototype") form_field_forbidden_key({ key });
	}
	/**
	* Sets a value in a nested object using an array of keys, mutating the original object.
	* @param {Record<string, any>} object
	* @param {string[]} keys
	* @param {any} value
	*/
	function deep_set(object, keys, value) {
		let current = object;
		for (let i = 0; i < keys.length - 1; i += 1) {
			const key = keys[i];
			check_prototype_pollution(key);
			const is_array = /^\d+$/.test(keys[i + 1]);
			const inner = Object.hasOwn(current, key) ? current[key] : void 0;
			const exists = inner != null;
			if (exists && is_array !== Array.isArray(inner)) form_field_array_conflict({ key: keys[i + 1] });
			if (!exists) {
				if (value === DELETE_KEY) return;
				current[key] = is_array ? [] : {};
			}
			current = current[key];
		}
		const final_key = keys[keys.length - 1];
		check_prototype_pollution(final_key);
		if (value === DELETE_KEY) delete current[final_key];
		else current[final_key] = value;
	}
	/**
	* @param {StandardSchemaV1.Issue} issue
	* @param {boolean} server Whether this issue came from server validation
	*/
	function normalize_issue(issue, server = false) {
		/** @type {InternalRemoteFormIssue} */
		const normalized = {
			name: "",
			path: [],
			message: issue.message,
			server
		};
		if (issue.path !== void 0) {
			let name = "";
			for (const segment of issue.path) {
				const key = typeof segment === "object" ? segment.key : segment;
				normalized.path.push(key);
				if (typeof key === "number") name += `[${key}]`;
				else if (typeof key === "string") name += name === "" ? key : "." + key;
			}
			normalized.name = name;
		}
		return normalized;
	}
	/**
	* @param {InternalRemoteFormIssue[]} issues
	*/
	function flatten_issues(issues) {
		/** @type {Record<string, InternalRemoteFormIssue[]>} */
		const result = Object.create(null);
		for (const issue of issues) {
			var _result$$;
			((_result$$ = result.$) !== null && _result$$ !== void 0 ? _result$$ : result.$ = []).push(issue);
			let name = "";
			if (issue.path !== void 0) for (const key of issue.path) {
				var _name, _result$_name;
				if (typeof key === "number") name += `[${key}]`;
				else if (typeof key === "string") name += name === "" ? key : "." + key;
				((_result$_name = result[_name = name]) !== null && _result$_name !== void 0 ? _result$_name : result[_name] = []).push(issue);
			}
		}
		return result;
	}
	/**
	* Gets a nested value from an object using a path array
	* @param {Record<string, any>} object
	* @param {(string | number)[]} path
	* @returns {any}
	*/
	function deep_get(object, path) {
		let current = object;
		for (const key of path) {
			if (current === null || typeof current !== "object" || !Object.hasOwn(current, key)) return;
			current = current[key];
		}
		return current;
	}
	/** name prefixes that tell the server which type to coerce a submitted string to */
	var type_prefixes = {
		number: "n:",
		boolean: "b:"
	};
	/**
	* adds props; a function becomes a getter that is computed each time it is read
	* @param {Record<string, any>} base_props
	* @param {Record<string, unknown>} props
	*/
	function add_props(base_props, props) {
		for (const prop in props) {
			const value = props[prop];
			if (typeof value === "function") Object.defineProperty(base_props, prop, {
				enumerable: true,
				get: value
			});
			else base_props[prop] = value;
		}
		return base_props;
	}
	/**
	* @param {string} type
	* @param {boolean} is_array
	* @param {unknown} input_value
	*/
	function get_type_prefix(type, is_array, input_value) {
		var _type_prefixes$input_;
		if (type === "number" || type === "range") return "n:";
		if (type === "image") return "i:";
		if (type === "checkbox" && !is_array) return "b:";
		if (type === "hidden" || type === "submit") return (_type_prefixes$input_ = type_prefixes[typeof input_value]) !== null && _type_prefixes$input_ !== void 0 ? _type_prefixes$input_ : "";
		return "";
	}
	/**
	* A deep-clone implementation specifically for form data, where
	* we don't need to worry about cycles and whatnot
	* @param {any} value
	* @returns {any}
	*/
	function deep_clone(value) {
		if (value !== null && typeof value === "object") {
			if (value instanceof Date) return new Date(value.getTime());
			if (value instanceof File) return value;
			if (Array.isArray(value)) return value.map(deep_clone);
			/** @type {Record<string, any>} */
			const clone = {};
			for (const key of Object.keys(value)) clone[key] = deep_clone(value[key]);
			return clone;
		}
		return value;
	}
	/** @type {ProxyHandler<object> | null} */
	var dev_traps = null;
	/** @param {InternalRemoteFormIssue} issue */
	var public_issue = (issue) => ({
		path: issue.path,
		message: issue.message
	});
	/** @typedef {{
	* 	form_id: string,
	* 	get: () => Record<string, any>,
	* 	set: (path: (string | number)[], value: any) => void,
	* 	get_issues: (path?: (string | number)[], all?: boolean) => Record<string, InternalRemoteFormIssue[]>,
	* 	get_touched: () => Record<string, boolean>,
	* 	get_dirty: () => Record<string, boolean>
	* }} FieldContext */
	/**
	* @param {FieldContext} context
	* @param {(string | number)[]} path
	* @param {string} prop
	* @returns {any} a method of the field at `path`, or undefined for a nested field
	*/
	function create_field_method(context, path, prop) {
		switch (prop) {
			case "set": return (value) => {
				context.set(path, value);
				return value;
			};
			case "value": return () => deep_clone(deep_get(context.get(), path));
			case "issues":
			case "allIssues": {
				const key = build_path_string(path);
				const all = prop === "allIssues";
				return () => {
					const issues = context.get_issues(path, all)[key === "" ? "$" : key];
					if (all) return issues === null || issues === void 0 ? void 0 : issues.map(public_issue);
					const own = issues === null || issues === void 0 ? void 0 : issues.filter((issue) => issue.name === key).map(public_issue);
					return (own === null || own === void 0 ? void 0 : own.length) ? own : void 0;
				};
			}
			case "touched":
			case "dirty": {
				const key = build_path_string(path);
				return () => {
					const object = prop === "dirty" ? context.get_dirty() : context.get_touched();
					if (Object.hasOwn(object, key)) return true;
					for (const candidate in object) {
						if (!Object.hasOwn(object, candidate)) continue;
						if (key === "") return true;
						if (!candidate.startsWith(key)) continue;
						const next = candidate[key.length];
						if (next === "." || next === "[") return true;
					}
					return false;
				};
			}
			case "as": {
				const key = build_path_string(path);
				/**
				* the field's value, or `fallback` until the field has been edited
				* (without a fallback there is nothing to suppress, so `dirty` is not read)
				* @param {unknown} [fallback]
				*/
				const read = (fallback) => {
					var _deep_get;
					return (_deep_get = deep_get(context.get(), path)) !== null && _deep_get !== void 0 ? _deep_get : fallback !== void 0 && Object.hasOwn(context.get_dirty(), key) ? void 0 : fallback;
				};
				/**
				* @param {string} type
				* @param {unknown} [input_value]
				* @param {boolean} [checked]
				*/
				return (type, input_value, checked) => {
					const is_array = type === "file multiple" || type === "select multiple" || type === "checkbox" && typeof input_value === "string";
					/** @type {Record<string, any>} */
					const base_props = {
						name: get_type_prefix(type, is_array, input_value) + key + (is_array ? "[]" : "") + "/" + context.form_id,
						get "aria-invalid"() {
							const issues = context.get_issues();
							return key in issues ? "true" : void 0;
						}
					};
					if (type !== "text" && type !== "select" && type !== "select multiple") base_props.type = type === "file multiple" ? "file" : type;
					if (type === "submit" || type === "hidden") return add_props(base_props, { value: typeof input_value === "boolean" ? input_value ? "on" : "off" : input_value });
					if (type === "select" || type === "select multiple") return add_props(base_props, {
						multiple: is_array,
						value: () => {
							const value = read(input_value);
							return Array.isArray(value) ? [...value] : value;
						}
					});
					if (type === "checkbox" || type === "radio") {
						if (type === "radio" || is_array) base_props.value = input_value !== null && input_value !== void 0 ? input_value : "on";
						else checked = input_value;
						return add_props(base_props, {
							defaultChecked: checked,
							checked: () => {
								const value = read();
								if (value == null) return read(checked);
								if (type === "radio") return value === input_value;
								if (is_array) return value.includes(input_value);
								return value;
							}
						});
					}
					if (type === "file" || type === "file multiple") return add_props(base_props, {
						multiple: is_array,
						files: () => {
							const value = read();
							const files = value instanceof File ? [value] : value;
							if (!Array.isArray(files) || !files.every((f) => f instanceof File)) return null;
							if (typeof DataTransfer === "undefined") return Object.assign({ length: files.length }, files);
							const transfer = new DataTransfer();
							for (const file of files) transfer.items.add(file);
							return transfer.files;
						}
					});
					if (type === "image") return base_props;
					return add_props(base_props, {
						defaultValue: input_value,
						value: () => {
							var _read;
							return String((_read = read(input_value)) !== null && _read !== void 0 ? _read : "");
						}
					});
				};
			}
		}
	}
	/**
	* Creates a proxy-based field accessor for form data
	* @param {FieldContext} context
	* @param {any} target - Function or empty POJO
	* @param {(string | number)[]} path - Current access path
	* @returns {any} Proxy object with name(), value(), and issues() methods
	*/
	function create_field_proxy(context, target = {}, path = []) {
		return new Proxy(target, _objectSpread2(_objectSpread2({}, dev_traps), {}, { get(target, prop) {
			if (typeof prop === "symbol") return target[prop];
			const next = [...path, /^\d+$/.test(prop) ? parseInt(prop, 10) : prop];
			return create_field_proxy(context, create_field_method(context, path, prop), next);
		} }));
	}
	/**
	* Builds a path string from an array of path segments
	* @param {(string | number)[]} path
	* @returns {string}
	*/
	function build_path_string(path) {
		let result = "";
		for (const segment of path) if (typeof segment === "number") result += `[${segment}]`;
		else result += result === "" ? segment : "." + segment;
		return result;
	}
	/** @import { RemoteFormInput, RemoteForm, RemoteFormInvalidField } from '$app/server' */
	/** @import { InternalRemoteFormIssue, MaybePromise, HasNonOptionalBoolean, RemoteFormInternals } from 'types' */
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	/**
	* Creates a form object that can be spread onto a `<form>` element.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#form) for full documentation.
	*
	* @template Output
	* @overload
	* @param {() => MaybePromise<Output>} fn
	* @returns {RemoteForm<void, Output>}
	* @since 2.27
	*/
	/**
	* Creates a form object that can be spread onto a `<form>` element.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#form) for full documentation.
	*
	* @template {RemoteFormInput} Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(data: Input, issue: RemoteFormInvalidField<Input>) => MaybePromise<Output>} fn
	* @returns {RemoteForm<Input, Output>}
	* @since 2.27
	*/
	/**
	* Creates a form object that can be spread onto a `<form>` element.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#form) for full documentation.
	*
	* @template {StandardSchemaV1<RemoteFormInput, Record<string, any>>} Schema
	* @template Output
	* @overload
	* @param {true extends HasNonOptionalBoolean<StandardSchemaV1.InferInput<Schema>> ? 'Error: All booleans in form schemas must be optional (e.g. `v.optional(v.boolean(), false)`) because checkbox inputs do not send a false value when unchecked.' : Schema} validate
	* @param {(data: StandardSchemaV1.InferOutput<Schema>, issue: RemoteFormInvalidField<StandardSchemaV1.InferInput<Schema>>) => MaybePromise<Output>} fn
	* @returns {RemoteForm<StandardSchemaV1.InferInput<Schema>, Output>}
	* @since 2.27
	*/
	/**
	* @template {RemoteFormInput} Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {(data_or_issue: any, issue?: any) => MaybePromise<Output>} [maybe_fn]
	* @returns {RemoteForm<Input, Output>}
	* @since 2.27
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function form$1(validate_or_fn, maybe_fn) {
		/** @type {any} */
		const fn = maybe_fn ?? validate_or_fn;
		/** @type {StandardSchemaV1 | null} */
		const schema = !maybe_fn || validate_or_fn === "unchecked" ? null : validate_or_fn;
		/**
		* @param {string | number | boolean} [key]
		*/
		function create_instance(key) {
			const instance = {};
			instance.method = "POST";
			Object.defineProperty(instance, "enhance", { value: () => {
				return {
					action: instance.action,
					method: instance.method
				};
			} });
			/** @type {RemoteFormInternals} */
			const __ = {
				type: "form",
				name: "",
				id: "",
				fn: async (data, meta, form_data) => {
					const output = {};
					output.submission = true;
					const { event, state } = get_request_store();
					const validated = await schema?.["~standard"].validate(data);
					if (meta.validate_only) return validated?.issues?.map((issue) => normalize_issue(issue, true)) ?? [];
					if (validated?.issues !== void 0) handle_issues(output, validated.issues, form_data, __.id);
					else {
						if (validated !== void 0) data = validated.value;
						const issue = create_issues();
						try {
							output.result = await run_remote_function(event, state, true, () => data, (data) => !maybe_fn ? fn() : fn(data, issue));
						} catch (error) {
							if (error instanceof ValidationError) handle_issues(output, error.issues, form_data, __.id);
							else throw error;
						}
					}
					if (!event.isRemoteRequest) {
						const cache = get_cache(__, state);
						cache[""] ??= output;
						get_implicit_lookup(__, state)[__.key ? `${__.id}/${__.key}` : __.id] = () => cache[""];
					}
					return output;
				}
			};
			Object.defineProperty(instance, "__", { value: __ });
			Object.defineProperty(instance, "action", {
				get: () => {
					const { event, state } = get_request_store();
					const search = new URLSearchParams(state.prerendering ? "" : event.url.search);
					search.delete("/remote");
					const query = search.toString();
					const action_id = __.key ? `${__.id}/${encodeURIComponent(__.key)}` : __.id;
					return `?${query ? `${query}&` : ""}/remote=${action_id}`;
				},
				enumerable: true
			});
			Object.defineProperty(instance, "fields", { get() {
				return create_field_proxy({
					form_id: __.id,
					get: () => get_cache(__, get_request_store().state)?.[""]?.input ?? {},
					set: (path, value) => {
						const cache = get_cache(__, get_request_store().state);
						const entry = cache[""];
						if (entry?.submission) return;
						if (path.length === 0) {
							(cache[""] ??= {}).input = value;
							return;
						}
						const input = entry?.input ?? {};
						deep_set(input, path.map(String), value);
						(cache[""] ??= {}).input = input;
					},
					get_issues: () => flatten_issues(get_cache(__, get_request_store().state)?.[""]?.issues ?? []),
					get_touched: () => ({}),
					get_dirty: () => ({})
				});
			} });
			Object.defineProperty(instance, "result", { get() {
				try {
					return get_cache(__, get_request_store().state)?.[""]?.result;
				} catch {
					return;
				}
			} });
			Object.defineProperty(instance, "pending", { get: () => 0 });
			Object.defineProperty(instance, "submitted", { get: () => false });
			Object.defineProperty(instance, "preflight", { value: () => instance });
			Object.defineProperty(instance, "validate", { value: () => {
				server_api_unavailable({ name: "form.validate()" });
			} });
			Object.defineProperty(instance, "submit", { value: () => {
				server_api_unavailable({ name: "form.submit()" });
			} });
			Object.defineProperty(instance, "element", { get: () => null });
			if (key == void 0) Object.defineProperty(instance, "for", { 
			/** @type {RemoteForm<any, any>['for']} */
value: (key) => {
				const { state } = get_request_store();
				const cache_key = __.id + "|" + JSON.stringify(key);
				/** @type {RemoteForm<Input, Output> & { __: RemoteFormInternals }} */
				let instance = (state.remote.forms ??= /* @__PURE__ */ new Map()).get(cache_key);
				if (!instance) {
					instance = create_instance(key);
					instance.__.id = __.id;
					instance.__.key = JSON.stringify(key);
					instance.__.name = __.name;
					state.remote.forms.set(cache_key, instance);
				}
				return instance;
			} });
			return instance;
		}
		return create_instance();
	}
	/**
	* @param {{ issues?: InternalRemoteFormIssue[], input?: Record<string, any>, result: any }} output
	* @param {readonly StandardSchemaV1.Issue[]} issues
	* @param {FormData | null} form_data - null if the form is progressively enhanced
	* @param {string} form_id - hash/name of the form
	*/
	function handle_issues(output, issues, form_data, form_id) {
		output.issues = issues.map((issue) => normalize_issue(issue, true));
		if (form_data) {
			output.input = {};
			for (const field_name of form_data.keys()) {
				const field = parse_form_key(form_id, field_name);
				const path = split_path(field.name);
				if (path.some((part) => part.startsWith("_"))) continue;
				const values = form_data.getAll(field_name).filter((value) => typeof value === "string");
				deep_set(output.input, path, field.is_array ? values : values[0]);
			}
		}
	}
	/**
	* Creates an invalid function that can be used to imperatively mark form fields as invalid
	* @returns {RemoteFormInvalidField<any>}
	*/
	function create_issues() {
		return new Proxy(
			/** @param {string} message */
			(message) => {
				return create_issue(message);
			},
			{ get(target, prop) {
				if (typeof prop === "symbol") return target[prop];
				return create_issue_proxy(prop, []);
			} }
		);
		/**
		* @param {string} message
		* @param {(string | number)[]} path
		* @returns {StandardSchemaV1.Issue}
		*/
		function create_issue(message, path = []) {
			return {
				message,
				path
			};
		}
		/**
		* Creates a proxy that builds up a path and returns a function to create an issue
		* @param {string | number} key
		* @param {(string | number)[]} path
		*/
		function create_issue_proxy(key, path) {
			const new_path = [...path, key];
			/**
			* @param {string} message
			* @returns {StandardSchemaV1.Issue}
			*/
			const issue_func = (message) => create_issue(message, new_path);
			return new Proxy(issue_func, { get(target, prop) {
				if (typeof prop === "symbol") return target[prop];
				if (/^\d+$/.test(prop)) return create_issue_proxy(parseInt(prop, 10), new_path);
				return create_issue_proxy(prop, new_path);
			} });
		}
	}
	/**
	* Same-origin urls are keyed by path, so prerendered pages can be served from any origin
	* @param {URL} url
	* @param {{ origin: string }} page
	*/
	function fetch_cache_url(url, page) {
		return url.origin === page.origin ? url.href.slice(page.origin.length) : url.href;
	}
	var object_proto_names = /* @__PURE__ */ Object.getOwnPropertyNames(Object.prototype).sort().join("\0");
	/**
	* @param {unknown} thing
	* @returns {thing is Record<PropertyKey, unknown>}
	*/
	function is_plain_object(thing) {
		if (typeof thing !== "object" || thing === null) return false;
		const proto = Object.getPrototypeOf(thing);
		return proto === Object.prototype || proto === null || Object.getPrototypeOf(proto) === null || Object.getOwnPropertyNames(proto).sort().join("\0") === object_proto_names;
	}
	/**
	* @param {Record<string, any>} value
	* @param {Map<object, any>} clones
	*/
	function to_sorted(value, clones) {
		const clone = Object.getPrototypeOf(value) === null ? Object.create(null) : {};
		clones.set(value, clone);
		Object.defineProperty(clone, remote_arg_marker, { value: true });
		for (const key of Object.keys(value).sort()) {
			const property = value[key];
			Object.defineProperty(clone, key, {
				value: clones.get(property) ?? property,
				enumerable: true,
				configurable: true,
				writable: true
			});
		}
		return clone;
	}
	var remote_object = "__skrao";
	var remote_map = "__skram";
	var remote_set = "__skras";
	var remote_regex_guard = "__skrag";
	var remote_arg_marker = Symbol(remote_object);
	/**
	* @param {boolean} sort
	*/
	function create_remote_arg_reducers(sort) {
		/** @type {Record<string, (value: unknown) => unknown>} */
		const remote_fns_reducers = { 
		/** @param {unknown} value */
[remote_regex_guard]: (value) => {
			if (value instanceof RegExp) remote_argument_unsupported({ type: "Regular expressions" });
		} };
		if (sort) {
			const clones = /* @__PURE__ */ new Map();
			/** @type {(value: unknown) => Array<[unknown, unknown]> | undefined} */
			remote_fns_reducers[remote_map] = (value) => {
				if (!(value instanceof Map)) return;
				/** @type {Array<[string, string]>} */
				const entries = [];
				for (const [key, val] of value) entries.push([stringify(key), stringify(val)]);
				return entries.sort(([a1, a2], [b1, b2]) => {
					if (a1 < b1) return -1;
					if (a1 > b1) return 1;
					if (a2 < b2) return -1;
					if (a2 > b2) return 1;
					return 0;
				});
			};
			/** @type {(value: unknown) => unknown[] | undefined} */
			remote_fns_reducers[remote_set] = (value) => {
				if (!(value instanceof Set)) return;
				/** @type {string[]} */
				const items = [];
				for (const item of value) items.push(stringify(item));
				items.sort();
				return items;
			};
			/** @type {(value: unknown) => Record<PropertyKey, unknown> | undefined} */
			remote_fns_reducers[remote_object] = (value) => {
				if (!is_plain_object(value)) return;
				if (Object.hasOwn(value, remote_arg_marker)) return;
				if (clones.has(value)) return clones.get(value);
				return to_sorted(value, clones);
			};
		}
		const all_reducers = {
			...encoders,
			...remote_fns_reducers
		};
		/** @type {(value: unknown) => string} */
		const stringify = (value) => stringify$1$1(value, all_reducers);
		return all_reducers;
	}
	/**
	* Stringifies the argument (if any) for a remote function in such a way that
	* it is both a valid URL and a valid file name (necessary for prerendering).
	* @param {any} value
	*/
	function stringify_remote_arg(value) {
		if (value === void 0) return "";
		return url_friendly_base64_encode(stringify$1$1(value, create_remote_arg_reducers(true)));
	}
	/**
	* Base64-encodes `string` in such a way that the result is safe to use
	* as both a URI component and a filename
	* @param {string} string
	*/
	function url_friendly_base64_encode(string) {
		return base64_encode$1(text_encoder$1.encode(string)).replaceAll("=", "").replaceAll("+", "-").replaceAll("/", "_");
	}
	/**
	* @param {string} id
	* @param {string} payload
	*/
	function create_remote_key(id, payload) {
		return id + "/" + payload;
	}
	var assets = "";
	var app_dir = "_app";
	/** @import { ParamMatcher, ParamValue } from '@sveltejs/kit/params' */
	var escape_sequence_pattern = /\[([ux])\+([^\]]+)\]/;
	/**
	* Decodes the codepoints of an `[x+nn]` or `[u+nnnn]` escape sequence
	* @param {string} code the sequence without its `[x+`/`[u+` prefix or `]` suffix
	*/
	function decode_escape_sequence(code) {
		return String.fromCodePoint(...code.split("-").map((codepoint) => parseInt(codepoint, 16)));
	}
	/**
	* Encodes the characters that `decode_pathname` leaves untouched, so that a decoded
	* escape sequence still matches the pattern `parse_route_id` builds for it
	* @param {string} str
	*/
	function encode_pathname_chars(str) {
		return str.replace(/[%/?#]/g, (char) => "%" + char.charCodeAt(0).toString(16).toUpperCase().padStart(2, "0"));
	}
	/**
	* Returns `false` for `(group)` segments
	* @param {string} segment
	*/
	function affects_path(segment) {
		return segment !== "" && !/^\([^)]+\)$/.test(segment);
	}
	/**
	* Splits a route id into its segments, removing segments that
	* don't affect the path (i.e. groups). The root route is represented by `/`
	* and will be returned as `['']`.
	* @param {string} route
	* @returns string[]
	*/
	function get_route_segments(route) {
		return route.slice(1).split("/").filter(affects_path);
	}
	var segment_pattern = new RegExp(`${escape_sequence_pattern.source}|${/\[(\[)?(\.\.\.)?([\w-]+?)(?:=([\w-]+))?\]\]?/g.source}`, "g");
	/**
	* Populate a route ID with params to resolve a pathname.
	* @example
	* ```js
	* resolveRoute(
	*   `/blog/[slug]/[...somethingElse]`,
	*   {
	*     slug: 'hello-world',
	*     somethingElse: 'something/else'
	*   }
	* ); // `/blog/hello-world/something/else`
	* ```
	* @param {string} id
	* @param {Record<string, ParamValue | undefined>} params
	* @returns {string}
	*/
	function resolve_route(id, params) {
		const segments = get_route_segments(id);
		const has_id_trailing_slash = id != "/" && id.endsWith("/");
		return "/" + segments.map((segment) => segment.replace(segment_pattern, (_, escape_type, escape_code, optional, rest, name) => {
			if (escape_type) return encode_pathname_chars(decode_escape_sequence(escape_code));
			const value = params[name];
			if (value === void 0 || value === "") {
				if (optional) return "";
				if (rest && value !== void 0) return "";
				route_param_missing({
					name,
					id
				});
			}
			if (typeof value === "string") {
				if (value.startsWith("/") || value.endsWith("/")) route_param_slash({
					name,
					id
				});
				return value;
			}
			if (typeof value === "number" || typeof value === "boolean" || typeof value === "bigint") return String(value);
			route_param_value_invalid({
				name,
				id
			});
		})).filter(Boolean).join("/") + (has_id_trailing_slash ? "/" : "");
	}
	var symbol = Symbol.for("sveltekit.global_state");
	globalThis[symbol] ??= {};
	/**
	* @param {Error} error
	* @param {string} stack
	* @returns {string | undefined}
	*/
	function set_error_stack(error, stack) {
		try {
			Reflect.set(error, "stack", stack);
		} catch {}
		return error.stack;
	}
	/**
	* @param {unknown} err
	* @return {Error}
	*/
	function coalesce_to_error(err) {
		return err instanceof Error || err && err.name && err.message ? err : new Error(JSON.stringify(err));
	}
	/**
	* @param {import('@sveltejs/kit').RequestEvent} event
	* @param {import('types').RequestState} state
	* @param {any} error
	* @returns {App.Error | Promise<App.Error>}
	*/
	function handle_error_and_jsonify(event, state, error) {
		if (error instanceof HandledHttpError) return error.body;
		/** @type {import('@sveltejs/kit/hooks').CaughtError} */
		let caught;
		if (error instanceof HttpError) caught = {
			kind: "app",
			error: error.body
		};
		else if (error instanceof SvelteKitError) caught = {
			kind: "framework",
			error: {
				status: error.status,
				message: error.text
			}
		};
		else if (error instanceof ValidationError) caught = {
			kind: "validation",
			error: {
				status: 400,
				message: "Bad Request"
			},
			issues: error.issues
		};
		else {
			caught = {
				kind: "unknown",
				error
			};
			let e = error;
			while (e instanceof Error) {
				fix_stack_trace(e);
				e = e.cause;
			}
		}
		const fallback = caught.kind === "unknown" ? {
			status: 500,
			message: "Internal Error"
		} : caught.error;
		/**
		* The hook returns only the properties it wants to override; anything it omits
		* (including by returning nothing at all) is inherited from the caught error.
		* @param {Awaited<ReturnType<import('@sveltejs/kit/hooks').HandleServerError>>} body
		* @returns {App.Error}
		*/
		function merge(body) {
			return {
				...fallback,
				...body
			};
		}
		let result;
		try {
			const input = {
				...caught,
				event
			};
			result = with_request_store({
				event,
				state
			}, () => hooks.handleError(input));
		} catch (hook_error) {
			log_handle_error_hook_failure(error, hook_error);
			return {
				status: fallback.status,
				message: "Internal Error"
			};
		}
		if (result instanceof Promise) return result.then(merge, (hook_error) => {
			log_handle_error_hook_failure(error, hook_error);
			return {
				status: fallback.status,
				message: "Internal Error"
			};
		});
		return merge(result);
	}
	/**
	* @param {unknown} error
	* @param {unknown} hook_error
	*/
	function log_handle_error_hook_failure(error, hook_error) {
		const failure = capture_error(() => handle_error_hook_failed(void 0, { cause: coalesce_to_error(hook_error) }));
		failure.stack = `${failure.name}: ${failure.message}`;
		console.error(failure);
		if (error instanceof SvelteKitError) console.error(`Original error: ${error.status} ${error.text}: ${error.message}`);
		else console.error("Original error:", error);
	}
	var fs = globalThis.process?.getBuiltinModule?.("node:fs");
	var url = globalThis.process?.getBuiltinModule?.("node:url");
	var path = globalThis.process?.getBuiltinModule?.("node:path");
	var module = globalThis.process?.getBuiltinModule?.("node:module");
	var cwd = globalThis.process?.cwd?.();
	/** @type {(file: string) => string} */
	var relative = cwd ? (file) => path.relative(cwd, file) : (file) => file;
	/**
	* Applies sourcemaps, makes paths relative to the cwd, and truncates
	* non-user code from the bottom of the stack
	* @param {Error} error
	* @returns void
	*/
	var fix_stack_trace = (error) => {
		if (!error.stack || !fs) return;
		let end = 0;
		set_error_stack(error, error.stack.split("\n").map((line, i) => {
			const match = line.match(/^ {4}at.+(file:\/\/\/.*):(\d+):(\d+)(\)?)$/);
			if (!match) {
				if (!line.includes("node:internal/")) end = i + 1;
				return line;
			}
			const file = url.fileURLToPath(match[1]);
			const traced = trace(file, Number(match[2]) - 1, Number(match[3]) - 1);
			if (!/[\\/]node_modules[\\/]/.test(traced?.file ?? file)) end = i + 1;
			if (traced?.line) {
				const location = `${match[1]}:${match[2]}:${match[3]}`;
				const original = `${relative(traced.file)}:${traced.line}:${traced.column}`;
				return line.replace(location, original);
			}
			if (traced) return `${line.replace(match[1], relative(file))} [${traced.file}]`;
			return line;
		}).slice(0, end).join("\n"));
	};
	/** @type {Map<string, { map: import('node:module').SourceMap; directory: string } | null>} */
	var source_maps = /* @__PURE__ */ new Map();
	/** @type {Map<string, Array<string | undefined>>} */
	var source_regions = /* @__PURE__ */ new Map();
	/** @param {string} file */
	function get_source_map(file) {
		if (source_maps.has(file)) return source_maps.get(file);
		try {
			let source;
			let directory = path.dirname(file);
			const code = fs.readFileSync(file, "utf8");
			const url = Array.from(code.matchAll(/\/\/[#@]\s*sourceMappingURL=(\S+)/g)).at(-1)?.[1];
			if (url?.startsWith("data:")) {
				const comma = url.indexOf(",");
				const metadata = url.slice(5, comma);
				const data = url.slice(comma + 1);
				source = metadata.endsWith(";base64") ? Buffer.from(data, "base64").toString() : decodeURIComponent(data);
			} else {
				const map_file = url ? path.resolve(path.dirname(file), decodeURIComponent(url)) : `${file}.map`;
				if (fs.existsSync(map_file)) {
					directory = path.dirname(map_file);
					source = fs.readFileSync(map_file, "utf8");
				}
			}
			if (source) {
				const source_map = {
					map: new module.SourceMap(JSON.parse(source)),
					directory
				};
				source_maps.set(file, source_map);
				return source_map;
			}
		} catch {}
		source_maps.set(file, null);
		return null;
	}
	/**
	*
	* @param {string} file
	* @param {number} line
	* @param {number} column
	* @returns {null | { file: string, line?: number, column?: number }}
	*/
	function trace(file, line, column) {
		const source_map = get_source_map(file);
		if (!source_map) return null;
		const entry = source_map.map.findEntry(line, column);
		if (entry && "originalSource" in entry && entry.originalSource && typeof entry.originalLine === "number" && typeof entry.originalColumn === "number") {
			const traced = {
				file: entry.originalSource.startsWith("file:") ? url.fileURLToPath(entry.originalSource) : path.resolve(source_map.directory, entry.originalSource),
				line: entry.originalLine + 1,
				column: entry.originalColumn + 1
			};
			return trace(traced.file, traced.line - 1, traced.column - 1) ?? traced;
		}
		let regions = source_regions.get(file);
		if (!regions) {
			/** @type {string | undefined} */
			let source;
			regions = fs.readFileSync(file, "utf8").split("\n").map((line) => {
				const start = line.match(/^\/\/#region (.+)$/);
				if (start) source = start[1];
				if (line === "//#endregion") source = void 0;
				return source;
			});
			source_regions.set(file, regions);
		}
		const source = regions[line];
		if (source) return { file: source };
		return null;
	}
	var _globalThis$process;
	var _globalThis$process$g2;
	var _ref3;
	(_globalThis$process = globalThis.process) === null || _globalThis$process === void 0 || (_globalThis$process$g2 = _globalThis$process.getBuiltinModule) === null || _globalThis$process$g2 === void 0 || (_globalThis$process$g2 = _globalThis$process$g2.call(_globalThis$process, "node:util")) === null || _globalThis$process$g2 === void 0 || _globalThis$process$g2.styleText;
	var hooks = (_ref3 = false) !== null && _ref3 !== void 0 ? _ref3 : null;
	/** @type {typeof import('./client.js').asset} */
	function asset(file) {
		if (file[0] === "/") file = file.slice(1);
		return assets !== "" ? `${assets}/${file}` : resolve(file);
	}
	/** @type {typeof import('./client.js').resolve} */
	function resolve(id, params) {
		let resolved;
		if (id[0] === "/") {
			if (id.includes("[") && !params) resolve_params_missing({ id });
			resolved = resolve_route(id, params ?? {});
		} else resolved = "/" + id;
		{
			const store = try_get_request_store();
			if (store && !store.state.prerendering?.fallback) return ((store.event.isDataRequest ? add_data_suffix(store.event.url.pathname) : store.event.url.pathname).slice(0).split("/").slice(2).map(() => "..").join("/") || ".") + resolved;
		}
		return "" + resolved;
	}
	/** @import { RemoteResource, RemotePrerenderFunction } from '$app/server' */
	/** @import { RemoteFunctionResponse, RemotePrerenderInputsGenerator, RemotePrerenderInternals, MaybePromise } from 'types' */
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	/**
	* Creates a remote prerender function. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#prerender) for full documentation.
	*
	* @template Output
	* @overload
	* @param {() => MaybePromise<Output>} fn
	* @param {{ inputs?: RemotePrerenderInputsGenerator<void>, dynamic?: boolean }} [options]
	* @returns {RemotePrerenderFunction<void, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote prerender function. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#prerender) for full documentation.
	*
	* @template Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(arg: Input) => MaybePromise<Output>} fn
	* @param {{ inputs?: RemotePrerenderInputsGenerator<Input>, dynamic?: boolean }} [options]
	* @returns {RemotePrerenderFunction<Input, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote prerender function. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#prerender) for full documentation.
	*
	* @template {StandardSchemaV1} Schema
	* @template Output
	* @overload
	* @param {Schema} schema
	* @param {(arg: StandardSchemaV1.InferOutput<Schema>) => MaybePromise<Output>} fn
	* @param {{ inputs?: RemotePrerenderInputsGenerator<StandardSchemaV1.InferInput<Schema>>, dynamic?: boolean }} [options]
	* @returns {RemotePrerenderFunction<StandardSchemaV1.InferInput<Schema>, Output>}
	* @since 2.27
	*/
	/**
	* @template Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {any} [fn_or_options]
	* @param {{ inputs?: RemotePrerenderInputsGenerator<Input>, dynamic?: boolean }} [maybe_options]
	* @returns {RemotePrerenderFunction<Input, Output>}
	* @since 2.27
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function prerender$1(validate_or_fn, fn_or_options, maybe_options) {
		const maybe_fn = typeof fn_or_options === "function" ? fn_or_options : void 0;
		/** @type {typeof maybe_options} */
		const options = maybe_options ?? (maybe_fn ? void 0 : fn_or_options);
		/** @type {(arg?: Input) => MaybePromise<Output>} */
		const fn = maybe_fn ?? validate_or_fn;
		/** @type {(arg?: any) => MaybePromise<Input>} */
		const validate = create_validator(validate_or_fn, maybe_fn);
		/** @type {RemotePrerenderInternals} */
		const __ = {
			type: "prerender",
			id: "",
			name: "",
			has_arg: !!maybe_fn,
			inputs: options?.inputs,
			dynamic: options?.dynamic
		};
		/** @type {RemotePrerenderFunction<Input, Output> & { __: RemotePrerenderInternals }} */
		const wrapper = (arg) => {
			const { event, state } = get_request_store();
			const payload = stringify_remote_arg(arg);
			/** @type {Promise<Output> & Partial<RemoteResource<Output>>} */
			const promise = get_response(__, payload, state, async () => {
				const url = `/${app_dir}/remote/${__.id}${payload ? `/${payload}` : ""}`;
				if (!state.prerendering && !event.isRemoteRequest) {
					/** @type {RemoteFunctionResponse | undefined} */
					let prerendered;
					try {
						const response = await fetch(new URL(url, event.request.url).href, { redirect: "manual" });
						if (response.ok) prerendered = await response.json();
					} catch {}
					if (prerendered) {
						if (prerendered.type === "error") throw new HandledHttpError(prerendered.error);
						return parse$2(prerendered.data)._;
					}
				}
				if (state.prerendering?.remote_responses.has(url)) return state.prerendering.remote_responses.get(url);
				const promise = run_remote_function(event, {
					...state,
					is_in_remote_prerender: true
				}, false, () => validate(arg), fn);
				if (state.prerendering) state.prerendering.remote_responses.set(url, promise);
				const result = await promise;
				if (state.prerendering) {
					const body = {
						type: "result",
						data: stringify$2({ _: result })
					};
					state.prerendering.dependencies.set(url, {
						body: JSON.stringify(body),
						response: Response.json(body)
					});
				}
				return result;
			});
			promise.catch(noop$2);
			return promise;
		};
		Object.defineProperty(wrapper, "__", { value: __ });
		return wrapper;
	}
	/**
	* A pull-style async iterator that fans out a single stream of values to
	* multiple `for await (...)` consumers. Each subscriber gets its own
	* `AsyncGenerator` whose `.next()` resolves whenever a value is pushed via
	* `push(value)`. Multiple consumers see the same values without each one
	* driving an independent underlying source.
	*
	* Backpressure is **latest-wins**: if values arrive faster than a particular
	* consumer drains its iterator, only the most-recently-pushed value is kept
	* pending for that subscriber. Earlier undrained values are dropped. This is
	* appropriate for live data streams (reactive state replication), not for
	* event logs where every value must be delivered.
	*
	* Lifecycle hooks are exposed via the constructor:
	*
	*   - `start()` is called when the subscriber count transitions
	*     from 0 to 1 (e.g. to start a pump pulling from a real source).
	*   - `stop()` is called when the subscriber count transitions
	*     from non-zero back to 0 (e.g. to tear down that pump).
	*
	* Either hook may be omitted.
	*
	* The owner is responsible for calling `push(value)` to broadcast values,
	* `done()` to signal natural completion to all subscribers, and `fail(error)`
	* to broadcast a terminal error. After `done()` or `fail()`, the iterator
	* rejects further `subscribe()` calls with the terminal state appropriately.
	*
	* @template T
	*/
	var SharedIterator = class {
		/**
		* @typedef {object} Subscriber
		* @property {{ value: any } | null} pending
		* @property {{ error: unknown } | null} pending_error
		* @property {boolean} finished
		* @property {((result: IteratorResult<any, void>) => void) | null} waiting_resolve
		* @property {((reason: unknown) => void) | null} waiting_reject
		*/
		/** @type {Set<Subscriber>} */
		#subscribers = /* @__PURE__ */ new Set();
		/** @type {((instance: SharedIterator<T>) => (() => void)) | undefined} */
		#start = void 0;
		/** @type {(() => void) | undefined} */
		#stop = void 0;
		/** Once `done()` or `fail()` has been broadcast, no new values are accepted. */
		#closed = false;
		/** @type {unknown} */
		#terminal_error = void 0;
		/** Whether `done()` or `fail()` has been broadcast. */
		get closed() {
			return this.#closed;
		}
		/**
		* @param {(instance: SharedIterator<T>) => (() => void)} [start]
		*/
		constructor(start) {
			this.#start = start;
		}
		/** @param {T} value */
		push(value) {
			if (this.#closed) return;
			for (const subscriber of this.#subscribers) if (subscriber.waiting_resolve) {
				const resolve = subscriber.waiting_resolve;
				subscriber.waiting_resolve = null;
				subscriber.waiting_reject = null;
				resolve({
					value,
					done: false
				});
			} else subscriber.pending = { value };
		}
		/**
		* Signal natural completion to all current subscribers, and to any future
		* subscriber (which will receive an immediately-done iterator).
		*/
		done() {
			if (this.#closed) return;
			this.#closed = true;
			for (const subscriber of this.#subscribers) {
				subscriber.finished = true;
				if (subscriber.waiting_resolve) {
					const resolve = subscriber.waiting_resolve;
					subscriber.waiting_resolve = null;
					subscriber.waiting_reject = null;
					resolve({
						value: void 0,
						done: true
					});
				}
			}
			this.#subscribers.clear();
		}
		/**
		* Broadcast a terminal error. All current subscribers will reject their
		* next `.next()` call with `error`. Future subscribers will also reject
		* their first `.next()`.
		*
		* @param {unknown} error
		*/
		fail(error) {
			if (this.#closed) return;
			this.#closed = true;
			this.#terminal_error = error;
			for (const subscriber of this.#subscribers) {
				subscriber.finished = true;
				if (subscriber.waiting_reject) {
					const reject = subscriber.waiting_reject;
					subscriber.waiting_resolve = null;
					subscriber.waiting_reject = null;
					reject(error);
				} else subscriber.pending_error = { error };
			}
			this.#subscribers.clear();
		}
		/**
		* Subscribe to the shared stream. Returns an `AsyncGenerator<T>` that
		* yields every value pushed after this call (and, if `initial_value` is
		* provided, that value as the first yield).
		*
		* @param {{ initial_value?: { value: T } }} [options]
		*   `initial_value` lets the caller seed the iterator with a synchronously-
		*   available current value before any new pushes arrive (e.g. the
		*   "last-seen value" of a reactive resource). Pass it wrapped in an
		*   object so `undefined` can be distinguished from "no initial value".
		* @returns {AsyncGenerator<T, void, void>}
		*/
		subscribe(options) {
			/** @type {Subscriber} */
			const subscriber = {
				pending: options?.initial_value ? { value: options.initial_value.value } : null,
				pending_error: this.#closed && this.#terminal_error !== void 0 ? { error: this.#terminal_error } : null,
				finished: this.#closed && this.#terminal_error === void 0,
				waiting_resolve: null,
				waiting_reject: null
			};
			if (!subscriber.finished && subscriber.pending_error === null) this.#subscribers.add(subscriber);
			if (!this.#closed) this.#stop ??= this.#start?.(this);
			const unsubscribe = () => {
				subscriber.finished = true;
				if (this.#subscribers.delete(subscriber) && this.#subscribers.size === 0) this.#stop?.();
			};
			/** @type {AsyncGenerator<T, void, void>} */
			const iterator = {
				next() {
					if (subscriber.pending_error) {
						const { error } = subscriber.pending_error;
						subscriber.pending_error = null;
						unsubscribe();
						return Promise.reject(error);
					}
					if (subscriber.pending) {
						const { value } = subscriber.pending;
						subscriber.pending = null;
						return Promise.resolve({
							value,
							done: false
						});
					}
					if (subscriber.finished) return Promise.resolve({
						value: void 0,
						done: true
					});
					return new Promise((resolve, reject) => {
						subscriber.waiting_resolve = resolve;
						subscriber.waiting_reject = reject;
					});
				},
				return(value) {
					unsubscribe();
					if (subscriber.waiting_resolve) {
						const resolve = subscriber.waiting_resolve;
						subscriber.waiting_resolve = null;
						subscriber.waiting_reject = null;
						resolve({
							value: void 0,
							done: true
						});
					}
					return Promise.resolve({
						value,
						done: true
					});
				},
				throw(error) {
					unsubscribe();
					if (subscriber.waiting_reject) {
						const reject = subscriber.waiting_reject;
						subscriber.waiting_resolve = null;
						subscriber.waiting_reject = null;
						reject(error);
					}
					return Promise.reject(error);
				},
				[Symbol.asyncIterator]() {
					return iterator;
				},
				async [Symbol.asyncDispose]() {}
			};
			return iterator;
		}
	};
	/** @import { RemoteLiveQuery, RemoteLiveQueryFunction, RemoteQuery, RemoteQueryFunction } from '$app/server' */
	/** @import { RequestEvent } from '@sveltejs/kit' */
	/** @import { RemoteInternals, MaybePromise, RequestState, RemoteQueryLiveInternals, RemoteQueryBatchInternals, RemoteQueryInternals, RemoteLiveQueryUserFunctionReturnType } from 'types' */
	/** @import { StandardSchemaV1 } from '@standard-schema/spec' */
	/**
	* Creates a remote query. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query) for full documentation.
	*
	* @template Output
	* @overload
	* @param {() => MaybePromise<Output>} fn
	* @returns {RemoteQueryFunction<void, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote query. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query) for full documentation.
	*
	* @template Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(arg: Input) => MaybePromise<Output>} fn
	* @returns {RemoteQueryFunction<Input, Output>}
	* @since 2.27
	*/
	/**
	* Creates a remote query. When called from the browser, the function will be invoked on the server via a `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query) for full documentation.
	*
	* @template {StandardSchemaV1} Schema
	* @template Output
	* @overload
	* @param {Schema} schema
	* @param {(arg: StandardSchemaV1.InferOutput<Schema>) => MaybePromise<Output>} fn
	* @returns {RemoteQueryFunction<StandardSchemaV1.InferInput<Schema>, Output, StandardSchemaV1.InferOutput<Schema>>}
	* @since 2.27
	*/
	/**
	* @template Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {(args?: Input) => MaybePromise<Output>} [maybe_fn]
	* @returns {RemoteQueryFunction<Input, Output>}
	* @since 2.27
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function query$1(validate_or_fn, maybe_fn) {
		/** @type {(arg?: Input) => Output} */
		const fn = maybe_fn !== null && maybe_fn !== void 0 ? maybe_fn : validate_or_fn;
		/** @type {(arg?: any) => MaybePromise<Input>} */
		const validate = create_validator(validate_or_fn, maybe_fn);
		/** @type {RemoteQueryInternals} */
		const __ = {
			type: "query",
			id: "",
			name: "",
			validate,
			bind(payload, validated_arg) {
				const { event, state } = get_request_store();
				return create_query_resource(__, payload, event, state, () => run_remote_function(event, _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_query: true }), false, () => validated_arg, fn));
			}
		};
		/** @type {RemoteQueryFunction<Input, Output> & { __: RemoteQueryInternals }} */
		const wrapper = (arg) => {
			if (prerendering) remote_query_prerender({
				type: "query",
				name: __.name
			});
			const { event, state } = get_request_store();
			const payload = stringify_remote_arg(arg);
			return create_query_resource(__, payload, event, state, () => run_remote_function(event, _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_query: true }), false, () => validate(arg), fn));
		};
		Object.defineProperty(wrapper, "__", { value: __ });
		return wrapper;
	}
	/**
	* Creates a live remote query. When called from the browser, the function will be invoked on the server via a streaming `fetch` call.
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query.live) for full documentation.
	*
	* @template Output
	* @overload
	* @param {(arg: void) => RemoteLiveQueryUserFunctionReturnType<Output>} fn
	* @returns {RemoteLiveQueryFunction<void, Output>}
	*/
	/**
	* @template Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(arg: Input) => RemoteLiveQueryUserFunctionReturnType<Output>} fn
	* @returns {RemoteLiveQueryFunction<Input, Output>}
	*/
	/**
	* @template {StandardSchemaV1} Schema
	* @template Output
	* @overload
	* @param {Schema} schema
	* @param {(arg: StandardSchemaV1.InferOutput<Schema>) => RemoteLiveQueryUserFunctionReturnType<Output>} fn
	* @returns {RemoteLiveQueryFunction<StandardSchemaV1.InferInput<Schema>, Output, StandardSchemaV1.InferOutput<Schema>>}
	*/
	/**
	* @template Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {(args: Input) => RemoteLiveQueryUserFunctionReturnType<Output>} [maybe_fn]
	* @returns {RemoteLiveQueryFunction<Input, Output>}
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function live(validate_or_fn, maybe_fn) {
		/** @type {(arg: Input) => RemoteLiveQueryUserFunctionReturnType<Output>} */
		const fn = maybe_fn !== null && maybe_fn !== void 0 ? maybe_fn : validate_or_fn;
		/** @type {(arg?: any) => MaybePromise<Input>} */
		const validate = create_validator(validate_or_fn, maybe_fn);
		/**
		* @param {any} event
		* @param {any} state
		* @param {any} get_input
		*/
		const run = (event, state, get_input) => run_remote_generator(event, _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_query: true }), false, get_input, fn, __.name);
		/** @type {RemoteQueryLiveInternals} */
		const __ = {
			type: "query_live",
			id: "",
			name: "",
			run: (event, state, arg) => run(event, state, () => validate(arg)),
			validate,
			bind(payload, validated_arg) {
				const { event, state } = get_request_store();
				return create_live_query_resource(__, payload, event, state, () => run(event, state, () => validated_arg));
			}
		};
		/** @type {RemoteLiveQueryFunction<Input, Output> & { __: RemoteQueryLiveInternals }} */
		const wrapper = (arg) => {
			if (prerendering) remote_query_prerender({
				type: "query.live",
				name: __.name
			});
			const { event, state } = get_request_store();
			const payload = stringify_remote_arg(arg);
			return create_live_query_resource(__, payload, event, state, () => run(event, state, () => validate(arg)));
		};
		Object.defineProperty(wrapper, "__", { value: __ });
		return wrapper;
	}
	/**
	* Creates a batch query function that collects multiple calls and executes them in a single request
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query.batch) for full documentation.
	*
	* @template Input
	* @template Output
	* @overload
	* @param {'unchecked'} validate
	* @param {(args: Input[]) => MaybePromise<(arg: Input, idx: number) => Output>} fn
	* @returns {RemoteQueryFunction<Input, Output>}
	* @since 2.35
	*/
	/**
	* Creates a batch query function that collects multiple calls and executes them in a single request
	*
	* See [Remote functions](https://svelte.dev/docs/kit/remote-functions#query.batch) for full documentation.
	*
	* @template {StandardSchemaV1} Schema
	* @template Output
	* @overload
	* @param {Schema} schema
	* @param {(args: StandardSchemaV1.InferOutput<Schema>[]) => MaybePromise<(arg: StandardSchemaV1.InferOutput<Schema>, idx: number) => Output>} fn
	* @returns {RemoteQueryFunction<StandardSchemaV1.InferInput<Schema>, Output, StandardSchemaV1.InferOutput<Schema>>}
	* @since 2.35
	*/
	/**
	* @template Input
	* @template Output
	* @param {any} validate_or_fn
	* @param {(args?: Input[]) => MaybePromise<(arg: Input, idx: number) => Output>} [maybe_fn]
	* @returns {RemoteQueryFunction<Input, Output>}
	* @since 2.35
	*/
	/*@__NO_SIDE_EFFECTS__*/
	function batch(validate_or_fn, maybe_fn) {
		/** @type {(args?: Input[]) => MaybePromise<(arg: Input, idx: number) => Output>} */
		const fn = maybe_fn !== null && maybe_fn !== void 0 ? maybe_fn : validate_or_fn;
		/** @type {(arg?: any) => MaybePromise<Input>} */
		const validate = create_validator(validate_or_fn, maybe_fn);
		/**
		* Enqueues a single call into the current batch (creating one if necessary)
		* and returns a promise that resolves with the result for this entry.
		*
		* @param {string} payload — the stringified raw argument (cache key)
		* @param {() => MaybePromise<any>} get_validated — produces the validated argument for this entry
		* @returns {Promise<any>}
		*/
		const enqueue = (payload, get_validated) => {
			const { event, state } = get_request_store();
			return new Promise((resolve, reject) => {
				var _state$remote, _state$remote$batches;
				const batches = (_state$remote$batches = (_state$remote = state.remote).batches) !== null && _state$remote$batches !== void 0 ? _state$remote$batches : _state$remote.batches = /* @__PURE__ */ new Map();
				let batched = batches.get(__.id);
				if (!batched) {
					batched = /* @__PURE__ */ new Map();
					batches.set(__.id, batched);
				}
				const entry = batched.get(payload);
				if (entry) {
					entry.resolvers.push({
						resolve,
						reject
					});
					return;
				}
				batched.set(payload, {
					get_validated,
					resolvers: [{
						resolve,
						reject
					}]
				});
				if (batched.size > 1) return;
				setTimeout(async () => {
					batches.delete(__.id);
					const entries = Array.from(batched.values());
					try {
						return await run_remote_function(event, _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_query: true }), false, async () => Promise.all(entries.map((entry) => entry.get_validated())), async (input) => {
							const get_result = await fn(input);
							for (let i = 0; i < entries.length; i++) try {
								const result = get_result(input[i], i);
								for (const resolver of entries[i].resolvers) resolver.resolve(result);
							} catch (error) {
								for (const resolver of entries[i].resolvers) resolver.reject(error);
							}
						});
					} catch (error) {
						for (const entry of batched.values()) for (const resolver of entry.resolvers) resolver.reject(error);
					}
				}, 0);
			});
		};
		/** @type {RemoteQueryBatchInternals} */
		const __ = {
			type: "query_batch",
			id: "",
			name: "",
			validate,
			run: async (args) => {
				const { event, state } = get_request_store();
				return run_remote_function(event, _objectSpread2(_objectSpread2({}, state), {}, { is_in_remote_query: true }), false, async () => Promise.all(args.map(validate)), async (input) => {
					const get_result = await fn(input);
					return Promise.all(input.map(async (arg, i) => {
						try {
							return {
								type: "result",
								data: get_result(arg, i)
							};
						} catch (error) {
							return {
								type: "error",
								error: await handle_error_and_jsonify(event, state, error)
							};
						}
					}));
				});
			},
			bind(payload, validated_arg) {
				const { event, state } = get_request_store();
				return create_query_resource(__, payload, event, state, () => enqueue(payload, () => validated_arg));
			}
		};
		/** @type {RemoteQueryFunction<Input, Output> & { __: RemoteQueryBatchInternals }} */
		const wrapper = (arg) => {
			if (prerendering) remote_query_prerender({
				type: "query.batch",
				name: __.name
			});
			const { event, state } = get_request_store();
			const payload = stringify_remote_arg(arg);
			return create_query_resource(__, payload, event, state, () => enqueue(payload, () => validate(arg)));
		};
		Object.defineProperty(wrapper, "__", { value: __ });
		return wrapper;
	}
	/**
	* Include this value in the returned payload...
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {RemoteInternals} internals
	* @param {string} payload
	* @param {() => Promise<any>} fn
	*/
	function refresh(event, state, internals, payload, fn) {
		var _state$remote2, _state$remote2$explic;
		if (!internals.id) return;
		if (!event.isRemoteRequest && state.is_in_remote_form_or_command) return;
		const key = create_remote_key(internals.id, payload);
		((_state$remote2$explic = (_state$remote2 = state.remote).explicit) !== null && _state$remote2$explic !== void 0 ? _state$remote2$explic : _state$remote2.explicit = /* @__PURE__ */ new Map()).set(key, {
			internals,
			fn
		});
	}
	/**
	* @param {RemoteInternals} __
	* @param {string} payload — the stringified raw argument (i.e. the cache key the client will use)
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {() => Promise<any>} fn
	* @returns {RemoteQuery<any>}
	*/
	function create_query_resource(__, payload, event, state, fn) {
		/** @type {Promise<any> | null} */
		let promise = null;
		const get_promise = () => {
			var _promise;
			return (_promise = promise) !== null && _promise !== void 0 ? _promise : promise = get_response(__, payload, state, fn);
		};
		const populate_hydratable = () => {
			if (__.id && state.is_in_render) get_promise().catch(noop$2);
		};
		return {
			/** @type {Promise<any>['catch']} */
			catch(onrejected) {
				return get_promise().catch(onrejected);
			},
			get current() {
				populate_hydratable();
			},
			get error() {
				populate_hydratable();
			},
			/** @type {Promise<any>['finally']} */
			finally(onfinally) {
				return get_promise().finally(onfinally);
			},
			get loading() {
				populate_hydratable();
				return true;
			},
			get ready() {
				populate_hydratable();
				return false;
			},
			refresh() {
				promise = null;
				delete get_cache(__, state)[payload];
				refresh(event, state, __, payload, get_promise);
				return Promise.resolve();
			},
			/** @param {any} value */
			set(value) {
				const p = promise = Promise.resolve(value);
				get_cache(__, state)[payload] = p;
				refresh(event, state, __, payload, () => p);
			},
			/** @type {Promise<any>['then']} */
			then(onfulfilled, onrejected) {
				return get_promise().then(onfulfilled, onrejected);
			},
			withOverride() {
				server_api_unavailable({ name: `${__.name}.withOverride()` });
			},
			get [Symbol.toStringTag]() {
				return "QueryResource";
			}
		};
	}
	/**
	* @param {RemoteQueryLiveInternals} __
	* @param {string} payload — the stringified raw argument (i.e. the cache key the client will use)
	* @param {RequestEvent} event
	* @param {RequestState} state
	* @param {() => AsyncGenerator<any, void, void>} get_generator
	* @returns {RemoteLiveQuery<any>}
	*/
	function create_live_query_resource(__, payload, event, state, get_generator) {
		/** @type {Promise<any> | null} */
		let promise = null;
		const get_first_value = async () => {
			var _iteratorAbruptCompletion = false;
			var _didIteratorError = false;
			var _iteratorError;
			try {
				for (var _iterator = _asyncIterator(get_generator()), _step; _iteratorAbruptCompletion = !(_step = await _iterator.next()).done; _iteratorAbruptCompletion = false) return _step.value;
			} catch (err) {
				_didIteratorError = true;
				_iteratorError = err;
			} finally {
				try {
					if (_iteratorAbruptCompletion && _iterator.return != null) await _iterator.return();
				} finally {
					if (_didIteratorError) throw _iteratorError;
				}
			}
			remote_query_live_no_value({ name: __.name });
		};
		const get_promise = () => {
			var _promise2;
			return (_promise2 = promise) !== null && _promise2 !== void 0 ? _promise2 : promise = get_response(__, payload, state, get_first_value);
		};
		const populate_hydratable = () => {
			if (__.id && state.is_in_render) get_promise().catch(noop$2);
		};
		return {
			/** @type {Promise<any>['catch']} */
			catch(onrejected) {
				return get_promise().catch(onrejected);
			},
			get current() {
				populate_hydratable();
			},
			get error() {
				populate_hydratable();
			},
			/** @type {Promise<any>['finally']} */
			finally(onfinally) {
				return get_promise().finally(onfinally);
			},
			get done() {
				populate_hydratable();
				return false;
			},
			get loading() {
				populate_hydratable();
				return true;
			},
			get ready() {
				populate_hydratable();
				return false;
			},
			get connected() {
				populate_hydratable();
				return false;
			},
			reconnect() {
				promise = null;
				delete get_cache(__, state)[payload];
				refresh(event, state, __, payload, get_promise);
				return Promise.resolve();
			},
			/** @type {Promise<any>['then']} */
			then(onfulfilled, onrejected) {
				return get_promise().then(onfulfilled, onrejected);
			},
			[Symbol.asyncIterator]() {
				var _state$remote3, _state$remote3$live_i;
				const key = create_remote_key(__.id, payload);
				const cache = (_state$remote3$live_i = (_state$remote3 = state.remote).live_iterators) !== null && _state$remote3$live_i !== void 0 ? _state$remote3$live_i : _state$remote3.live_iterators = /* @__PURE__ */ new Map();
				let cached = cache.get(key);
				if (!cached) {
					cached = create_shared_live_iterator(event.request.signal, get_generator);
					cache.set(key, cached);
				}
				return cached.subscribe();
			},
			get [Symbol.toStringTag]() {
				return "LiveQueryResource";
			}
		};
	}
	/**
	* Wraps a lazily-created live-query generator so that multiple `for await`
	* consumers within the same request share one underlying iteration. The first
	* subscriber starts the generator; values are broadcast to all subscribers
	* via a `SharedIterator`. When the last subscriber unsubscribes, the generator
	* is closed via `generator.return(undefined)`.
	*
	* If `signal` aborts (typically because the client has disconnected), the
	* pump is torn down and any in-flight `next()` calls on consumer iterators
	* resolve with `{ done: true }`, so suspended `for await` loops unwind
	* cleanly rather than leaking.
	*
	* @param {AbortSignal} signal
	* @param {() => AsyncGenerator<any, void, void>} get_generator
	*/
	function create_shared_live_iterator(signal, get_generator) {
		return new SharedIterator((instance) => {
			if (signal.aborted) {
				instance.done();
				return noop$2;
			}
			const generator = get_generator();
			let aborted = false;
			const close = () => {
				aborted = true;
				generator.return().catch(noop$2);
			};
			signal.addEventListener("abort", () => (close(), instance.done()), { once: true });
			(async () => {
				try {
					while (true) {
						const result = await generator.next();
						if (result.done) {
							instance.done();
							return;
						}
						instance.push(result.value);
					}
				} catch (error) {
					if (!aborted) instance.fail(error);
				} finally {
					close();
				}
			})();
			return close;
		});
	}
	Object.defineProperty(query$1, "batch", {
		value: batch,
		enumerable: true
	});
	Object.defineProperty(query$1, "live", {
		value: live,
		enumerable: true
	});
	/**
	* Calls Go asynchronously. Its I/O runs outside the runtime while other
	* independent queries start; the runtime resumes when Go settles the promise.
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
	async function host(id, payload) {
		const raw = await globalThis.__skgo_remote(id, payload);
		const res = JSON.parse(raw);
		if (res.h) throw new HandledHttpError(res.h);
		if (res.r) throw new Redirect(res.r.status, res.r.location);
		if (res.e) throw new HttpError({
			status: res.e.status ?? 500,
			message: res.e.message
		});
		return parse$2(res.v);
	}
	function query(validate_or_fn, maybe_fn) {
		const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
		const wrapper = maybe_fn ? /* @__PURE__ */ query$1(validate_or_fn, fn) : /* @__PURE__ */ query$1(fn);
		return wrapper;
	}
	function prerender(validate_or_fn, maybe_fn, options) {
		const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
		const hasValidator = typeof maybe_fn === "function";
		const normalizedOptions = options ?? (hasValidator ? void 0 : maybe_fn);
		const wrapper = !normalizedOptions?.dynamic ? /* @__PURE__ */ prerender$1("unchecked", fn, normalizedOptions) : hasValidator ? /* @__PURE__ */ prerender$1(validate_or_fn, fn, options) : /* @__PURE__ */ prerender$1(fn, maybe_fn);
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
	async function host_batch(id, args) {
		const payloads = args.map((arg) => stringify_remote_arg(arg));
		const raw = await globalThis.__skgo_remote(id, JSON.stringify(payloads));
		const res = JSON.parse(raw);
		if (res.r) throw new Redirect(res.r.status, res.r.location);
		if (res.e) throw new HttpError({
			status: res.e.status ?? 500,
			message: res.e.message
		});
		const nodes = res.n ?? [];
		return (_arg, i) => {
			const node = nodes[i];
			if (!node) throw new HttpError({
				status: 500,
				message: "Internal Error"
			});
			if (node.e) throw new HttpError({
				status: node.e.status ?? 500,
				message: node.e.message
			});
			return parse$2(node.v);
		};
	}
	query.batch = (validate_or_fn, maybe_fn) => {
		const fn = (args) => host_batch(wrapper.__.id, args);
		const wrapper = maybe_fn ? query$1.batch(validate_or_fn, fn) : query$1.batch(fn);
		return wrapper;
	};
	query.live = (validate_or_fn, maybe_fn) => {
		const fn = (arg) => {
			const payload = stringify_remote_arg(arg);
			let taken = false;
			return {
				next: async () => {
					if (taken) return {
						value: void 0,
						done: true
					};
					taken = true;
					return {
						value: await host(wrapper.__.id, payload),
						done: false
					};
				},
				return: () => ({
					value: void 0,
					done: true
				})
			};
		};
		const wrapper = maybe_fn ? query$1.live(validate_or_fn, fn) : query$1.live(fn);
		return wrapper;
	};
	function command(validate_or_fn, maybe_fn) {
		const fn = (arg) => host(wrapper.__.id, stringify_remote_arg(arg));
		const wrapper = maybe_fn ? /* @__PURE__ */ command$1(validate_or_fn, fn) : /* @__PURE__ */ command$1(fn);
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
	function form(validate_or_fn, maybe_fn) {
		const wrapper = maybe_fn === void 0 ? /* @__PURE__ */ form$1(validate_or_fn) : /* @__PURE__ */ form$1(validate_or_fn, maybe_fn);
		(globalThis.__skgo_forms ??= []).push(wrapper);
		return wrapper;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/contact.remote-BqzQT4jM.js
	var contact_remote_exports = /* @__PURE__ */ __exportAll({
		getMessages: () => getMessages,
		previewMessage: () => previewMessage,
		sendMessage: () => sendMessage
	});
	var unimplemented$17 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getMessages = query(() => unimplemented$17());
	var previewMessage = command("unchecked", (_arg) => unimplemented$17());
	var sendMessage = form("unchecked", (_arg) => unimplemented$17());
	init_remote_functions(contact_remote_exports, "src/routes/contact/contact.remote.ts", "14q4me9");
	for (const [name, fn] of Object.entries(contact_remote_exports)) {
		fn.__.id = "14q4me9/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/auth.remote-ZfK2QniR.js
	var auth_remote_exports = /* @__PURE__ */ __exportAll({
		signIn: () => signIn,
		signOut: () => signOut,
		whoami: () => whoami
	});
	var unimplemented$16 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var signIn = command("unchecked", (_arg) => unimplemented$16());
	var signOut = command(() => unimplemented$16());
	var whoami = query(() => unimplemented$16());
	init_remote_functions(auth_remote_exports, "src/lib/auth.remote.ts", "4cga8b");
	for (const [name, fn] of Object.entries(auth_remote_exports)) {
		fn.__.id = "4cga8b/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/todos.remote-DnD3SnZZ.js
	var todos_remote_exports = /* @__PURE__ */ __exportAll({
		addTodo: () => addTodo,
		getTodo: () => getTodo,
		getTodos: () => getTodos,
		renameTodo: () => renameTodo,
		retitleTodo: () => retitleTodo,
		watchCount: () => watchCount
	});
	var unimplemented$15 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var addTodo = command("unchecked", (_arg) => unimplemented$15());
	var getTodo = query("unchecked", (_arg) => unimplemented$15());
	var getTodos = query(() => unimplemented$15());
	var renameTodo = command("unchecked", (_arg) => unimplemented$15());
	var retitleTodo = command("unchecked", (_arg) => unimplemented$15());
	var watchCount = query.live(() => unimplemented$15());
	init_remote_functions(todos_remote_exports, "src/routes/todos/todos.remote.ts", "16h93j5");
	for (const [name, fn] of Object.entries(todos_remote_exports)) {
		fn.__.id = "16h93j5/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/site.remote-D4TvdCpj.js
	var site_remote_exports = /* @__PURE__ */ __exportAll({ getSite: () => getSite$1 });
	var unimplemented$14 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getSite$1 = query(() => unimplemented$14());
	init_remote_functions(site_remote_exports, "src/routes/site.remote.ts", "ezl04l");
	for (const [name, fn] of Object.entries(site_remote_exports)) {
		fn.__.id = "ezl04l/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/pricing.remote-DgC0nzCp.js
	var pricing_remote_exports = /* @__PURE__ */ __exportAll({
		getPlans: () => getPlans,
		getSpotlight: () => getSpotlight,
		quoteFor: () => quoteFor
	});
	var unimplemented$13 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getPlans = query(() => unimplemented$13());
	var getSpotlight = query(() => unimplemented$13());
	var quoteFor = command("unchecked", (_arg) => unimplemented$13());
	init_remote_functions(pricing_remote_exports, "src/routes/(marketing)/pricing/pricing.remote.ts", "gu007f");
	for (const [name, fn] of Object.entries(pricing_remote_exports)) {
		fn.__.id = "gu007f/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/about.remote-jTNIDD0v.js
	var about_remote_exports = /* @__PURE__ */ __exportAll({ buildReceipt: () => buildReceipt });
	var unimplemented$12 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var buildReceipt = prerender("unchecked", async (_arg) => unimplemented$12());
	init_remote_functions(about_remote_exports, "src/routes/about/about.remote.ts", "ks8sip");
	for (const [name, fn] of Object.entries(about_remote_exports)) {
		fn.__.id = "ks8sip/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/coexist.remote-B9Bmmn9A.js
	var coexist_remote_exports = /* @__PURE__ */ __exportAll({ sendRemoteNote: () => sendRemoteNote });
	var unimplemented$11 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var sendRemoteNote = form("unchecked", (_arg) => unimplemented$11());
	init_remote_functions(coexist_remote_exports, "src/routes/actions/coexist.remote.ts", "1klskj7");
	for (const [name, fn] of Object.entries(coexist_remote_exports)) {
		fn.__.id = "1klskj7/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/probe.remote-DPi48z2T.js
	var probe_remote_exports = /* @__PURE__ */ __exportAll({
		getBatch: () => getBatch,
		getDependent: () => getDependent,
		getHeld: () => getHeld,
		getSeed: () => getSeed,
		getValue: () => getValue,
		watchValue: () => watchValue
	});
	var unimplemented$10 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getBatch = query.batch("unchecked", (_args) => unimplemented$10());
	var getDependent = query("unchecked", (_arg) => unimplemented$10());
	var getHeld = query("unchecked", (_arg) => unimplemented$10());
	var getSeed = query("unchecked", (_arg) => unimplemented$10());
	var getValue = query("unchecked", (_arg) => unimplemented$10());
	var watchValue = query.live("unchecked", (_arg) => unimplemented$10());
	init_remote_functions(probe_remote_exports, "src/routes/async-ssr/[mode]/[group]/probe.remote.ts", "toba7i");
	for (const [name, fn] of Object.entries(probe_remote_exports)) {
		fn.__.id = "toba7i/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/quotes.remote-CS-h8F-o.js
	var quotes_remote_exports = /* @__PURE__ */ __exportAll({ getQuotes: () => getQuotes });
	var unimplemented$9 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getQuotes = query.batch("unchecked", (_args) => unimplemented$9());
	init_remote_functions(quotes_remote_exports, "src/routes/batch/quotes.remote.ts", "x3uq7o");
	for (const [name, fn] of Object.entries(quotes_remote_exports)) {
		fn.__.id = "x3uq7o/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/docs.remote-C_nwTMZ5.js
	var docs_remote_exports = /* @__PURE__ */ __exportAll({ getPage: () => getPage });
	var unimplemented$8 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getPage = query("unchecked", (_arg) => unimplemented$8());
	init_remote_functions(docs_remote_exports, "src/routes/docs/[...rest]/docs.remote.ts", "9lqpme");
	for (const [name, fn] of Object.entries(docs_remote_exports)) {
		fn.__.id = "9lqpme/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/empty.remote-glnnsAQo.js
	var empty_remote_exports = /* @__PURE__ */ __exportAll({
		getKnownReport: () => getKnownReport,
		getModels: () => getModels,
		getReport: () => getReport,
		reparse: () => reparse
	});
	var unimplemented$7 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getKnownReport = query(() => unimplemented$7());
	var getModels = query(() => unimplemented$7());
	var getReport = query(() => unimplemented$7());
	var reparse = command(() => unimplemented$7());
	init_remote_functions(empty_remote_exports, "src/routes/empty/empty.remote.ts", "1sgmw1");
	for (const [name, fn] of Object.entries(empty_remote_exports)) {
		fn.__.id = "1sgmw1/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/boundary.remote-avw4kQgO.js
	var boundary_remote_exports = /* @__PURE__ */ __exportAll({ readSensor: () => readSensor });
	var unimplemented$6 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var readSensor = query(() => unimplemented$6());
	init_remote_functions(boundary_remote_exports, "src/routes/error/boundary/boundary.remote.ts", "1627chi");
	for (const [name, fn] of Object.entries(boundary_remote_exports)) {
		fn.__.id = "1627chi/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/command.remote-D_a-V-o4.js
	var command_remote_exports = /* @__PURE__ */ __exportAll({
		bumpTally: () => bumpTally,
		getTally: () => getTally
	});
	var unimplemented$5 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var bumpTally = command(() => unimplemented$5());
	var getTally = query(() => unimplemented$5());
	init_remote_functions(command_remote_exports, "src/routes/error/command/command.remote.ts", "nv8t2");
	for (const [name, fn] of Object.entries(command_remote_exports)) {
		fn.__.id = "nv8t2/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/redirect.remote-Cv7KS6CZ.js
	var redirect_remote_exports = /* @__PURE__ */ __exportAll({ whereTo: () => whereTo });
	var unimplemented$4 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var whereTo = query(() => unimplemented$4());
	init_remote_functions(redirect_remote_exports, "src/routes/error/redirect/redirect.remote.ts", "62gfcm");
	for (const [name, fn] of Object.entries(redirect_remote_exports)) {
		fn.__.id = "62gfcm/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/item.remote-Dud7ZQCH.js
	var item_remote_exports = /* @__PURE__ */ __exportAll({ getItem: () => getItem });
	var unimplemented$3 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getItem = query("unchecked", (_arg) => unimplemented$3());
	init_remote_functions(item_remote_exports, "src/routes/items/[id]/item.remote.ts", "4rw8ae");
	for (const [name, fn] of Object.entries(item_remote_exports)) {
		fn.__.id = "4rw8ae/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/board.remote-fs9ryq8h.js
	var board_remote_exports = /* @__PURE__ */ __exportAll({ watchBoard: () => watchBoard });
	var unimplemented$2 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var watchBoard = query.live(() => unimplemented$2());
	init_remote_functions(board_remote_exports, "src/routes/live/board.remote.ts", "wx3cst");
	for (const [name, fn] of Object.entries(board_remote_exports)) {
		fn.__.id = "wx3cst/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/optional.remote-C9YErA6h.js
	var optional_remote_exports = /* @__PURE__ */ __exportAll({ submit: () => submit });
	var unimplemented$1 = () => {
		throw new Error("skgo: implemented in Go");
	};
	var submit = form("unchecked", (_arg) => unimplemented$1());
	init_remote_functions(optional_remote_exports, "src/routes/optional/optional.remote.ts", "83n4w1");
	for (const [name, fn] of Object.entries(optional_remote_exports)) {
		fn.__.id = "83n4w1/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/assets/gate.remote-D3tHRTBW.js
	var gate_remote_exports = /* @__PURE__ */ __exportAll({
		getBanner: () => getBanner,
		getNote: () => getNote,
		writeNotes: () => writeNotes
	});
	var unimplemented = () => {
		throw new Error("skgo: implemented in Go");
	};
	var getBanner = query(() => unimplemented());
	var getNote = query("unchecked", (_arg) => unimplemented());
	var writeNotes = command("unchecked", (_arg) => unimplemented());
	init_remote_functions(gate_remote_exports, "src/routes/todos/gate/gate.remote.ts", "1eqvn7x");
	for (const [name, fn] of Object.entries(gate_remote_exports)) {
		fn.__.id = "1eqvn7x/" + name;
		fn.__.name = name;
	}
	//#endregion
	//#region ../../ephemeral/experiments/20261015-live-metadata/product-runtime/goja-intermediate/bundle.js
	var UNINITIALIZED = Symbol("uninitialized");
	var ATTACHMENT_KEY = "@attach";
	var ATTR_REGEX = /[&"<]/g;
	var CONTENT_REGEX = /[&<]/g;
	/**
	* @template V
	* @param {V} value
	* @param {boolean} [is_attr]
	*/
	function escape_html$1(value, is_attr) {
		const str = String(value ?? "");
		const pattern = is_attr ? ATTR_REGEX : CONTENT_REGEX;
		pattern.lastIndex = 0;
		let escaped = "";
		let last = 0;
		while (pattern.test(str)) {
			const i = pattern.lastIndex - 1;
			const ch = str[i];
			escaped += str.substring(last, i) + (ch === "&" ? "&amp;" : ch === "\"" ? "&quot;" : "&lt;");
			last = i + 1;
		}
		return escaped + str.substring(last);
	}
	function r(e) {
		var t, f, n = "";
		if ("string" == typeof e || "number" == typeof e) n += e;
		else if ("object" == typeof e) if (Array.isArray(e)) {
			var o = e.length;
			for (t = 0; t < o; t++) e[t] && (f = r(e[t])) && (n && (n += " "), n += f);
		} else for (f in e) e[f] && (n && (n += " "), n += f);
		return n;
	}
	function clsx$1() {
		for (var e, t, f = 0, n = "", o = arguments.length; f < o; f++) (e = arguments[f]) && (t = r(e)) && (n && (n += " "), n += t);
		return n;
	}
	var is_array = Array.isArray;
	Array.prototype.indexOf;
	Array.prototype.includes;
	Array.from;
	var define_property = Object.defineProperty;
	Array.prototype;
	var has_own_property = Object.prototype.hasOwnProperty;
	var noop$1 = () => {};
	/**
	* @template [T=any]
	* @param {any} value
	* @returns {value is PromiseLike<T>}
	*/
	function is_promise(value) {
		return typeof value?.then === "function";
	}
	/** @param {Function} fn */
	function run(fn) {
		return fn();
	}
	/** @param {Array<() => void>} arr */
	function run_all(arr) {
		for (var i = 0; i < arr.length; i++) arr[i]();
	}
	/**
	* TODO replace with Promise.withResolvers once supported widely enough
	* @template [T=void]
	*/
	function deferred() {
		/** @type {(value: T) => void} */
		var resolve;
		/** @type {(reason: any) => void} */
		var reject;
		return {
			promise: new Promise((res, rej) => {
				resolve = res;
				reject = rej;
			}),
			resolve,
			reject
		};
	}
	/**
	* `<div translate={false}>` should be rendered as `<div translate="no">` and _not_
	* `<div translate="false">`, which is equivalent to `<div translate="yes">`. There
	* may be other odd cases that need to be added to this list in future
	* @type {Record<string, Map<any, string>>}
	*/
	var replacements$1 = { translate: /* @__PURE__ */ new Map([[true, "yes"], [false, "no"]]) };
	/**
	* @template V
	* @param {string} name
	* @param {V} value
	* @param {boolean} [is_boolean]
	* @returns {string}
	*/
	function attr(name, value, is_boolean = false) {
		if (name === "hidden" && value !== "until-found") is_boolean = true;
		if (value == null || is_boolean && !value && value !== "") return "";
		const normalized = has_own_property.call(replacements$1, name) && replacements$1[name].get(value) || value;
		return ` ${name}${is_boolean ? `=""` : `="${escape_html$1(normalized, true)}"`}`;
	}
	/**
	* Small wrapper around clsx to preserve Svelte's (weird) handling of falsy values.
	* TODO Svelte 6 revisit this, and likely turn all falsy values into the empty string (what clsx also does)
	* @param  {any} value
	*/
	function clsx(value) {
		if (typeof value === "object") return clsx$1(value);
		else return value ?? "";
	}
	var whitespace = [..." 	\n\r\f\xA0\v﻿"];
	/**
	* @param {any} value
	* @param {string | null} [hash]
	* @param {Record<string, boolean>} [directives]
	* @returns {string | null}
	*/
	function to_class(value, hash, directives) {
		var classname = value == null ? "" : "" + value;
		if (hash) classname = classname ? classname + " " + hash : hash;
		if (directives) {
			for (var key of Object.keys(directives)) if (directives[key]) classname = classname ? classname + " " + key : key;
			else if (classname.length) {
				var len = key.length;
				var a = 0;
				while ((a = classname.indexOf(key, a)) >= 0) {
					var b = a + len;
					if ((a === 0 || whitespace.includes(classname[a - 1])) && (b === classname.length || whitespace.includes(classname[b]))) classname = (a === 0 ? "" : classname.substring(0, a)) + classname.substring(b + 1);
					else a = b;
				}
			}
		}
		return classname === "" ? null : classname;
	}
	/**
	*
	* @param {Record<string,any>} styles
	* @param {boolean} important
	*/
	function append_styles(styles, important = false) {
		var separator = important ? " !important;" : ";";
		var css = "";
		for (var key of Object.keys(styles)) {
			var value = styles[key];
			if (value != null && value !== "") css += " " + key + ": " + value + separator;
		}
		return css;
	}
	/**
	* @param {string} name
	* @returns {string}
	*/
	function to_css_name(name) {
		if (name[0] !== "-" || name[1] !== "-") return name.toLowerCase();
		return name;
	}
	/**
	* @param {any} value
	* @param {Record<string, any> | [Record<string, any>, Record<string, any>]} [styles]
	* @returns {string | null}
	*/
	function to_style(value, styles) {
		if (styles) {
			var new_style = "";
			/** @type {Record<string,any> | undefined} */
			var normal_styles;
			/** @type {Record<string,any> | undefined} */
			var important_styles;
			if (Array.isArray(styles)) {
				normal_styles = styles[0];
				important_styles = styles[1];
			} else normal_styles = styles;
			if (value) {
				value = String(value).replaceAll(/\/\*.*?\*\//g, "").trim();
				/** @type {boolean | '"' | "'"} */
				var in_str = false;
				var in_apo = 0;
				var in_comment = false;
				var reserved_names = [];
				if (normal_styles) reserved_names.push(...Object.keys(normal_styles).map(to_css_name));
				if (important_styles) reserved_names.push(...Object.keys(important_styles).map(to_css_name));
				var start_index = 0;
				var name_index = -1;
				const len = value.length;
				for (var i = 0; i < len; i++) {
					var c = value[i];
					if (in_comment) {
						if (c === "/" && value[i - 1] === "*") in_comment = false;
					} else if (in_str) {
						if (in_str === c) in_str = false;
					} else if (c === "/" && value[i + 1] === "*") in_comment = true;
					else if (c === "\"" || c === "'") in_str = c;
					else if (c === "(") in_apo++;
					else if (c === ")") in_apo--;
					if (!in_comment && in_str === false && in_apo === 0) {
						if (c === ":" && name_index === -1) name_index = i;
						else if (c === ";" || i === len - 1) {
							if (name_index !== -1) {
								var name = to_css_name(value.substring(start_index, name_index).trim());
								if (!reserved_names.includes(name)) {
									if (c !== ";") i++;
									var property = value.substring(start_index, i).trim();
									new_style += " " + property + ";";
								}
							}
							start_index = i + 1;
							name_index = -1;
						}
					}
				}
			}
			if (normal_styles) new_style += append_styles(normal_styles);
			if (important_styles) new_style += append_styles(important_styles, true);
			new_style = new_style.trim();
			return new_style === "" ? null : new_style;
		}
		return value == null ? null : String(value);
	}
	var CLEAN = 1024;
	var DIRTY = 2048;
	var MAYBE_DIRTY = 4096;
	/**
	* 'Transparent' effects do not create a transition boundary.
	* This is on a block effect 99% of the time but may also be on a branch effect if its parent block effect was pruned
	*/
	var EFFECT_TRANSPARENT = 65536;
	var EFFECT_PRESERVED = 1 << 19;
	/** allow users to ignore aborted signal errors if `reason.name === 'StaleReactionError` */
	var STALE_REACTION = new class StaleReactionError extends Error {
		name = "StaleReactionError";
		message = "The reaction that called `getAbortSignal()` was re-run or destroyed";
	}();
	globalThis.document?.contentType;
	/**
	* `%name%(...)` can only be used during component initialisation
	* @param {string} name
	* @returns {never}
	*/
	function lifecycle_outside_component(name) {
		throw new Error(`https://svelte.dev/e/lifecycle_outside_component`);
	}
	/**
	* `setContext` must be called when a component first initializes, not in a subsequent effect or after an `await` expression
	* @returns {never}
	*/
	function set_context_after_init() {
		throw new Error(`https://svelte.dev/e/set_context_after_init`);
	}
	/** True if experimental.async=true */
	var async_mode_flag = false;
	function enable_async_mode_flag() {
		async_mode_flag = true;
	}
	/**
	* @typedef {{ p: Context | null, c: Map<unknown, unknown> | null }} Context
	*/
	/**
	* @param {Context} context
	* @returns {Map<unknown, unknown> | null}
	*/
	function get_parent_context(context) {
		let parent = context.p;
		while (parent !== null && parent.c === null) parent = parent.p;
		return parent?.c ?? null;
	}
	/**
	* @param {Context | null} context
	* @param {string} name
	* @returns {Map<unknown, unknown>}
	*/
	function get_or_init_context_map(context, name) {
		if (context === null) lifecycle_outside_component(name);
		return context.c ??= new Map(get_parent_context(context) || void 0);
	}
	/** @type {Array<() => void>} */
	var micro_tasks = [];
	function run_micro_tasks() {
		var tasks = micro_tasks;
		micro_tasks = [];
		run_all(tasks);
	}
	/**
	* @param {() => void} fn
	*/
	function queue_micro_task(fn) {
		if (micro_tasks.length === 0 && !is_flushing_sync) {
			var tasks = micro_tasks;
			queueMicrotask(() => {
				if (tasks === micro_tasks) run_micro_tasks();
			});
		}
		micro_tasks.push(fn);
	}
	~(DIRTY | MAYBE_DIRTY | CLEAN);
	/**
	* @template T
	* @param {() => T} fn
	*/
	function without_reactive_context(fn) {
		var previous_reaction = active_reaction;
		var previous_effect = active_effect;
		set_active_reaction(null);
		set_active_effect(null);
		try {
			return fn();
		} finally {
			set_active_reaction(previous_reaction);
			set_active_effect(previous_effect);
		}
	}
	var is_flushing_sync = false;
	/** @type {null | Reaction} */
	var active_reaction = null;
	/** @param {null | Reaction} reaction */
	function set_active_reaction(reaction) {
		active_reaction = reaction;
	}
	/** @type {null | Effect} */
	var active_effect = null;
	/** @param {null | Effect} effect */
	function set_active_effect(effect) {
		active_effect = effect;
	}
	var BLOCK_OPEN = `<!--[-->`;
	var BLOCK_OPEN_ELSE = `<!--[!-->`;
	var BLOCK_CLOSE = `<!--]-->`;
	var EMPTY_COMMENT = `<!---->`;
	/**
	* Attributes that are boolean, i.e. they are present or not present.
	*/
	var DOM_BOOLEAN_ATTRIBUTES = [
		"allowfullscreen",
		"async",
		"autofocus",
		"autoplay",
		"checked",
		"controls",
		"default",
		"disabled",
		"formnovalidate",
		"indeterminate",
		"inert",
		"ismap",
		"loop",
		"multiple",
		"muted",
		"nomodule",
		"novalidate",
		"open",
		"playsinline",
		"readonly",
		"required",
		"reversed",
		"seamless",
		"selected",
		"webkitdirectory",
		"defer",
		"disablepictureinpicture",
		"disableremoteplayback"
	];
	/**
	* Returns `true` if `name` is a boolean attribute
	* @param {string} name
	*/
	function is_boolean_attribute(name) {
		return DOM_BOOLEAN_ATTRIBUTES.includes(name);
	}
	[...DOM_BOOLEAN_ATTRIBUTES];
	/**
	* The node API `AsyncLocalStorage` is not available, but is required to use async server rendering.
	* @returns {never}
	*/
	function async_local_storage_unavailable() {
		const error = /* @__PURE__ */ new Error(`async_local_storage_unavailable\nThe node API \`AsyncLocalStorage\` is not available, but is required to use async server rendering.\nhttps://svelte.dev/e/async_local_storage_unavailable`);
		error.name = "Svelte error";
		throw error;
	}
	/**
	* Encountered asynchronous work while rendering synchronously.
	* @returns {never}
	*/
	function await_invalid() {
		const error = /* @__PURE__ */ new Error(`await_invalid\nEncountered asynchronous work while rendering synchronously.\nhttps://svelte.dev/e/await_invalid`);
		error.name = "Svelte error";
		throw error;
	}
	/**
	* The `html` property of server render results has been deprecated. Use `body` instead.
	* @returns {never}
	*/
	function html_deprecated() {
		const error = /* @__PURE__ */ new Error(`html_deprecated\nThe \`html\` property of server render results has been deprecated. Use \`body\` instead.\nhttps://svelte.dev/e/html_deprecated`);
		error.name = "Svelte error";
		throw error;
	}
	/**
	* `csp.nonce` was set while `csp.hash` was `true`. These options cannot be used simultaneously.
	* @returns {never}
	*/
	function invalid_csp() {
		const error = /* @__PURE__ */ new Error(`invalid_csp\n\`csp.nonce\` was set while \`csp.hash\` was \`true\`. These options cannot be used simultaneously.\nhttps://svelte.dev/e/invalid_csp`);
		error.name = "Svelte error";
		throw error;
	}
	/**
	* The `idPrefix` option cannot include `--`.
	* @returns {never}
	*/
	function invalid_id_prefix() {
		const error = /* @__PURE__ */ new Error(`invalid_id_prefix\nThe \`idPrefix\` option cannot include \`--\`.\nhttps://svelte.dev/e/invalid_id_prefix`);
		error.name = "Svelte error";
		throw error;
	}
	/**
	* Could not resolve `render` context.
	* @returns {never}
	*/
	function server_context_required() {
		const error = /* @__PURE__ */ new Error(`server_context_required\nCould not resolve \`render\` context.\nhttps://svelte.dev/e/server_context_required`);
		error.name = "Svelte error";
		throw error;
	}
	/** @import { SSRContext } from '#server' */
	/** @type {SSRContext | null} */
	var ssr_context = null;
	/** @param {SSRContext | null} v */
	function set_ssr_context(v) {
		ssr_context = v;
	}
	/**
	* @template T
	* @param {any} key
	* @returns {T}
	*/
	function getContext(key) {
		return get_or_init_context_map(ssr_context, "getContext").get(key);
	}
	/**
	* @template T
	* @param {any} key
	* @param {T} context
	* @returns {T}
	*/
	function setContext(key, context) {
		const context_map = get_or_init_context_map(ssr_context, "setContext");
		if (ssr_context.i) set_context_after_init();
		context_map.set(key, context);
		return context;
	}
	/**
	* @param {any} key
	* @returns {boolean}
	*/
	function hasContext(key) {
		return get_or_init_context_map(ssr_context, "hasContext").has(key);
	}
	/**
	* @param {Function} [fn]
	*/
	function push(fn) {
		ssr_context = {
			p: ssr_context,
			c: null,
			r: null,
			i: false
		};
	}
	function pop() {
		ssr_context = ssr_context.p;
	}
	/**
	* Wraps an `await` expression in such a way that the component context that was
	* active before the expression evaluated can be reapplied afterwards —
	* `await a + b()` becomes `(await $.save(a))() + b()`, meaning `b()` will have access
	* to the context of its component.
	* @template T
	* @param {Promise<T>} promise
	* @returns {Promise<() => T>}
	*/
	async function save(promise) {
		var previous_context = ssr_context;
		var value = await promise;
		return () => {
			ssr_context = previous_context;
			return value;
		};
	}
	/**
	* A `hydratable` value with key `%key%` was created, but at least part of it was not used during the render.
	* 
	* The `hydratable` was initialized in:
	* %stack%
	* @param {string} key
	* @param {string} stack
	*/
	function unresolved_hydratable(key, stack) {
		console.warn(`https://svelte.dev/e/unresolved_hydratable`);
	}
	/** @import { AsyncLocalStorage } from 'node:async_hooks' */
	/** @import { RenderContext } from '#server' */
	/** @type {Promise<void> | null} */
	var current_render = null;
	/** @type {RenderContext | null} */
	var context$1 = null;
	/** @returns {RenderContext} */
	function get_render_context() {
		const store = context$1 ?? als?.getStore();
		if (!store) server_context_required();
		return store;
	}
	/**
	* @template T
	* @param {() => Promise<T>} fn
	* @returns {Promise<T>}
	*/
	async function with_render_context(fn) {
		context$1 = { hydratable: {
			lookup: /* @__PURE__ */ new Map(),
			comparisons: [],
			unresolved_promises: /* @__PURE__ */ new Map()
		} };
		if (in_webcontainer()) {
			const { promise, resolve } = deferred();
			const previous_render = current_render;
			current_render = promise;
			await previous_render;
			return fn().finally(resolve);
		}
		try {
			if (als === null) async_local_storage_unavailable();
			return als.run(context$1, fn);
		} finally {
			context$1 = null;
		}
	}
	/** @type {AsyncLocalStorage<RenderContext | null> | null} */
	var als = null;
	/** @type {Promise<void> | null} */
	var als_import = null;
	/**
	*
	* @returns {Promise<void>}
	*/
	function init_render_context() {
		als_import ??= Promise.resolve().then(() => /* @__PURE__ */ __toESM(require__skgo_skgo_missing(), 1)).then((hooks) => {
			als = new hooks.AsyncLocalStorage();
		}).then(noop$1, noop$1);
		return als_import;
	}
	function in_webcontainer() {
		return !!globalThis.process?.versions?.webcontainer;
	}
	var text_encoder;
	var crypto;
	/** @param {string} module_name */
	var obfuscated_import = (module_name) => Promise.reject(/* @__PURE__ */ new Error("skgo: no dynamic import in the SSR engine"));
	/** @param {string} data */
	async function sha256(data) {
		text_encoder ??= new TextEncoder();
		crypto ??= globalThis.crypto?.subtle?.digest ? globalThis.crypto : (await obfuscated_import("node:crypto")).webcrypto;
		return base64_encode(await crypto.subtle.digest("SHA-256", text_encoder.encode(data)));
	}
	/**
	* @param {Uint8Array} bytes
	* @returns {string}
	*/
	function base64_encode(bytes) {
		if (globalThis.Buffer) return globalThis.Buffer.from(bytes).toString("base64");
		let binary = "";
		for (let i = 0; i < bytes.length; i++) binary += String.fromCharCode(bytes[i]);
		return btoa(binary);
	}
	/** @import { Component } from 'svelte' */
	/** @import { HydratableContext, SSRContext } from './types.js' */
	/** @import { Csp, RenderOutput, SyncRenderOutput, Sha256Source } from '../../server/public.js' */
	/** @import { MaybePromise } from '#shared' */
	/** @typedef {'head' | 'body'} RendererType */
	/** @typedef {{ [key in RendererType]: string }} AccumulatedContent */
	/**
	* @typedef {string | Renderer} RendererItem
	*/
	var RenderResult = class {
		/** @type {() => AccumulatedContent} */
		#render;
		/** @type {() => Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }>} */
		#render_async;
		/** @type {AccumulatedContent | undefined} */
		#sync;
		/** @type {{ script: '' }} */
		#hashes = { script: "" };
		/** @type {Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }> | undefined} */
		#promise;
		/**
		* @param {() => AccumulatedContent} render
		* @param {() => Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }>} render_async
		*/
		constructor(render, render_async) {
			this.#render = render;
			this.#render_async = render_async;
		}
		#get() {
			return this.#sync ??= this.#render();
		}
		get html() {
			return this.#get().body;
		}
		get head() {
			return this.#get().head;
		}
		get body() {
			return this.#get().body;
		}
		get hashes() {
			return this.#hashes;
		}
		/**
		* This is not type-safe, but honestly it's the best I can do right now, and it's a straightforward function.
		*
		* @template TResult1
		* @template [TResult2=never]
		* @param {(value: SyncRenderOutput) => TResult1} onfulfilled
		* @param {(reason: unknown) => TResult2} onrejected
		*/
		then(onfulfilled, onrejected) {
			if (!async_mode_flag) {
				const result = this.#get();
				const user_result = onfulfilled({
					head: result.head,
					body: result.body,
					html: result.body,
					hashes: { script: [] }
				});
				return Promise.resolve(user_result);
			}
			this.#promise ??= this.#render_async().then((result) => {
				Object.defineProperty(result, "html", { get: () => {
					html_deprecated();
				} });
				return result;
			});
			return this.#promise.then((result) => onfulfilled(result), onrejected);
		}
	};
	/**
	* Renderers are basically a tree of `string | Renderer`s, where each `Renderer` in the tree represents
	* work that may or may not have completed. A renderer can be {@link collect}ed to aggregate the
	* content from itself and all of its children, but this will throw if any of the children are
	* performing asynchronous work. To asynchronously collect a renderer, just `await` it.
	*
	* The `string` values within a renderer are always associated with the {@link type} of that renderer. To switch types,
	* call {@link child} with a different `type` argument.
	*/
	var Renderer = class Renderer {
		/**
		* The contents of the renderer.
		* @type {RendererItem[]}
		*/
		#out = [];
		/**
		* Any `onDestroy` callbacks registered during execution of this renderer.
		* @type {(() => void)[] | undefined}
		*/
		#on_destroy = void 0;
		/**
		* Whether this renderer is a component body.
		* @type {boolean}
		*/
		#is_component_body = false;
		/**
		* If set, this renderer is an error boundary. When async collection
		* of the children fails, the failed snippet is rendered instead.
		* @type {{
		* 	failed: (renderer: Renderer, error: unknown, reset: () => void) => void;
		* 	transformError: (error: unknown) => unknown;
		* 	context: SSRContext | null;
		* } | null}
		*/
		#boundary = null;
		/**
		* The type of string content that this renderer is accumulating.
		* @type {RendererType}
		*/
		type;
		/** @type {Renderer | undefined} */
		#parent;
		/**
		* Asynchronous work associated with this renderer
		* @type {Promise<void> | undefined}
		*/
		promise = void 0;
		/**
		* State which is associated with the content tree as a whole.
		* It will be re-exposed, uncopied, on all children.
		* @type {SSRState}
		* @readonly
		*/
		global;
		/**
		* State that is local to the branch it is declared in.
		* It will be shallow-copied to all children.
		*
		* @type {{ select_value: any, multiple: boolean }}
		*/
		local;
		/**
		* @param {SSRState} global
		* @param {Renderer | undefined} [parent]
		*/
		constructor(global, parent) {
			this.#parent = parent;
			this.global = global;
			this.local = parent ? { ...parent.local } : {
				select_value: void 0,
				multiple: false
			};
			this.type = parent ? parent.type : "body";
		}
		/**
		* @param {(renderer: Renderer) => void} fn
		*/
		head(fn) {
			const head = new Renderer(this.global, this);
			head.type = "head";
			this.#out.push(head);
			head.child(fn);
		}
		/**
		* @param {Array<Promise<void>>} blockers
		* @param {(renderer: Renderer) => void} fn
		*/
		async_block(blockers, fn) {
			this.#out.push(BLOCK_OPEN);
			this.async(blockers, fn);
			this.#out.push(BLOCK_CLOSE);
		}
		/**
		* @param {Array<Promise<void>>} blockers
		* @param {(renderer: Renderer) => void} fn
		*/
		async(blockers, fn) {
			let callback = fn;
			if (blockers.length > 0) {
				const context = ssr_context;
				callback = (renderer) => {
					return Promise.all(blockers).then(() => {
						const previous_context = ssr_context;
						try {
							set_ssr_context(context);
							return fn(renderer);
						} finally {
							set_ssr_context(previous_context);
						}
					});
				};
			}
			this.child(callback);
		}
		/**
		* @param {Array<() => void>} thunks
		*/
		run(thunks) {
			const context = ssr_context;
			let promise = Promise.resolve(thunks[0]());
			const promises = [promise];
			if (context !== null && thunks.length > 1) context.i = true;
			for (const fn of thunks.slice(1)) {
				promise = promise.then(() => {
					const previous_context = ssr_context;
					set_ssr_context(context);
					try {
						return fn();
					} finally {
						set_ssr_context(previous_context);
					}
				});
				promises.push(promise);
			}
			promise.catch(noop$1);
			this.promise = this.global.track(promise);
			return promises;
		}
		/**
		* @param {(renderer: Renderer) => MaybePromise<void>} fn
		*/
		child_block(fn) {
			this.#out.push(BLOCK_OPEN);
			this.child(fn);
			this.#out.push(BLOCK_CLOSE);
		}
		/**
		* Create a child renderer. The child renderer inherits the state from the parent,
		* but has its own content.
		* @param {(renderer: Renderer) => MaybePromise<void>} fn
		*/
		child(fn) {
			const child = new Renderer(this.global, this);
			this.#out.push(child);
			const parent = ssr_context;
			set_ssr_context({
				...ssr_context,
				p: parent,
				c: null,
				r: child,
				i: ssr_context?.i ?? false
			});
			const result = fn(child);
			set_ssr_context(parent);
			if (result instanceof Promise) {
				result.catch(noop$1);
				result.finally(() => set_ssr_context(null)).catch(noop$1);
				if (child.global.mode === "sync") await_invalid();
				child.promise = child.global.track(result);
			}
			return child;
		}
		/**
		* Render children inside an error boundary. If the children throw and the API-level
		* `transformError` transform handles the error (doesn't re-throw), the `failed` snippet is
		* rendered instead. Otherwise the error propagates.
		*
		* @param {{ failed?: (renderer: Renderer, error: unknown, reset: () => void) => void }} props
		* @param {(renderer: Renderer) => MaybePromise<void>} children_fn
		*/
		boundary(props, children_fn) {
			const child = new Renderer(this.global, this);
			this.#out.push(child);
			const parent_context = ssr_context;
			if (props.failed) child.#boundary = {
				failed: props.failed,
				transformError: this.global.transformError,
				context: parent_context
			};
			set_ssr_context({
				...ssr_context,
				p: parent_context,
				c: null,
				r: child,
				i: ssr_context?.i ?? false
			});
			try {
				const result = children_fn(child);
				set_ssr_context(parent_context);
				if (result instanceof Promise) {
					if (child.global.mode === "sync") await_invalid();
					result.catch(noop$1);
					child.promise = child.global.track(result);
				}
			} catch (error) {
				set_ssr_context(parent_context);
				const failed_snippet = props.failed;
				if (!failed_snippet) throw error;
				const result = this.global.transformError(error);
				child.#out.length = 0;
				child.#boundary = null;
				if (result instanceof Promise) {
					if (this.global.mode === "sync") await_invalid();
					child.promise = child.global.track(
						/** @type {Promise<unknown>} */
						result.then((transformed) => {
							set_ssr_context(parent_context);
							child.#out.push(Renderer.#serialize_failed_boundary(transformed));
							failed_snippet(child, transformed, noop$1);
							child.#out.push(BLOCK_CLOSE);
						})
					);
					child.promise.catch(noop$1);
				} else {
					child.#out.push(Renderer.#serialize_failed_boundary(result));
					failed_snippet(child, result, noop$1);
					child.#out.push(BLOCK_CLOSE);
				}
			}
		}
		/**
		* Create a component renderer. The component renderer inherits the state from the parent,
		* but has its own content. It is treated as an ordering boundary for ondestroy callbacks.
		* @param {(renderer: Renderer) => MaybePromise<void>} fn
		* @param {Function} [component_fn]
		* @returns {void}
		*/
		component(fn, component_fn) {
			push(component_fn);
			this.child((renderer) => {
				renderer.#is_component_body = true;
				return fn(renderer);
			});
			pop();
		}
		/**
		* @param {Record<string, any>} attrs
		* @param {(renderer: Renderer) => void} fn
		* @param {string | undefined} [css_hash]
		* @param {Record<string, boolean> | undefined} [classes]
		* @param {Record<string, string> | undefined} [styles]
		* @param {number | undefined} [flags]
		* @param {boolean | undefined} [is_rich]
		* @returns {void}
		*/
		select(attrs, fn, css_hash, classes, styles, flags, is_rich) {
			const { value, defaultValue, ...select_attrs } = attrs;
			if (select_attrs.multiple === "") select_attrs.multiple = true;
			this.push(`<select${attributes(select_attrs, css_hash, classes, styles, flags)}>`);
			this.child((renderer) => {
				renderer.local.select_value = value === void 0 ? defaultValue : value;
				renderer.local.multiple = !!select_attrs.multiple;
				fn(renderer);
			});
			this.push(`${is_rich ? "<!>" : ""}</select>`);
		}
		/**
		* @param {Record<string, any>} attrs
		* @param {string | number | boolean | ((renderer: Renderer) => void)} body
		* @param {string | undefined} [css_hash]
		* @param {Record<string, boolean> | undefined} [classes]
		* @param {Record<string, string> | undefined} [styles]
		* @param {number | undefined} [flags]
		* @param {boolean | undefined} [is_rich]
		*/
		option(attrs, body, css_hash, classes, styles, flags, is_rich) {
			this.#out.push(`<option${attributes(attrs, css_hash, classes, styles, flags)}`);
			/**
			* @param {Renderer} renderer
			* @param {any} value
			* @param {{ head?: string, body: any }} content
			*/
			const close = (renderer, value, { head, body }) => {
				if (has_own_property.call(attrs, "value")) value = attrs.value;
				var select_value = this.local.select_value;
				if (this.local.multiple && is_array(select_value) ? select_value.includes(value) : value === select_value) renderer.#out.push(" selected=\"\"");
				renderer.#out.push(`>${body}${is_rich ? "<!>" : ""}</option>`);
				if (head) renderer.head((child) => child.push(head));
			};
			if (typeof body === "function") this.child((renderer) => {
				const r = new Renderer(this.global, this);
				body(r);
				if (this.global.mode === "async") return r.#collect_content_async().then((content) => {
					close(renderer, content.body.replaceAll("<!---->", ""), content);
				});
				else {
					const content = r.#collect_content();
					close(renderer, content.body.replaceAll("<!---->", ""), content);
				}
			});
			else close(this, body, { body: escape_html$1(body) });
		}
		/**
		* @param {(renderer: Renderer) => void} fn
		*/
		title(fn) {
			const path = this.get_path();
			/** @param {string} head */
			const close = (head) => {
				this.global.set_title(head, path);
			};
			this.child((renderer) => {
				const r = new Renderer(renderer.global, renderer);
				fn(r);
				if (renderer.global.mode === "async") return r.#collect_content_async().then((content) => {
					close(content.head);
				});
				else {
					const content = r.#collect_content();
					close(content.head);
				}
			});
		}
		/**
		* @param {string | (() => Promise<string>)} content
		*/
		push(content) {
			if (typeof content === "function") this.child(async (renderer) => renderer.push(await content()));
			else this.#out.push(content);
		}
		/**
		* @param {() => void} fn
		*/
		on_destroy(fn) {
			(this.#on_destroy ??= []).push(fn);
		}
		/**
		* @returns {number[]}
		*/
		get_path() {
			return this.#parent ? [...this.#parent.get_path(), this.#parent.#out.indexOf(this)] : [];
		}
		/**
		* @deprecated this is needed for legacy component bindings
		*/
		copy() {
			const copy = new Renderer(this.global, this.#parent);
			copy.type = this.type;
			copy.#out = this.#out.map((item) => item instanceof Renderer ? item.copy() : item);
			copy.promise = this.promise;
			return copy;
		}
		/**
		* @param {Renderer} other
		* @deprecated this is needed for legacy component bindings
		*/
		subsume(other) {
			if (this.global.mode !== other.global.mode) throw new Error("invariant: A renderer cannot switch modes. If you're seeing this, there's a compiler bug. File an issue!");
			this.local = other.local;
			this.#out = other.#out.map((item, i) => {
				const current = this.#out[i];
				if (current instanceof Renderer && item instanceof Renderer) {
					current.subsume(item);
					return current;
				}
				return item;
			});
			this.promise = other.promise;
			this.type = other.type;
		}
		get length() {
			return this.#out.length;
		}
		/**
		* Creates the hydration comment that marks the start of a failed boundary.
		* The error is JSON-serialized and embedded inside an HTML comment for the client
		* to parse during hydration. The JSON is escaped to prevent `-->` or `<!--` sequences
		* from breaking out of the comment (XSS). Uses unicode escapes which `JSON.parse()`
		* handles transparently.
		* @param {unknown} error
		* @returns {string}
		*/
		static #serialize_failed_boundary(error) {
			return `<!--[?${JSON.stringify(error).replace(/>/g, "\\u003e").replace(/</g, "\\u003c")}-->`;
		}
		/**
		* Only available on the server and when compiling with the `server` option.
		* Takes a component and returns an object with `body` and `head` properties on it, which you can use to populate the HTML when server-rendering your app.
		* @template {Record<string, any>} Props
		* @param {Component<Props>} component
		* @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string; csp?: Csp }} [options]
		* @returns {RenderOutput}
		*/
		static render(component, options = {}) {
			return new RenderResult(() => Renderer.#render(component, options), () => init_render_context().then(() => with_render_context(() => Renderer.#render_async(component, options))));
		}
		/**
		* Collect all of the `onDestroy` callbacks registered during rendering. In an async context, this is only safe to call
		* after awaiting `collect_async`.
		*
		* Child renderers are "porous" and don't affect execution order, but component body renderers
		* create ordering boundaries. Within a renderer, callbacks run in order until hitting a component boundary.
		* @returns {Iterable<() => void>}
		*/
		*#collect_on_destroy() {
			for (const component of this.#traverse_components()) yield* component.#collect_ondestroy();
		}
		/**
		* Performs a depth-first search of renderers, yielding the deepest components first, then additional components as we backtrack up the tree.
		* @returns {Iterable<Renderer>}
		*/
		*#traverse_components() {
			for (const child of this.#out) if (typeof child !== "string") yield* child.#traverse_components();
			if (this.#is_component_body) yield this;
		}
		/**
		* @returns {Iterable<() => void>}
		*/
		*#collect_ondestroy() {
			if (this.#on_destroy) for (const fn of this.#on_destroy) yield fn;
			for (const child of this.#out) if (child instanceof Renderer && !child.#is_component_body) yield* child.#collect_ondestroy();
		}
		/**
		* Runs every `onDestroy` callback in this renderer tree. On a failed render,
		* cleanup errors are suppressed so they do not mask the render error.
		* @param {boolean} suppress_errors
		*/
		#run_on_destroy(suppress_errors) {
			let first_error;
			let has_error = false;
			for (const cleanup of this.#collect_on_destroy()) try {
				cleanup();
			} catch (error) {
				if (!suppress_errors && !has_error) {
					first_error = error;
					has_error = true;
				}
			}
			if (has_error) throw first_error;
		}
		/**
		* @param {'sync' | 'async'} mode
		* @param {{ idPrefix?: string; csp?: Csp; transformError?: (error: unknown) => unknown }} options
		* @returns {Renderer}
		*/
		static #create(mode, options) {
			if (options.idPrefix?.includes("--")) invalid_id_prefix();
			return new Renderer(new SSRState(mode, options.idPrefix ? options.idPrefix + "-" : "", options.csp, options.transformError));
		}
		/**
		* Render a component. Throws if any of the children are performing asynchronous work.
		*
		* @template {Record<string, any>} Props
		* @param {Component<Props>} component
		* @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string }} options
		* @returns {AccumulatedContent}
		*/
		static #render(component, options) {
			var previous_context = ssr_context;
			const renderer = Renderer.#create("sync", options);
			/** @type {AccumulatedContent | undefined} */
			let result;
			let render_error;
			let failed = false;
			try {
				try {
					Renderer.#open_render(renderer, component, options);
					result = Renderer.#close_render(renderer.#collect_content(), renderer);
				} catch (error) {
					render_error = error;
					failed = true;
				}
				renderer.#run_on_destroy(failed);
				if (failed) throw render_error;
				return result;
			} finally {
				renderer.global.abort();
				set_ssr_context(previous_context);
			}
		}
		/**
		* Render a component.
		*
		* @template {Record<string, any>} Props
		* @param {Component<Props>} component
		* @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string; csp?: Csp }} options
		* @returns {Promise<AccumulatedContent & { hashes: { script: Sha256Source[] } }>}
		*/
		static async #render_async(component, options) {
			const previous_context = ssr_context;
			const renderer = Renderer.#create("async", options);
			/** @type {(AccumulatedContent & { hashes: { script: Sha256Source[] } }) | undefined} */
			let result;
			let render_error;
			let failed = false;
			try {
				try {
					Renderer.#open_render(renderer, component, options);
					const content = await renderer.#collect_content_async();
					const hydratables = await renderer.#collect_hydratables();
					if (hydratables !== null) content.head = hydratables + content.head;
					result = Renderer.#close_render(content, renderer);
				} catch (error) {
					render_error = error;
					failed = true;
					renderer.global.abort();
					await renderer.global.settle();
				}
				renderer.#run_on_destroy(failed);
				if (failed) throw render_error;
				return result;
			} finally {
				set_ssr_context(previous_context);
				renderer.global.abort();
			}
		}
		/**
		* Collect all of the code from the `out` array and return it as a string, or a promise resolving to a string.
		* @param {AccumulatedContent} content
		* @returns {AccumulatedContent}
		*/
		#collect_content(content = {
			head: "",
			body: ""
		}) {
			for (const item of this.#out) if (typeof item === "string") content[this.type] += item;
			else if (item instanceof Renderer) item.#collect_content(content);
			return content;
		}
		/**
		* Collect all of the code from the `out` array and return it as a string.
		* @param {AccumulatedContent} content
		* @returns {Promise<AccumulatedContent>}
		*/
		async #collect_content_async(content = {
			head: "",
			body: ""
		}) {
			await this.promise;
			for (const item of this.#out) if (typeof item === "string") content[this.type] += item;
			else if (item instanceof Renderer) {
				if (item.#boundary) {
					/** @type {AccumulatedContent} */
					const boundary_content = {
						head: "",
						body: ""
					};
					try {
						await item.#collect_content_async(boundary_content);
						content.head += boundary_content.head;
						content.body += boundary_content.body;
					} catch (error) {
						const { context, failed, transformError } = item.#boundary;
						set_ssr_context(context);
						let promise = transformError(error);
						set_ssr_context(null);
						let transformed = await promise;
						set_ssr_context(context);
						const failed_renderer = new Renderer(item.global, item);
						failed_renderer.type = item.type;
						failed_renderer.#out.push(Renderer.#serialize_failed_boundary(transformed));
						failed(failed_renderer, transformed, noop$1);
						failed_renderer.#out.push(BLOCK_CLOSE);
						await failed_renderer.#collect_content_async(content);
					}
				} else await item.#collect_content_async(content);
			}
			return content;
		}
		async #collect_hydratables() {
			const ctx = get_render_context().hydratable;
			for (const [_, key] of ctx.unresolved_promises) unresolved_hydratable(key, ctx.lookup.get(key)?.stack ?? "<missing stack trace>");
			for (const comparison of ctx.comparisons) await comparison;
			return await this.#hydratable_block(ctx);
		}
		/**
		* @template {Record<string, any>} Props
		* @param {Renderer} renderer
		* @param {import('svelte').Component<Props>} component
		* @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string; csp?: Csp; transformError?: (error: unknown) => unknown }} options
		* @returns {void}
		*/
		static #open_render(renderer, component, options) {
			var previous_context = ssr_context;
			try {
				set_ssr_context({
					p: null,
					c: options.context ?? null,
					r: renderer,
					i: false
				});
				renderer.push(BLOCK_OPEN);
				component(renderer, options.props ?? {});
				renderer.push(BLOCK_CLOSE);
			} finally {
				set_ssr_context(previous_context);
			}
		}
		/**
		* @param {AccumulatedContent} content
		* @param {Renderer} renderer
		* @returns {AccumulatedContent & { hashes: { script: Sha256Source[] } }}
		*/
		static #close_render(content, renderer) {
			let head = content.head + renderer.global.get_title();
			let body = content.body;
			for (const { hash, code } of renderer.global.css) head += `<style id="${hash}">${code}</style>`;
			return {
				head,
				body,
				hashes: { script: renderer.global.csp.script_hashes }
			};
		}
		/**
		* @param {HydratableContext} ctx
		*/
		async #hydratable_block(ctx) {
			if (ctx.lookup.size === 0) return null;
			let entries = [];
			let has_promises = false;
			for (const [k, v] of ctx.lookup) {
				if (v.promises) {
					has_promises = true;
					for (const p of v.promises) await p;
				}
				entries.push(`[${uneval$1(k)},${v.serialized}]`);
			}
			let prelude = `const h = (window.__svelte ??= {}).h ??= new Map();`;
			if (has_promises) prelude = `const r = (v) => Promise.resolve(v);
				${prelude}`;
			const body = `
			{
				${prelude}

				for (const [k, v] of [
					${entries.join(",\n					")}
				]) {
					h.set(k, v);
				}
			}
		`;
			let csp_attr = "";
			if (this.global.csp.nonce) csp_attr = ` nonce="${this.global.csp.nonce}"`;
			else if (this.global.csp.hash) {
				const hash = await sha256(body);
				this.global.csp.script_hashes.push(`sha256-${hash}`);
			}
			return `\n\t\t<script${csp_attr}>${body}<\/script>`;
		}
	};
	var SSRState = class {
		/** @readonly @type {Csp & { script_hashes: Sha256Source[] }} */
		csp;
		/** @readonly @type {'sync' | 'async'} */
		mode;
		/** @readonly @type {() => string} */
		uid;
		/** @readonly @type {Set<{ hash: string; code: string }>} */
		css = /* @__PURE__ */ new Set();
		/** @type {Set<Promise<unknown>>} */
		#pending = /* @__PURE__ */ new Set();
		/** @type {AbortController | null} */
		#controller = null;
		#aborted = false;
		/**
		* `transformError` passed to `render`. Called when an error boundary catches an error.
		* Throws by default if unset in `render`.
		* @type {(error: unknown) => unknown}
		*/
		transformError;
		/** @type {{ path: number[], value: string }} */
		#title = {
			path: [],
			value: ""
		};
		/**
		* @param {'sync' | 'async'} mode
		* @param {string} id_prefix
		* @param {Csp} csp
		* @param {((error: unknown) => unknown) | undefined} [transformError]
		*/
		constructor(mode, id_prefix = "", csp = { hash: false }, transformError) {
			this.mode = mode;
			this.csp = {
				...csp,
				script_hashes: []
			};
			this.transformError = transformError ?? ((error) => {
				throw error;
			});
			let uid = 1;
			this.uid = () => `${id_prefix}s${uid++}`;
		}
		/**
		* @template T
		* @param {Promise<T>} promise
		* @returns {Promise<T>}
		*/
		track(promise) {
			this.#pending.add(promise);
			promise.then(() => this.#pending.delete(promise), () => this.#pending.delete(promise));
			return promise;
		}
		async settle() {
			while (this.#pending.size > 0) await Promise.allSettled([...this.#pending]);
		}
		abort() {
			if (this.#aborted) return;
			this.#aborted = true;
			this.#controller?.abort(STALE_REACTION);
		}
		get_abort_signal() {
			const controller = this.#controller ??= new AbortController();
			if (this.#aborted) controller.abort(STALE_REACTION);
			return controller.signal;
		}
		get_title() {
			return this.#title.value;
		}
		/**
		* Performs a depth-first (lexicographic) comparison using the path. Rejects sets
		* from earlier than or equal to the current value.
		* @param {string} value
		* @param {number[]} path
		*/
		set_title(value, path) {
			const current = this.#title.path;
			let i = 0;
			let l = Math.min(path.length, current.length);
			while (i < l && path[i] === current[i]) i += 1;
			if (path[i] === void 0) return;
			if (current[i] === void 0 || path[i] > current[i]) {
				this.#title.path = path;
				this.#title.value = value;
			}
		}
	};
	var INVALID_ATTR_NAME_CHAR_REGEX = /[\s'">/=\u{FDD0}-\u{FDEF}\u{FFFE}\u{FFFF}\u{1FFFE}\u{1FFFF}\u{2FFFE}\u{2FFFF}\u{3FFFE}\u{3FFFF}\u{4FFFE}\u{4FFFF}\u{5FFFE}\u{5FFFF}\u{6FFFE}\u{6FFFF}\u{7FFFE}\u{7FFFF}\u{8FFFE}\u{8FFFF}\u{9FFFE}\u{9FFFF}\u{AFFFE}\u{AFFFF}\u{BFFFE}\u{BFFFF}\u{CFFFE}\u{CFFFF}\u{DFFFE}\u{DFFFF}\u{EFFFE}\u{EFFFF}\u{FFFFE}\u{FFFFF}\u{10FFFE}\u{10FFFF}]/u;
	/**
	* Only available on the server and when compiling with the `server` option.
	* Takes a component and returns an object with `body` and `head` properties on it, which you can use to populate the HTML when server-rendering your app.
	* @template {Record<string, any>} Props
	* @param {Component<Props> | ComponentType<SvelteComponent<Props>>} component
	* @param {{ props?: Omit<Props, '$$slots' | '$$events'>; context?: Map<any, any>; idPrefix?: string; csp?: Csp; transformError?: (error: unknown) => unknown }} [options]
	* @returns {RenderOutput}
	*/
	function render(component, options = {}) {
		if (options.csp?.hash && options.csp.nonce) invalid_csp();
		return Renderer.render(component, options);
	}
	/**
	* @param {string} hash
	* @param {Renderer} renderer
	* @param {(renderer: Renderer) => Promise<void> | void} fn
	* @returns {void}
	*/
	function head(hash, renderer, fn) {
		renderer.head((renderer) => {
			renderer.push(`<!--${hash}-->`);
			renderer.child(fn);
			renderer.push(EMPTY_COMMENT);
		});
	}
	/**
	* @param {Record<string, unknown>} attrs
	* @param {string} [css_hash]
	* @param {Record<string, boolean>} [classes]
	* @param {Record<string, string>} [styles]
	* @param {number} [flags]
	* @returns {string}
	*/
	function attributes(attrs, css_hash, classes, styles, flags = 0) {
		if (styles) attrs.style = to_style(attrs.style, styles);
		if (attrs.class) attrs.class = clsx(attrs.class);
		if (css_hash || classes) attrs.class = to_class(attrs.class, css_hash, classes);
		let attr_str = "";
		let name;
		const is_html = (flags & 1) === 0;
		const lowercase = (flags & 2) === 0;
		const is_input = (flags & 4) !== 0;
		const names = Object.keys(attrs);
		outer: for (let i = 0; i < names.length; i++) {
			name = names[i];
			if (typeof attrs[name] === "function") continue;
			if (name[0] === "$" && name[1] === "$") continue;
			if (name === "" || INVALID_ATTR_NAME_CHAR_REGEX.test(name)) continue;
			var value = attrs[name];
			var lower = name.toLowerCase();
			if (lowercase) name = lower;
			if (lower.length > 2 && lower.startsWith("on")) continue;
			if (is_input) {
				if (name === "defaultvalue" || name === "defaultchecked") {
					name = name === "defaultvalue" ? "value" : "checked";
					if (name in attrs) continue;
					for (let j = 0; j < names.length; j++) if (names[j].toLowerCase() === name) continue outer;
				}
			}
			attr_str += attr(name, value, is_html && is_boolean_attribute(name));
		}
		return attr_str;
	}
	/**
	* @param {Record<string, unknown>[]} props
	* @returns {Record<string, unknown>}
	*/
	function spread_props(props) {
		/** @type {Record<string, unknown>} */
		const merged_props = {};
		let key;
		for (let i = 0; i < props.length; i++) {
			const obj = props[i];
			if (obj == null) continue;
			for (key of Object.keys(obj)) {
				const desc = Object.getOwnPropertyDescriptor(obj, key);
				if (desc) Object.defineProperty(merged_props, key, desc);
				else merged_props[key] = obj[key];
			}
		}
		return merged_props;
	}
	/**
	* @param {unknown} value
	* @returns {string}
	*/
	function stringify$1(value) {
		return typeof value === "string" ? value : value == null ? "" : value + "";
	}
	/**
	* @param {any} value
	* @param {string | undefined} [hash]
	* @param {Record<string, boolean>} [directives]
	*/
	function attr_class(value, hash, directives) {
		var result = to_class(value, hash, directives);
		return result ? ` class="${escape_html$1(result, true)}"` : "";
	}
	/**
	* Legacy mode: If the prop has a fallback and is bound in the
	* parent component, propagate the fallback value upwards.
	* @param {Record<string, unknown>} props_parent
	* @param {Record<string, unknown>} props_now
	*/
	function bind_props(props_parent, props_now) {
		for (const key of Object.keys(props_now)) {
			const initial_value = props_parent[key];
			const value = props_now[key];
			if (initial_value === void 0 && value !== void 0 && Object.getOwnPropertyDescriptor(props_parent, key)?.set) props_parent[key] = value;
		}
	}
	/**
	* @template V
	* @param {Renderer} renderer
	* @param {Promise<V>} promise
	* @param {null | (() => void)} pending_fn
	* @param {(value: V) => void} then_fn
	* @returns {void}
	*/
	function await_block(renderer, promise, pending_fn, then_fn) {
		if (is_promise(promise)) {
			renderer.push(BLOCK_OPEN);
			promise.then(null, noop$1);
			if (pending_fn !== null) pending_fn();
		} else if (then_fn !== null) {
			renderer.push(BLOCK_OPEN_ELSE);
			then_fn(promise);
		}
	}
	/** @param {any} array_like_or_iterator */
	function ensure_array_like(array_like_or_iterator) {
		if (array_like_or_iterator) return array_like_or_iterator.length !== void 0 ? array_like_or_iterator : Array.from(array_like_or_iterator);
		return [];
	}
	/**
	* @template V
	* @param {() => V} get_value
	*/
	function once(get_value) {
		let value = UNINITIALIZED;
		return () => {
			if (value === UNINITIALIZED) value = get_value();
			return value;
		};
	}
	/**
	* Create an unique ID
	* @param {Renderer} renderer
	* @returns {string}
	*/
	function props_id(renderer) {
		const uid = renderer.global.uid();
		renderer.push("<!--$" + uid + "-->");
		return uid;
	}
	/**
	* @template T
	* @param {()=>T} fn
	* @returns {(new_value?: T) => (T | void)}
	*/
	function derived(fn) {
		const get_value = ssr_context === null ? fn : once(fn);
		/** @type {T | undefined} */
		let updated_value;
		return function(new_value) {
			if (arguments.length === 0) return updated_value ?? get_value();
			updated_value = new_value;
			return updated_value;
		};
	}
	/**
	* @template T
	* @param {()=>T} fn
	*/
	function async_derived(fn) {
		return Promise.resolve(fn()).then((value) => {
			return () => value;
		});
	}
	enable_async_mode_flag();
	var afterNavigate = noop$2;
	function Root($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const { page, components, onerror, tree, form, error } = $$props;
			let mounted = false;
			let navigated = false;
			let title = "";
			afterNavigate(() => {
				if (mounted) {
					navigated = true;
					title = document.title || "untitled page";
				} else mounted = true;
			});
			function node($$renderer, n, depth) {
				const Component = derived(() => n.component);
				const Error = derived(() => n.error);
				const data = derived(() => n.data);
				function failed($$renderer, error) {
					if (Error()) {
						$$renderer.push("<!--[-->");
						Error()($$renderer, { error });
						$$renderer.push("<!--]-->");
					} else {
						$$renderer.push("<!--[!-->");
						$$renderer.push("<!--]-->");
					}
				}
				$$renderer.boundary({ failed: Error() ? failed : void 0 }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					if (n.child) {
						$$renderer.push("<!--[0-->");
						if (Component()) {
							$$renderer.push("<!--[-->");
							Component()($$renderer, {
								data: data(),
								form,
								params: page.params,
								children: ($$renderer) => {
									node($$renderer, n.child, depth + 1);
								},
								$$slots: { default: true }
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
					} else {
						$$renderer.push("<!--[-1-->");
						if (Component()) {
							$$renderer.push("<!--[-->");
							Component()($$renderer, {
								data: data(),
								form,
								params: page.params,
								error
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
					}
					$$renderer.push(`<!--]-->`);
					$$renderer.push(`<!--]-->`);
				});
			}
			node($$renderer, tree, 0);
			$$renderer.push(`<!----> `);
			if (mounted) {
				$$renderer.push(`<!--[0--><div id="svelte-announcer" aria-live="assertive" aria-atomic="true" style="position: absolute; left: 0; top: 0; clip: rect(0 0 0 0); clip-path: inset(50%); overflow: hidden; white-space: nowrap; width: 1px; height: 1px">`);
				if (navigated) $$renderer.push(`<!--[0-->${escape_html$1(title)}`);
				else $$renderer.push("<!--[-1-->");
				$$renderer.push(`<!--]--></div>`);
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	var Props = class {
		/** @type {Page} */
		page;
		/**
		* An array of the `+layout.svelte` and `+page.svelte` component instances
		* that currently live on the page — used for capturing and restoring snapshots.
		* It's updated/manipulated through `bind:this` in `Root.svelte`.
		* @type {Array<Record<string, any>>}
		* @deprecated only used for `export const snapshot` — TODO 4.0 get rid
		*/
		components = [];
		/** @type {any} */
		form;
		/** @type {App.Error | undefined} */
		error;
		/** @type {RenderNode} */
		tree;
		/** @type {(error: unknown, reset: () => void) => void} */
		onerror;
		/**
		* @param {{
		*   page: Page;
		*   tree: RenderNode;
		*   form: any;
		*   error: App.Error | undefined;
		*   onerror?: (error: unknown, reset: () => void) => void;
		* }} props
		*/
		constructor({ page, tree, form, error, onerror = noop$2 }) {
			this.page = page;
			this.tree = tree;
			this.onerror = onerror;
			this.form = form;
			this.error = error;
		}
	};
	var RenderNode = class {
		/** @type {Component} */
		component;
		/** @type {Component | undefined} */
		error;
		/** @type {Record<string, any>} */
		data = {};
		/** @type {RenderNode | undefined} */
		child;
		/**
		*
		* @param {Component} component
		* @param {Component | undefined} error
		*/
		constructor(component, error) {
			this.component = component;
			this.error = error;
		}
	};
	async function tick() {}
	function context() {
		return getContext("__request__");
	}
	var page = {
		get data() {
			return context().page.data;
		},
		get error() {
			return context().page.error;
		},
		get form() {
			return context().page.form;
		},
		get params() {
			return context().page.params;
		},
		get route() {
			return context().page.route;
		},
		get shallow() {
			return context().page.shallow;
		},
		get state() {
			return context().page.state;
		},
		get status() {
			return context().page.status;
		},
		get url() {
			return context().page.url;
		}
	};
	var Money;
	var transport$1;
	var init_hooks = __esmMin(() => {
		Money = class {
			cents;
			constructor(cents) {
				this.cents = cents;
			}
			format() {
				const sign = this.cents < 0 ? "-" : "";
				const cents = Math.abs(this.cents);
				return `${sign}$${Math.floor(cents / 100)}.${String(cents % 100).padStart(2, "0")}`;
			}
		};
		transport$1 = { Money: {
			encode: (value) => value instanceof Money && { cents: value.cents },
			decode: (data) => new Money(data.cents)
		} };
	});
	function MessagePreview($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<section aria-label="Message preview"><label>Preview message <input data-testid="preview-body"${attr("value", "")}/></label> <button type="button" data-testid="preview-message"${attr("disabled", previewMessage.pending > 0, true)}>Preview</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section>`);
		});
	}
	function ContactForms($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { showInbox = true } = $$props;
			let unreachable = false;
			const fields = sendMessage.fields;
			const keyed = sendMessage.for("k1");
			const keyedFields = keyed.fields;
			$$renderer.push(`<nav aria-label="Form callers"><a href="/typed-load/42">Numeric caller</a> <a href="/items/42">String caller</a> <a href="/contact">Absent caller</a></nav> `);
			MessagePreview($$renderer, {});
			$$renderer.push(`<!----> <form${attributes({
				"data-testid": "contact-form",
				enctype: "multipart/form-data",
				...sendMessage.enhance(async (form) => {
					unreachable = false;
					try {
						if (await form.submit().updates(getMessages())) form.element.reset();
					} catch {
						unreachable = true;
					}
				})
			}, "svelte-1he4tv0")}><label class="svelte-1he4tv0">Your name <input${attributes({
				"data-testid": "field-from",
				...fields.from.as("text")
			}, "svelte-1he4tv0", void 0, void 0, 4)}/></label> <!--[-->`);
			const each_array = ensure_array_like(fields.from.issues() ?? []);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let issue = each_array[$$index];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="issue-from">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label class="svelte-1he4tv0">Email <input${attributes({
				"data-testid": "field-email",
				...fields.email.as("text")
			}, "svelte-1he4tv0", void 0, void 0, 4)}/></label> <!--[-->`);
			const each_array_1 = ensure_array_like(fields.email.issues() ?? []);
			for (let $$index_1 = 0, $$length = each_array_1.length; $$index_1 < $$length; $$index_1++) {
				let issue = each_array_1[$$index_1];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="issue-email">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label class="svelte-1he4tv0">Message <textarea${attributes({
				"data-testid": "field-body",
				...fields.body.as("text")
			}, "svelte-1he4tv0")}></textarea></label> <!--[-->`);
			const each_array_2 = ensure_array_like(fields.body.issues() ?? []);
			for (let $$index_2 = 0, $$length = each_array_2.length; $$index_2 < $$length; $$index_2++) {
				let issue = each_array_2[$$index_2];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="issue-body">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label class="svelte-1he4tv0">Attachment <input${attributes({
				"data-testid": "field-attachment",
				...fields.attachment.as("file")
			}, "svelte-1he4tv0", void 0, void 0, 4)}/></label> <button data-testid="send" type="submit">Send</button></form> <p data-testid="contact-dirty" aria-live="polite">Changed fields:
  ${escape_html$1([
				fields.from.dirty() && "name",
				fields.email.dirty() && "email",
				fields.body.dirty() && "message",
				fields.attachment.dirty() && "attachment"
			].filter(Boolean).join(", ") || "none")}</p> `);
			if (sendMessage.result) $$renderer.push(`<!--[0--><p data-testid="receipt">${escape_html$1(sendMessage.result.summary)}</p> <p data-testid="receipt-caller">${escape_html$1(sendMessage.result.caller)}</p> <p data-testid="receipt-body">${escape_html$1(sendMessage.result.body)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if ((sendMessage.fields.allIssues() ?? []).length > 0) $$renderer.push(`<!--[0--><p data-testid="rejected">That message was not sent.</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if (unreachable) $$renderer.push(`<!--[0--><p data-testid="unreachable">The server could not be reached.</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <h2>Reply (keyed)</h2> <form${attributes({
				"data-testid": "keyed-contact-form",
				...keyed.enhance(async (form) => {
					unreachable = false;
					try {
						if (await form.submit().updates(getMessages())) form.element.reset();
					} catch {
						unreachable = true;
					}
				})
			}, "svelte-1he4tv0")}><label class="svelte-1he4tv0">Your name <input${attributes({
				"data-testid": "keyed-field-from",
				...keyedFields.from.as("text")
			}, "svelte-1he4tv0", void 0, void 0, 4)}/></label> <!--[-->`);
			const each_array_3 = ensure_array_like(keyedFields.from.issues() ?? []);
			for (let $$index_3 = 0, $$length = each_array_3.length; $$index_3 < $$length; $$index_3++) {
				let issue = each_array_3[$$index_3];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="keyed-issue-from">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label class="svelte-1he4tv0">Email <input${attributes({
				"data-testid": "keyed-field-email",
				...keyedFields.email.as("text")
			}, "svelte-1he4tv0", void 0, void 0, 4)}/></label> <!--[-->`);
			const each_array_4 = ensure_array_like(keyedFields.email.issues() ?? []);
			for (let $$index_4 = 0, $$length = each_array_4.length; $$index_4 < $$length; $$index_4++) {
				let issue = each_array_4[$$index_4];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="keyed-issue-email">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label class="svelte-1he4tv0">Message <textarea${attributes({
				"data-testid": "keyed-field-body",
				...keyedFields.body.as("text")
			}, "svelte-1he4tv0")}></textarea></label> <!--[-->`);
			const each_array_5 = ensure_array_like(keyedFields.body.issues() ?? []);
			for (let $$index_5 = 0, $$length = each_array_5.length; $$index_5 < $$length; $$index_5++) {
				let issue = each_array_5[$$index_5];
				$$renderer.push(`<p class="issue svelte-1he4tv0" data-testid="keyed-issue-body">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <button data-testid="keyed-send" type="submit">Send</button></form> `);
			if (keyed.result) $$renderer.push(`<!--[0--><p data-testid="keyed-receipt">${escape_html$1(keyed.result.summary)}</p> <p data-testid="keyed-receipt-caller">${escape_html$1(keyed.result.caller)}</p> <p data-testid="keyed-receipt-body">${escape_html$1(keyed.result.body)}</p> <p data-testid="keyed-receipt-key">carried key: ${escape_html$1(keyed.result.key)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if ((keyed.fields.allIssues() ?? []).length > 0) $$renderer.push(`<!--[0--><p data-testid="keyed-rejected">That message was not sent.</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if (showInbox) {
				$$renderer.push(`<!--[0--><h2>Inbox</h2> `);
				{
					function failed($$renderer, error) {
						$$renderer.push(`<p data-testid="messages-failed">${escape_html$1(error.message)}</p>`);
					}
					$$renderer.boundary({ failed }, ($$renderer) => {
						$$renderer.push(`<!--[!-->`);
						$$renderer.push(`<p data-testid="messages-pending">loading…</p>`);
						$$renderer.push(`<!--]-->`);
					});
				}
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	function SignIn($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let name = "";
			let busy = false;
			const session = whoami();
			$$renderer.child_block(async ($$renderer) => {
				if ((await save(session))().user) {
					$$renderer.push(`<!--[0--><span data-testid="session">Signed in as `);
					$$renderer.push(async () => escape_html$1((await save(session))().user));
					$$renderer.push(`</span> <button data-testid="sign-out"${attr("disabled", busy, true)}>Sign out</button>`);
				} else $$renderer.push(`<!--[-1--><span data-testid="session">Signed out</span> <form><input data-testid="user" name="user"${attr("value", name)} placeholder="your name"/> <button data-testid="sign-in" type="submit"${attr("disabled", busy, true)}>Sign in</button></form>`);
			});
			$$renderer.push(`<!--]-->`);
		});
	}
	function _layout$2($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, children } = $$props;
			/**
			* The root layout is the one node in a branch that no `+error.svelte` can
			* guard: kit wraps the root error page *inside* this layout rather than
			* this layout inside it (`runtime/error-chain.js`), so the boundary around
			* it has no `failed` snippet and a throw here has nowhere to go. That makes
			* it the only place a page can be made to fail the way an engine failure
			* fails, and /error/render is the fixture that does it.
			*/
			const failTheWholeDocument = () => {
				throw new Error("skgo: the root layout cannot render /error/render");
			};
			if (page.route.id === "/error/render") $$renderer.push(`<!--[0-->${escape_html$1(failTheWholeDocument())}`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <nav data-testid="app-nav" class="svelte-12qhfyh"><a href="/" class="svelte-12qhfyh">Home</a> <a href="/about" class="svelte-12qhfyh">About</a> <a href="/items/42" class="svelte-12qhfyh">Item 42</a> <a href="/items/42?retain=1" class="svelte-12qhfyh">Item 42 retained</a> <a href="/todos" class="svelte-12qhfyh">Todos</a> <a href="/empty" class="svelte-12qhfyh">Empty</a> <a href="/api" class="svelte-12qhfyh">API</a> <a href="/fetch" class="svelte-12qhfyh">Server fetch</a> <a href="/request-fetch" class="svelte-12qhfyh">Request fetch</a> <a href="/nested-universal" class="svelte-12qhfyh">Nested fetch</a> <a href="/destinations" class="svelte-12qhfyh">Destinations</a> <a href="/pricing" class="svelte-12qhfyh">Pricing</a> <a href="/actions" class="svelte-12qhfyh">Actions</a> <a href="/docs/guide/getting-started" class="svelte-12qhfyh">Docs</a> <a href="/account" class="svelte-12qhfyh">Account</a> <a href="/stream" class="svelte-12qhfyh">Stream</a> <a href="/live" class="svelte-12qhfyh">Live</a> <a href="/batch" class="svelte-12qhfyh">Batch</a></nav> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="session-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					$$renderer.push("<!--[0-->");
					SignIn($$renderer, {});
					$$renderer.push(`<!--]-->`);
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` `);
			if (data.scriptSafe) $$renderer.push(`<!--[0--><p data-testid="script-safe" class="svelte-12qhfyh">${escape_html$1(data.scriptSafe)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <main class="svelte-12qhfyh">`);
			children($$renderer);
			$$renderer.push(`<!----> `);
			if (page.route.id === "/contact" || page.route.id === "/items/[id]" || page.route.id === "/typed-load/[number=Order]") {
				$$renderer.push("<!--[0-->");
				ContactForms($$renderer, { showInbox: page.route.id === "/contact" });
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></main> <footer data-testid="deployment">${escape_html$1(data.deployment)}</footer>`);
		});
	}
	function Error$1($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1>${escape_html$1(page.status)}</h1> <p>${escape_html$1(page.error?.message)}</p>`);
		});
	}
	function _layout$1($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, children } = $$props;
			$$renderer.push(`<section><p data-testid="account-user">Account of ${escape_html$1(data.accountUser)}</p> <p data-testid="account-serial">${escape_html$1(data.accountSerial)}</p> <nav><a href="/account">Overview</a> <a href="/account/orders">Orders</a></nav> <button data-testid="refresh-account">Refresh the account</button> `);
			children($$renderer);
			$$renderer.push(`<!----></section>`);
		});
	}
	function _error$3($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Account error ${escape_html$1(page.status)}</h1> <p data-testid="error-message">${escape_html$1(page.error?.message)}</p>`);
		});
	}
	function _error$2($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			head("diuh45", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Action error | skgo example</title>`);
				});
			});
			$$renderer.push(`<section class="card svelte-diuh45"><h1 data-testid="action-error-title">Action error ${escape_html$1(page.status)}</h1> <p data-testid="action-error-message">${escape_html$1(page.error?.message)}</p> `);
			if (page.error?.supportId) $$renderer.push(`<!--[0--><p data-testid="action-error-support-id">${escape_html$1(page.error.supportId)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <p><a data-testid="action-error-return" href="/actions">Return to the profile editor</a></p> <button data-testid="client-interaction" type="button" class="svelte-diuh45">Check client interaction</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section>`);
		});
	}
	function _error$1($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			head("mroyd2", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Cross-page action error | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-mroyd2"><h1 data-testid="cross-error-title">Cross-page action error ${escape_html$1(page.status)}</h1> <noscript><p data-testid="cross-error-noscript">JavaScript is disabled. This error page was rendered by Go.</p></noscript> <p data-testid="cross-error-message">${escape_html$1(page.error?.message)}</p> `);
			if (page.error?.supportId) $$renderer.push(`<!--[0--><p data-testid="cross-error-support-id">${escape_html$1(page.error.supportId)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <p><a data-testid="cross-error-return" href="/actions/cross/send">Return to cross-page forms</a></p> <button data-testid="client-interaction" type="button" class="svelte-mroyd2">Check client interaction</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _error($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Error ${escape_html$1(page.status)}</h1> <p data-testid="error-message">${escape_html$1(page.error?.message)}</p> `);
			if (page.error?.supportId) $$renderer.push(`<!--[0--><p data-testid="error-support-id">${escape_html$1(page.error.supportId)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	function _layout($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, children } = $$props;
			$$renderer.push(`<h1>Typed layout request event</h1> <p data-testid="event-layout-values">${escape_html$1(data.eventSection)}|${escape_html$1(data.eventItem)}|${escape_html$1(data.eventRoute)}|${escape_html$1(data.eventUser)}|${escape_html$1(data.eventSerial)}</p> <nav class="svelte-wms7ux"><a href="/event-layout/red/first/two?route=untracked">Unread sibling</a> <a href="/event-layout/red/second/two?route=untracked">Unread item</a> <a href="/event-layout/blue/second/two?route=untracked">Tracked section</a> <a href="/event-layout/blue/second/two?route=tracked">Track route</a> <a href="/event-layout/blue/second/one?route=tracked">Tracked sibling</a> <a href="/event-layout/blue/third/one?route=tracked">Tracked item</a></nav> `);
			children($$renderer);
			$$renderer.push(`<!---->`);
		});
	}
	function Greeting($$renderer, $$props) {
		let { name = "world" } = $$props;
		$$renderer.push(`<p data-testid="greeting">Hello, ${escape_html$1(name)}! This is Svelte, served by Go.</p>`);
	}
	var ALIAS = Symbol.for("yaml.alias");
	var DOC = Symbol.for("yaml.document");
	var MAP = Symbol.for("yaml.map");
	var PAIR = Symbol.for("yaml.pair");
	var SCALAR$1 = Symbol.for("yaml.scalar");
	var SEQ = Symbol.for("yaml.seq");
	var NODE_TYPE = Symbol.for("yaml.node.type");
	var isAlias = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === ALIAS;
	var isDocument$1 = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === DOC;
	var isMap = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === MAP;
	var isPair = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === PAIR;
	var isScalar = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === SCALAR$1;
	var isSeq = (node) => !!node && typeof node === "object" && node[NODE_TYPE] === SEQ;
	function isCollection(node) {
		if (node && typeof node === "object") switch (node[NODE_TYPE]) {
			case MAP:
			case SEQ: return true;
		}
		return false;
	}
	function isNode$1(node) {
		if (node && typeof node === "object") switch (node[NODE_TYPE]) {
			case ALIAS:
			case MAP:
			case SCALAR$1:
			case SEQ: return true;
		}
		return false;
	}
	var hasAnchor = (node) => (isScalar(node) || isCollection(node)) && !!node.anchor;
	var BREAK$1 = Symbol("break visit");
	var SKIP$1 = Symbol("skip children");
	var REMOVE$1 = Symbol("remove node");
	/**
	* Apply a visitor to an AST node or document.
	*
	* Walks through the tree (depth-first) starting from `node`, calling a
	* `visitor` function with three arguments:
	*   - `key`: For sequence values and map `Pair`, the node's index in the
	*     collection. Within a `Pair`, `'key'` or `'value'`, correspondingly.
	*     `null` for the root node.
	*   - `node`: The current node.
	*   - `path`: The ancestry of the current node.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this node, continue with next
	*     sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current node, then continue with the next one
	*   - `Node`: Replace the current node, then continue by visiting it
	*   - `number`: While iterating the items of a sequence or map, set the index
	*     of the next step. This is useful especially if the index of the current
	*     node has changed.
	*
	* If `visitor` is a single function, it will be called with all values
	* encountered in the tree, including e.g. `null` values. Alternatively,
	* separate visitor functions may be defined for each `Map`, `Pair`, `Seq`,
	* `Alias` and `Scalar` node. To define the same visitor function for more than
	* one node type, use the `Collection` (map and seq), `Value` (map, seq & scalar)
	* and `Node` (alias, map, seq & scalar) targets. Of all these, only the most
	* specific defined one will be used for each node.
	*/
	function visit$1(node, visitor) {
		const visitor_ = initVisitor(visitor);
		if (isDocument$1(node)) {
			if (visit_(null, node.contents, visitor_, Object.freeze([node])) === REMOVE$1) node.contents = null;
		} else visit_(null, node, visitor_, Object.freeze([]));
	}
	/** Terminate visit traversal completely */
	visit$1.BREAK = BREAK$1;
	/** Do not visit the children of the current node */
	visit$1.SKIP = SKIP$1;
	/** Remove the current node */
	visit$1.REMOVE = REMOVE$1;
	function visit_(key, node, visitor, path) {
		const ctrl = callVisitor(key, node, visitor, path);
		if (isNode$1(ctrl) || isPair(ctrl)) {
			replaceNode(key, path, ctrl);
			return visit_(key, ctrl, visitor, path);
		}
		if (typeof ctrl !== "symbol") {
			if (isCollection(node)) {
				path = Object.freeze(path.concat(node));
				for (let i = 0; i < node.items.length; ++i) {
					const ci = visit_(i, node.items[i], visitor, path);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK$1) return BREAK$1;
					else if (ci === REMOVE$1) {
						node.items.splice(i, 1);
						i -= 1;
					}
				}
			} else if (isPair(node)) {
				path = Object.freeze(path.concat(node));
				const ck = visit_("key", node.key, visitor, path);
				if (ck === BREAK$1) return BREAK$1;
				else if (ck === REMOVE$1) node.key = null;
				const cv = visit_("value", node.value, visitor, path);
				if (cv === BREAK$1) return BREAK$1;
				else if (cv === REMOVE$1) node.value = null;
			}
		}
		return ctrl;
	}
	/**
	* Apply an async visitor to an AST node or document.
	*
	* Walks through the tree (depth-first) starting from `node`, calling a
	* `visitor` function with three arguments:
	*   - `key`: For sequence values and map `Pair`, the node's index in the
	*     collection. Within a `Pair`, `'key'` or `'value'`, correspondingly.
	*     `null` for the root node.
	*   - `node`: The current node.
	*   - `path`: The ancestry of the current node.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `Promise`: Must resolve to one of the following values
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this node, continue with next
	*     sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current node, then continue with the next one
	*   - `Node`: Replace the current node, then continue by visiting it
	*   - `number`: While iterating the items of a sequence or map, set the index
	*     of the next step. This is useful especially if the index of the current
	*     node has changed.
	*
	* If `visitor` is a single function, it will be called with all values
	* encountered in the tree, including e.g. `null` values. Alternatively,
	* separate visitor functions may be defined for each `Map`, `Pair`, `Seq`,
	* `Alias` and `Scalar` node. To define the same visitor function for more than
	* one node type, use the `Collection` (map and seq), `Value` (map, seq & scalar)
	* and `Node` (alias, map, seq & scalar) targets. Of all these, only the most
	* specific defined one will be used for each node.
	*/
	async function visitAsync(node, visitor) {
		const visitor_ = initVisitor(visitor);
		if (isDocument$1(node)) {
			if (await visitAsync_(null, node.contents, visitor_, Object.freeze([node])) === REMOVE$1) node.contents = null;
		} else await visitAsync_(null, node, visitor_, Object.freeze([]));
	}
	/** Terminate visit traversal completely */
	visitAsync.BREAK = BREAK$1;
	/** Do not visit the children of the current node */
	visitAsync.SKIP = SKIP$1;
	/** Remove the current node */
	visitAsync.REMOVE = REMOVE$1;
	async function visitAsync_(key, node, visitor, path) {
		const ctrl = await callVisitor(key, node, visitor, path);
		if (isNode$1(ctrl) || isPair(ctrl)) {
			replaceNode(key, path, ctrl);
			return visitAsync_(key, ctrl, visitor, path);
		}
		if (typeof ctrl !== "symbol") {
			if (isCollection(node)) {
				path = Object.freeze(path.concat(node));
				for (let i = 0; i < node.items.length; ++i) {
					const ci = await visitAsync_(i, node.items[i], visitor, path);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK$1) return BREAK$1;
					else if (ci === REMOVE$1) {
						node.items.splice(i, 1);
						i -= 1;
					}
				}
			} else if (isPair(node)) {
				path = Object.freeze(path.concat(node));
				const ck = await visitAsync_("key", node.key, visitor, path);
				if (ck === BREAK$1) return BREAK$1;
				else if (ck === REMOVE$1) node.key = null;
				const cv = await visitAsync_("value", node.value, visitor, path);
				if (cv === BREAK$1) return BREAK$1;
				else if (cv === REMOVE$1) node.value = null;
			}
		}
		return ctrl;
	}
	function initVisitor(visitor) {
		if (typeof visitor === "object" && (visitor.Collection || visitor.Node || visitor.Value)) return Object.assign({
			Alias: visitor.Node,
			Map: visitor.Node,
			Scalar: visitor.Node,
			Seq: visitor.Node
		}, visitor.Value && {
			Map: visitor.Value,
			Scalar: visitor.Value,
			Seq: visitor.Value
		}, visitor.Collection && {
			Map: visitor.Collection,
			Seq: visitor.Collection
		}, visitor);
		return visitor;
	}
	function callVisitor(key, node, visitor, path) {
		if (typeof visitor === "function") return visitor(key, node, path);
		if (isMap(node)) return visitor.Map?.(key, node, path);
		if (isSeq(node)) return visitor.Seq?.(key, node, path);
		if (isPair(node)) return visitor.Pair?.(key, node, path);
		if (isScalar(node)) return visitor.Scalar?.(key, node, path);
		if (isAlias(node)) return visitor.Alias?.(key, node, path);
	}
	function replaceNode(key, path, node) {
		const parent = path[path.length - 1];
		if (isCollection(parent)) parent.items[key] = node;
		else if (isPair(parent)) {
			if (key === "key") parent.key = node;
			else parent.value = node;
		} else if (isDocument$1(parent)) parent.contents = node;
		else {
			const pt = isAlias(parent) ? "alias" : "scalar";
			throw new Error(`Cannot replace node with ${pt} parent`);
		}
	}
	var escapeChars = {
		"!": "%21",
		",": "%2C",
		"[": "%5B",
		"]": "%5D",
		"{": "%7B",
		"}": "%7D"
	};
	var escapeTagName = (tn) => tn.replace(/[!,[\]{}]/g, (ch) => escapeChars[ch]);
	var Directives = class Directives {
		constructor(yaml, tags) {
			/**
			* The directives-end/doc-start marker `---`. If `null`, a marker may still be
			* included in the document's stringified representation.
			*/
			this.docStart = null;
			/** The doc-end marker `...`.  */
			this.docEnd = false;
			this.yaml = Object.assign({}, Directives.defaultYaml, yaml);
			this.tags = Object.assign({}, Directives.defaultTags, tags);
		}
		clone() {
			const copy = new Directives(this.yaml, this.tags);
			copy.docStart = this.docStart;
			return copy;
		}
		/**
		* During parsing, get a Directives instance for the current document and
		* update the stream state according to the current version's spec.
		*/
		atDocument() {
			const res = new Directives(this.yaml, this.tags);
			switch (this.yaml.version) {
				case "1.1":
					this.atNextDocument = true;
					break;
				case "1.2":
					this.atNextDocument = false;
					this.yaml = {
						explicit: Directives.defaultYaml.explicit,
						version: "1.2"
					};
					this.tags = Object.assign({}, Directives.defaultTags);
			}
			return res;
		}
		/**
		* @param onError - May be called even if the action was successful
		* @returns `true` on success
		*/
		add(line, onError) {
			if (this.atNextDocument) {
				this.yaml = {
					explicit: Directives.defaultYaml.explicit,
					version: "1.1"
				};
				this.tags = Object.assign({}, Directives.defaultTags);
				this.atNextDocument = false;
			}
			const parts = line.trim().split(/[ \t]+/);
			const name = parts.shift();
			switch (name) {
				case "%TAG": {
					if (parts.length !== 2) {
						onError(0, "%TAG directive should contain exactly two parts");
						if (parts.length < 2) return false;
					}
					const [handle, prefix] = parts;
					this.tags[handle] = prefix;
					return true;
				}
				case "%YAML": {
					this.yaml.explicit = true;
					if (parts.length !== 1) {
						onError(0, "%YAML directive should contain exactly one part");
						return false;
					}
					const [version] = parts;
					if (version === "1.1" || version === "1.2") {
						this.yaml.version = version;
						return true;
					} else {
						const isValid = /^\d+\.\d+$/.test(version);
						onError(6, `Unsupported YAML version ${version}`, isValid);
						return false;
					}
				}
				default:
					onError(0, `Unknown directive ${name}`, true);
					return false;
			}
		}
		/**
		* Resolves a tag, matching handles to those defined in %TAG directives.
		*
		* @returns Resolved tag, which may also be the non-specific tag `'!'` or a
		*   `'!local'` tag, or `null` if unresolvable.
		*/
		tagName(source, onError) {
			if (source === "!") return "!";
			if (source[0] !== "!") {
				onError(`Not a valid tag: ${source}`);
				return null;
			}
			if (source[1] === "<") {
				const verbatim = source.slice(2, -1);
				if (verbatim === "!" || verbatim === "!!") {
					onError(`Verbatim tags aren't resolved, so ${source} is invalid.`);
					return null;
				}
				if (source[source.length - 1] !== ">") onError("Verbatim tags must end with a >");
				return verbatim;
			}
			const [, handle, suffix] = source.match(/^(.*!)([^!]*)$/s);
			if (!suffix) onError(`The ${source} tag has no suffix`);
			const prefix = this.tags[handle];
			if (prefix) try {
				return prefix + decodeURIComponent(suffix);
			} catch (error) {
				onError(String(error));
				return null;
			}
			if (handle === "!") return source;
			onError(`Could not resolve tag: ${source}`);
			return null;
		}
		/**
		* Given a fully resolved tag, returns its printable string form,
		* taking into account current tag prefixes and defaults.
		*/
		tagString(tag) {
			for (const [handle, prefix] of Object.entries(this.tags)) if (tag.startsWith(prefix)) return handle + escapeTagName(tag.substring(prefix.length));
			return tag[0] === "!" ? tag : `!<${tag}>`;
		}
		toString(doc) {
			const lines = this.yaml.explicit ? [`%YAML ${this.yaml.version || "1.2"}`] : [];
			const tagEntries = Object.entries(this.tags);
			let tagNames;
			if (doc && tagEntries.length > 0 && isNode$1(doc.contents)) {
				const tags = {};
				visit$1(doc.contents, (_key, node) => {
					if (isNode$1(node) && node.tag) tags[node.tag] = true;
				});
				tagNames = Object.keys(tags);
			} else tagNames = [];
			for (const [handle, prefix] of tagEntries) {
				if (handle === "!!" && prefix === "tag:yaml.org,2002:") continue;
				if (!doc || tagNames.some((tn) => tn.startsWith(prefix))) lines.push(`%TAG ${handle} ${prefix}`);
			}
			return lines.join("\n");
		}
	};
	Directives.defaultYaml = {
		explicit: false,
		version: "1.2"
	};
	Directives.defaultTags = { "!!": "tag:yaml.org,2002:" };
	/**
	* Verify that the input string is a valid anchor.
	*
	* Will throw on errors.
	*/
	function anchorIsValid(anchor) {
		if (/[\x00-\x19\s,[\]{}]/.test(anchor)) {
			const msg = `Anchor must not contain whitespace or control characters: ${JSON.stringify(anchor)}`;
			throw new Error(msg);
		}
		return true;
	}
	function anchorNames(root) {
		const anchors = /* @__PURE__ */ new Set();
		visit$1(root, { Value(_key, node) {
			if (node.anchor) anchors.add(node.anchor);
		} });
		return anchors;
	}
	/** Find a new anchor name with the given `prefix` and a one-indexed suffix. */
	function findNewAnchor(prefix, exclude) {
		for (let i = 1;; ++i) {
			const name = `${prefix}${i}`;
			if (!exclude.has(name)) return name;
		}
	}
	function createNodeAnchors(doc, prefix) {
		const aliasObjects = [];
		const sourceObjects = /* @__PURE__ */ new Map();
		let prevAnchors = null;
		return {
			onAnchor: (source) => {
				aliasObjects.push(source);
				prevAnchors ?? (prevAnchors = anchorNames(doc));
				const anchor = findNewAnchor(prefix, prevAnchors);
				prevAnchors.add(anchor);
				return anchor;
			},
			/**
			* With circular references, the source node is only resolved after all
			* of its child nodes are. This is why anchors are set only after all of
			* the nodes have been created.
			*/
			setAnchors: () => {
				for (const source of aliasObjects) {
					const ref = sourceObjects.get(source);
					if (typeof ref === "object" && ref.anchor && (isScalar(ref.node) || isCollection(ref.node))) ref.node.anchor = ref.anchor;
					else {
						const error = /* @__PURE__ */ new Error("Failed to resolve repeated object (this should not happen)");
						error.source = source;
						throw error;
					}
				}
			},
			sourceObjects
		};
	}
	/**
	* Applies the JSON.parse reviver algorithm as defined in the ECMA-262 spec,
	* in section 24.5.1.1 "Runtime Semantics: InternalizeJSONProperty" of the
	* 2021 edition: https://tc39.es/ecma262/#sec-json.parse
	*
	* Includes extensions for handling Map and Set objects.
	*/
	function applyReviver(reviver, obj, key, val) {
		if (val && typeof val === "object") {
			if (Array.isArray(val)) for (let i = 0, len = val.length; i < len; ++i) {
				const v0 = val[i];
				const v1 = applyReviver(reviver, val, String(i), v0);
				if (v1 === void 0) delete val[i];
				else if (v1 !== v0) val[i] = v1;
			}
			else if (val instanceof Map) for (const k of Array.from(val.keys())) {
				const v0 = val.get(k);
				const v1 = applyReviver(reviver, val, k, v0);
				if (v1 === void 0) val.delete(k);
				else if (v1 !== v0) val.set(k, v1);
			}
			else if (val instanceof Set) for (const v0 of Array.from(val)) {
				const v1 = applyReviver(reviver, val, v0, v0);
				if (v1 === void 0) val.delete(v0);
				else if (v1 !== v0) {
					val.delete(v0);
					val.add(v1);
				}
			}
			else for (const [k, v0] of Object.entries(val)) {
				const v1 = applyReviver(reviver, val, k, v0);
				if (v1 === void 0) delete val[k];
				else if (v1 !== v0) val[k] = v1;
			}
		}
		return reviver.call(obj, key, val);
	}
	/**
	* Recursively convert any node or its contents to native JavaScript
	*
	* @param value - The input value
	* @param arg - If `value` defines a `toJSON()` method, use this
	*   as its first argument
	* @param ctx - Conversion context, originally set in Document#toJS(). If
	*   `{ keep: true }` is not set, output should be suitable for JSON
	*   stringification.
	*/
	function toJS(value, arg, ctx) {
		if (Array.isArray(value)) return value.map((v, i) => toJS(v, String(i), ctx));
		if (value && typeof value.toJSON === "function") {
			if (!ctx || !hasAnchor(value)) return value.toJSON(arg, ctx);
			const data = {
				aliasCount: 0,
				count: 1,
				res: void 0
			};
			ctx.anchors.set(value, data);
			ctx.onCreate = (res) => {
				data.res = res;
				delete ctx.onCreate;
			};
			const res = value.toJSON(arg, ctx);
			if (ctx.onCreate) ctx.onCreate(res);
			return res;
		}
		if (typeof value === "bigint" && !ctx?.keep) return Number(value);
		return value;
	}
	var NodeBase = class {
		constructor(type) {
			Object.defineProperty(this, NODE_TYPE, { value: type });
		}
		/** Create a copy of this node.  */
		clone() {
			const copy = Object.create(Object.getPrototypeOf(this), Object.getOwnPropertyDescriptors(this));
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/** A plain JavaScript representation of this node. */
		toJS(doc, { mapAsMap, maxAliasCount, onAnchor, reviver } = {}) {
			if (!isDocument$1(doc)) throw new TypeError("A document argument is required");
			const ctx = {
				anchors: /* @__PURE__ */ new Map(),
				doc,
				keep: true,
				mapAsMap: mapAsMap === true,
				mapKeyWarned: false,
				maxAliasCount: typeof maxAliasCount === "number" ? maxAliasCount : 100
			};
			const res = toJS(this, "", ctx);
			if (typeof onAnchor === "function") for (const { count, res } of ctx.anchors.values()) onAnchor(res, count);
			return typeof reviver === "function" ? applyReviver(reviver, { "": res }, "", res) : res;
		}
	};
	var Alias = class extends NodeBase {
		constructor(source) {
			super(ALIAS);
			this.source = source;
			Object.defineProperty(this, "tag", { set() {
				throw new Error("Alias nodes cannot have tags");
			} });
		}
		/**
		* Resolve the value of this alias within `doc`, finding the last
		* instance of the `source` anchor before this node.
		*/
		resolve(doc, ctx) {
			if (ctx?.maxAliasCount === 0) throw new ReferenceError("Alias resolution is disabled");
			let nodes;
			if (ctx?.aliasResolveCache) nodes = ctx.aliasResolveCache;
			else {
				nodes = [];
				visit$1(doc, { Node: (_key, node) => {
					if (isAlias(node) || hasAnchor(node)) nodes.push(node);
				} });
				if (ctx) ctx.aliasResolveCache = nodes;
			}
			let found = void 0;
			for (const node of nodes) {
				if (node === this) break;
				if (node.anchor === this.source) found = node;
			}
			if (found && ctx) {
				const { anchors, doc, maxAliasCount } = ctx;
				let data = anchors.get(found);
				if (!data) {
					toJS(found, null, ctx);
					data = anchors.get(found);
				}
				/* istanbul ignore if */
				if (data?.res === void 0) throw new ReferenceError("This should not happen: Alias anchor was not resolved?");
				if (maxAliasCount >= 0) {
					data.count += 1;
					if (data.aliasCount === 0) data.aliasCount = getAliasCount(doc, found, anchors);
					if (data.count * data.aliasCount > maxAliasCount) throw new ReferenceError("Excessive alias count indicates a resource exhaustion attack");
				}
			}
			return found;
		}
		toJSON(_arg, ctx) {
			if (!ctx) return { source: this.source };
			const source = this.resolve(ctx.doc, ctx);
			if (!source) {
				const msg = `Unresolved alias (the anchor must be set before the alias): ${this.source}`;
				throw new ReferenceError(msg);
			}
			return ctx.anchors.get(source).res;
		}
		toString(ctx, _onComment, _onChompKeep) {
			const src = `*${this.source}`;
			if (ctx) {
				anchorIsValid(this.source);
				if (ctx.options.verifyAliasOrder && !ctx.anchors.has(this.source)) {
					const msg = `Unresolved alias (the anchor must be set before the alias): ${this.source}`;
					throw new Error(msg);
				}
				if (ctx.implicitKey) return `${src} `;
			}
			return src;
		}
	};
	function getAliasCount(doc, node, anchors) {
		if (isAlias(node)) {
			const source = node.resolve(doc);
			const anchor = anchors && source && anchors.get(source);
			return anchor ? anchor.count * anchor.aliasCount : 0;
		} else if (isCollection(node)) {
			let count = 0;
			for (const item of node.items) {
				const c = getAliasCount(doc, item, anchors);
				if (c > count) count = c;
			}
			return count;
		} else if (isPair(node)) {
			const kc = getAliasCount(doc, node.key, anchors);
			const vc = getAliasCount(doc, node.value, anchors);
			return Math.max(kc, vc);
		}
		return 1;
	}
	var isScalarValue = (value) => !value || typeof value !== "function" && typeof value !== "object";
	var Scalar = class extends NodeBase {
		constructor(value) {
			super(SCALAR$1);
			this.value = value;
		}
		toJSON(arg, ctx) {
			return ctx?.keep ? this.value : toJS(this.value, arg, ctx);
		}
		toString() {
			return String(this.value);
		}
	};
	Scalar.BLOCK_FOLDED = "BLOCK_FOLDED";
	Scalar.BLOCK_LITERAL = "BLOCK_LITERAL";
	Scalar.PLAIN = "PLAIN";
	Scalar.QUOTE_DOUBLE = "QUOTE_DOUBLE";
	Scalar.QUOTE_SINGLE = "QUOTE_SINGLE";
	var defaultTagPrefix = "tag:yaml.org,2002:";
	function findTagObject(value, tagName, tags) {
		if (tagName) {
			const match = tags.filter((t) => t.tag === tagName);
			const tagObj = match.find((t) => !t.format) ?? match[0];
			if (!tagObj) throw new Error(`Tag ${tagName} not found`);
			return tagObj;
		}
		return tags.find((t) => t.identify?.(value) && !t.format);
	}
	function createNode(value, tagName, ctx) {
		if (isDocument$1(value)) value = value.contents;
		if (isNode$1(value)) return value;
		if (isPair(value)) {
			const map = ctx.schema[MAP].createNode?.(ctx.schema, null, ctx);
			map.items.push(value);
			return map;
		}
		if (value instanceof String || value instanceof Number || value instanceof Boolean || typeof BigInt !== "undefined" && value instanceof BigInt) value = value.valueOf();
		const { aliasDuplicateObjects, onAnchor, onTagObj, schema, sourceObjects } = ctx;
		let ref = void 0;
		if (aliasDuplicateObjects && value && typeof value === "object") {
			ref = sourceObjects.get(value);
			if (ref) {
				ref.anchor ?? (ref.anchor = onAnchor(value));
				return new Alias(ref.anchor);
			} else {
				ref = {
					anchor: null,
					node: null
				};
				sourceObjects.set(value, ref);
			}
		}
		if (tagName?.startsWith("!!")) tagName = defaultTagPrefix + tagName.slice(2);
		let tagObj = findTagObject(value, tagName, schema.tags);
		if (!tagObj) {
			if (value && typeof value.toJSON === "function") value = value.toJSON();
			if (!value || typeof value !== "object") {
				const node = new Scalar(value);
				if (ref) ref.node = node;
				return node;
			}
			tagObj = value instanceof Map ? schema[MAP] : Symbol.iterator in Object(value) ? schema[SEQ] : schema[MAP];
		}
		if (onTagObj) {
			onTagObj(tagObj);
			delete ctx.onTagObj;
		}
		const node = tagObj?.createNode ? tagObj.createNode(ctx.schema, value, ctx) : typeof tagObj?.nodeClass?.from === "function" ? tagObj.nodeClass.from(ctx.schema, value, ctx) : new Scalar(value);
		if (tagName) node.tag = tagName;
		else if (!tagObj.default) node.tag = tagObj.tag;
		if (ref) ref.node = node;
		return node;
	}
	function collectionFromPath(schema, path, value) {
		let v = value;
		for (let i = path.length - 1; i >= 0; --i) {
			const k = path[i];
			if (typeof k === "number" && Number.isInteger(k) && k >= 0) {
				const a = [];
				a[k] = v;
				v = a;
			} else v = /* @__PURE__ */ new Map([[k, v]]);
		}
		return createNode(v, void 0, {
			aliasDuplicateObjects: false,
			keepUndefined: false,
			onAnchor: () => {
				throw new Error("This should not happen, please report a bug.");
			},
			schema,
			sourceObjects: /* @__PURE__ */ new Map()
		});
	}
	var isEmptyPath = (path) => path == null || typeof path === "object" && !!path[Symbol.iterator]().next().done;
	var Collection = class extends NodeBase {
		constructor(type, schema) {
			super(type);
			Object.defineProperty(this, "schema", {
				value: schema,
				configurable: true,
				enumerable: false,
				writable: true
			});
		}
		/**
		* Create a copy of this collection.
		*
		* @param schema - If defined, overwrites the original's schema
		*/
		clone(schema) {
			const copy = Object.create(Object.getPrototypeOf(this), Object.getOwnPropertyDescriptors(this));
			if (schema) copy.schema = schema;
			copy.items = copy.items.map((it) => isNode$1(it) || isPair(it) ? it.clone(schema) : it);
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/**
		* Adds a value to the collection. For `!!map` and `!!omap` the value must
		* be a Pair instance or a `{ key, value }` object, which may not have a key
		* that already exists in the map.
		*/
		addIn(path, value) {
			if (isEmptyPath(path)) this.add(value);
			else {
				const [key, ...rest] = path;
				const node = this.get(key, true);
				if (isCollection(node)) node.addIn(rest, value);
				else if (node === void 0 && this.schema) this.set(key, collectionFromPath(this.schema, rest, value));
				else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
			}
		}
		/**
		* Removes a value from the collection.
		* @returns `true` if the item was found and removed.
		*/
		deleteIn(path) {
			const [key, ...rest] = path;
			if (rest.length === 0) return this.delete(key);
			const node = this.get(key, true);
			if (isCollection(node)) return node.deleteIn(rest);
			else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
		}
		/**
		* Returns item at `key`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		getIn(path, keepScalar) {
			const [key, ...rest] = path;
			const node = this.get(key, true);
			if (rest.length === 0) return !keepScalar && isScalar(node) ? node.value : node;
			else return isCollection(node) ? node.getIn(rest, keepScalar) : void 0;
		}
		hasAllNullValues(allowScalar) {
			return this.items.every((node) => {
				if (!isPair(node)) return false;
				const n = node.value;
				return n == null || allowScalar && isScalar(n) && n.value == null && !n.commentBefore && !n.comment && !n.tag;
			});
		}
		/**
		* Checks if the collection includes a value with the key `key`.
		*/
		hasIn(path) {
			const [key, ...rest] = path;
			if (rest.length === 0) return this.has(key);
			const node = this.get(key, true);
			return isCollection(node) ? node.hasIn(rest) : false;
		}
		/**
		* Sets a value in this collection. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		setIn(path, value) {
			const [key, ...rest] = path;
			if (rest.length === 0) this.set(key, value);
			else {
				const node = this.get(key, true);
				if (isCollection(node)) node.setIn(rest, value);
				else if (node === void 0 && this.schema) this.set(key, collectionFromPath(this.schema, rest, value));
				else throw new Error(`Expected YAML collection at ${key}. Remaining path: ${rest}`);
			}
		}
	};
	/**
	* Stringifies a comment.
	*
	* Empty comment lines are left empty,
	* lines consisting of a single space are replaced by `#`,
	* and all other lines are prefixed with a `#`.
	*/
	var stringifyComment = (str) => str.replace(/^(?!$)(?: $)?/gm, "#");
	function indentComment(comment, indent) {
		if (/^\n+$/.test(comment)) return comment.substring(1);
		return indent ? comment.replace(/^(?! *$)/gm, indent) : comment;
	}
	var lineComment = (str, indent, comment) => str.endsWith("\n") ? indentComment(comment, indent) : comment.includes("\n") ? "\n" + indentComment(comment, indent) : (str.endsWith(" ") ? "" : " ") + comment;
	var FOLD_FLOW = "flow";
	var FOLD_BLOCK = "block";
	var FOLD_QUOTED = "quoted";
	/**
	* Tries to keep input at up to `lineWidth` characters, splitting only on spaces
	* not followed by newlines or spaces unless `mode` is `'quoted'`. Lines are
	* terminated with `\n` and started with `indent`.
	*/
	function foldFlowLines(text, indent, mode = "flow", { indentAtStart, lineWidth = 80, minContentWidth = 20, onFold, onOverflow } = {}) {
		if (!lineWidth || lineWidth < 0) return text;
		if (lineWidth < minContentWidth) minContentWidth = 0;
		const endStep = Math.max(1 + minContentWidth, 1 + lineWidth - indent.length);
		if (text.length <= endStep) return text;
		const folds = [];
		const escapedFolds = {};
		let end = lineWidth - indent.length;
		if (typeof indentAtStart === "number") {
			if (indentAtStart > lineWidth - Math.max(2, minContentWidth)) folds.push(0);
			else end = lineWidth - indentAtStart;
		}
		let split = void 0;
		let prev = void 0;
		let overflow = false;
		let i = -1;
		let escStart = -1;
		let escEnd = -1;
		if (mode === "block") {
			i = consumeMoreIndentedLines(text, i, indent.length);
			if (i !== -1) end = i + endStep;
		}
		for (let ch; ch = text[i += 1];) {
			if (mode === "quoted" && ch === "\\") {
				escStart = i;
				switch (text[i + 1]) {
					case "x":
						i += 3;
						break;
					case "u":
						i += 5;
						break;
					case "U":
						i += 9;
						break;
					default: i += 1;
				}
				escEnd = i;
			}
			if (ch === "\n") {
				if (mode === "block") i = consumeMoreIndentedLines(text, i, indent.length);
				end = i + indent.length + endStep;
				split = void 0;
			} else {
				if (ch === " " && prev && prev !== " " && prev !== "\n" && prev !== "	") {
					const next = text[i + 1];
					if (next && next !== " " && next !== "\n" && next !== "	") split = i;
				}
				if (i >= end) {
					if (split) {
						folds.push(split);
						end = split + endStep;
						split = void 0;
					} else if (mode === "quoted") {
						while (prev === " " || prev === "	") {
							prev = ch;
							ch = text[i += 1];
							overflow = true;
						}
						const j = i > escEnd + 1 ? i - 2 : escStart - 1;
						if (escapedFolds[j]) return text;
						folds.push(j);
						escapedFolds[j] = true;
						end = j + endStep;
						split = void 0;
					} else overflow = true;
				}
			}
			prev = ch;
		}
		if (overflow && onOverflow) onOverflow();
		if (folds.length === 0) return text;
		if (onFold) onFold();
		let res = text.slice(0, folds[0]);
		for (let i = 0; i < folds.length; ++i) {
			const fold = folds[i];
			const end = folds[i + 1] || text.length;
			if (fold === 0) res = `\n${indent}${text.slice(0, end)}`;
			else {
				if (mode === "quoted" && escapedFolds[fold]) res += `${text[fold]}\\`;
				res += `\n${indent}${text.slice(fold + 1, end)}`;
			}
		}
		return res;
	}
	/**
	* Presumes `i + 1` is at the start of a line
	* @returns index of last newline in more-indented block
	*/
	function consumeMoreIndentedLines(text, i, indent) {
		let end = i;
		let start = i + 1;
		let ch = text[start];
		while (ch === " " || ch === "	") if (i < start + indent) ch = text[++i];
		else {
			do
				ch = text[++i];
			while (ch && ch !== "\n");
			end = i;
			start = i + 1;
			ch = text[start];
		}
		return end;
	}
	var getFoldOptions = (ctx, isBlock) => ({
		indentAtStart: isBlock ? ctx.indent.length : ctx.indentAtStart,
		lineWidth: ctx.options.lineWidth,
		minContentWidth: ctx.options.minContentWidth
	});
	var containsDocumentMarker = (str) => /^(%|---|\.\.\.)/m.test(str);
	function lineLengthOverLimit(str, lineWidth, indentLength) {
		if (!lineWidth || lineWidth < 0) return false;
		const limit = lineWidth - indentLength;
		const strLen = str.length;
		if (strLen <= limit) return false;
		for (let i = 0, start = 0; i < strLen; ++i) if (str[i] === "\n") {
			if (i - start > limit) return true;
			start = i + 1;
			if (strLen - start <= limit) return false;
		}
		return true;
	}
	function doubleQuotedString(value, ctx) {
		const json = JSON.stringify(value);
		if (ctx.options.doubleQuotedAsJSON) return json;
		const { implicitKey } = ctx;
		const minMultiLineLength = ctx.options.doubleQuotedMinMultiLineLength;
		const indent = ctx.indent || (containsDocumentMarker(value) ? "  " : "");
		let str = "";
		let start = 0;
		for (let i = 0, ch = json[i]; ch; ch = json[++i]) {
			if (ch === " " && json[i + 1] === "\\" && json[i + 2] === "n") {
				str += json.slice(start, i) + "\\ ";
				i += 1;
				start = i;
				ch = "\\";
			}
			if (ch === "\\") switch (json[i + 1]) {
				case "u":
					{
						str += json.slice(start, i);
						const code = json.substr(i + 2, 4);
						switch (code) {
							case "0000":
								str += "\\0";
								break;
							case "0007":
								str += "\\a";
								break;
							case "000b":
								str += "\\v";
								break;
							case "001b":
								str += "\\e";
								break;
							case "0085":
								str += "\\N";
								break;
							case "00a0":
								str += "\\_";
								break;
							case "2028":
								str += "\\L";
								break;
							case "2029":
								str += "\\P";
								break;
							default: if (code.substr(0, 2) === "00") str += "\\x" + code.substr(2);
							else str += json.substr(i, 6);
						}
						i += 5;
						start = i + 1;
					}
					break;
				case "n":
					if (implicitKey || json[i + 2] === "\"" || json.length < minMultiLineLength) i += 1;
					else {
						str += json.slice(start, i) + "\n\n";
						while (json[i + 2] === "\\" && json[i + 3] === "n" && json[i + 4] !== "\"") {
							str += "\n";
							i += 2;
						}
						str += indent;
						if (json[i + 2] === " ") str += "\\";
						i += 1;
						start = i + 1;
					}
					break;
				default: i += 1;
			}
		}
		str = start ? str + json.slice(start) : json;
		return implicitKey ? str : foldFlowLines(str, indent, FOLD_QUOTED, getFoldOptions(ctx, false));
	}
	function singleQuotedString(value, ctx) {
		if (ctx.options.singleQuote === false || ctx.implicitKey && value.includes("\n") || /[ \t]\n|\n[ \t]/.test(value)) return doubleQuotedString(value, ctx);
		const indent = ctx.indent || (containsDocumentMarker(value) ? "  " : "");
		const res = "'" + value.replace(/'/g, "''").replace(/\n+/g, `$&\n${indent}`) + "'";
		return ctx.implicitKey ? res : foldFlowLines(res, indent, FOLD_FLOW, getFoldOptions(ctx, false));
	}
	function quotedString(value, ctx) {
		const { singleQuote } = ctx.options;
		let qs;
		if (singleQuote === false) qs = doubleQuotedString;
		else {
			const hasDouble = value.includes("\"");
			const hasSingle = value.includes("'");
			if (hasDouble && !hasSingle) qs = singleQuotedString;
			else if (hasSingle && !hasDouble) qs = doubleQuotedString;
			else qs = singleQuote ? singleQuotedString : doubleQuotedString;
		}
		return qs(value, ctx);
	}
	var blockEndNewlines;
	try {
		blockEndNewlines = /* @__PURE__ */ new RegExp("(^|(?<!\n))\n+(?!\n|$)", "g");
	} catch {
		blockEndNewlines = /\n+(?!\n|$)/g;
	}
	function blockString({ comment, type, value }, ctx, onComment, onChompKeep) {
		const { blockQuote, commentString, lineWidth } = ctx.options;
		if (!blockQuote || /\n[\t ]+$/.test(value)) return quotedString(value, ctx);
		const indent = ctx.indent || (ctx.forceBlockIndent || containsDocumentMarker(value) ? "  " : "");
		const literal = blockQuote === "literal" ? true : blockQuote === "folded" || type === Scalar.BLOCK_FOLDED ? false : type === Scalar.BLOCK_LITERAL ? true : !lineLengthOverLimit(value, lineWidth, indent.length);
		if (!value) return literal ? "|\n" : ">\n";
		let chomp;
		let endStart;
		for (endStart = value.length; endStart > 0; --endStart) {
			const ch = value[endStart - 1];
			if (ch !== "\n" && ch !== "	" && ch !== " ") break;
		}
		let end = value.substring(endStart);
		const endNlPos = end.indexOf("\n");
		if (endNlPos === -1) chomp = "-";
		else if (value === end || endNlPos !== end.length - 1) {
			chomp = "+";
			if (onChompKeep) onChompKeep();
		} else chomp = "";
		if (end) {
			value = value.slice(0, -end.length);
			if (end[end.length - 1] === "\n") end = end.slice(0, -1);
			end = end.replace(blockEndNewlines, `$&${indent}`);
		}
		let startWithSpace = false;
		let startEnd;
		let startNlPos = -1;
		for (startEnd = 0; startEnd < value.length; ++startEnd) {
			const ch = value[startEnd];
			if (ch === " ") startWithSpace = true;
			else if (ch === "\n") startNlPos = startEnd;
			else break;
		}
		let start = value.substring(0, startNlPos < startEnd ? startNlPos + 1 : startEnd);
		if (start) {
			value = value.substring(start.length);
			start = start.replace(/\n+/g, `$&${indent}`);
		}
		let header = (startWithSpace ? indent ? "2" : "1" : "") + chomp;
		if (comment) {
			header += " " + commentString(comment.replace(/ ?[\r\n]+/g, " "));
			if (onComment) onComment();
		}
		if (!literal) {
			const foldedValue = value.replace(/\n+/g, "\n$&").replace(/(?:^|\n)([\t ].*)(?:([\n\t ]*)\n(?![\n\t ]))?/g, "$1$2").replace(/\n+/g, `$&${indent}`);
			let literalFallback = false;
			const foldOptions = getFoldOptions(ctx, true);
			if (blockQuote !== "folded" && type !== Scalar.BLOCK_FOLDED) foldOptions.onOverflow = () => {
				literalFallback = true;
			};
			const body = foldFlowLines(`${start}${foldedValue}${end}`, indent, FOLD_BLOCK, foldOptions);
			if (!literalFallback) return `>${header}\n${indent}${body}`;
		}
		value = value.replace(/\n+/g, `$&${indent}`);
		return `|${header}\n${indent}${start}${value}${end}`;
	}
	function plainString(item, ctx, onComment, onChompKeep) {
		const { type, value } = item;
		const { actualString, implicitKey, indent, indentStep, inFlow } = ctx;
		if (implicitKey && value.includes("\n") || inFlow && /[[\]{},]/.test(value)) return quotedString(value, ctx);
		if (/^[\n\t ,[\]{}#&*!|>'"%@`]|^[?-]$|^[?-][ \t]|[\n:][ \t]|[ \t]\n|[\n\t ]#|[\n\t :]$/.test(value)) return implicitKey || inFlow || !value.includes("\n") ? quotedString(value, ctx) : blockString(item, ctx, onComment, onChompKeep);
		if (!implicitKey && !inFlow && type !== Scalar.PLAIN && value.includes("\n")) return blockString(item, ctx, onComment, onChompKeep);
		if (containsDocumentMarker(value)) {
			if (indent === "") {
				ctx.forceBlockIndent = true;
				return blockString(item, ctx, onComment, onChompKeep);
			} else if (implicitKey && indent === indentStep) return quotedString(value, ctx);
		}
		const str = value.replace(/\n+/g, `$&\n${indent}`);
		if (actualString) {
			const test = (tag) => tag.default && tag.tag !== "tag:yaml.org,2002:str" && tag.test?.test(str);
			const { compat, tags } = ctx.doc.schema;
			if (tags.some(test) || compat?.some(test)) return quotedString(value, ctx);
		}
		return implicitKey ? str : foldFlowLines(str, indent, FOLD_FLOW, getFoldOptions(ctx, false));
	}
	function stringifyString(item, ctx, onComment, onChompKeep) {
		const { implicitKey, inFlow } = ctx;
		const ss = typeof item.value === "string" ? item : Object.assign({}, item, { value: String(item.value) });
		let { type } = item;
		if (type !== Scalar.QUOTE_DOUBLE) {
			if (/[\x00-\x08\x0b-\x1f\x7f-\x9f\u{D800}-\u{DFFF}]/u.test(ss.value)) type = Scalar.QUOTE_DOUBLE;
		}
		const _stringify = (_type) => {
			switch (_type) {
				case Scalar.BLOCK_FOLDED:
				case Scalar.BLOCK_LITERAL: return implicitKey || inFlow ? quotedString(ss.value, ctx) : blockString(ss, ctx, onComment, onChompKeep);
				case Scalar.QUOTE_DOUBLE: return doubleQuotedString(ss.value, ctx);
				case Scalar.QUOTE_SINGLE: return singleQuotedString(ss.value, ctx);
				case Scalar.PLAIN: return plainString(ss, ctx, onComment, onChompKeep);
				default: return null;
			}
		};
		let res = _stringify(type);
		if (res === null) {
			const { defaultKeyType, defaultStringType } = ctx.options;
			const t = implicitKey && defaultKeyType || defaultStringType;
			res = _stringify(t);
			if (res === null) throw new Error(`Unsupported default string type ${t}`);
		}
		return res;
	}
	function createStringifyContext(doc, options) {
		const opt = Object.assign({
			blockQuote: true,
			commentString: stringifyComment,
			defaultKeyType: null,
			defaultStringType: "PLAIN",
			directives: null,
			doubleQuotedAsJSON: false,
			doubleQuotedMinMultiLineLength: 40,
			falseStr: "false",
			flowCollectionPadding: true,
			indentSeq: true,
			lineWidth: 80,
			minContentWidth: 20,
			nullStr: "null",
			simpleKeys: false,
			singleQuote: null,
			trailingComma: false,
			trueStr: "true",
			verifyAliasOrder: true
		}, doc.schema.toStringOptions, options);
		let inFlow;
		switch (opt.collectionStyle) {
			case "block":
				inFlow = false;
				break;
			case "flow":
				inFlow = true;
				break;
			default: inFlow = null;
		}
		return {
			anchors: /* @__PURE__ */ new Set(),
			doc,
			flowCollectionPadding: opt.flowCollectionPadding ? " " : "",
			indent: "",
			indentStep: typeof opt.indent === "number" ? " ".repeat(opt.indent) : "  ",
			inFlow,
			options: opt
		};
	}
	function getTagObject(tags, item) {
		if (item.tag) {
			const match = tags.filter((t) => t.tag === item.tag);
			if (match.length > 0) return match.find((t) => t.format === item.format) ?? match[0];
		}
		let tagObj = void 0;
		let obj;
		if (isScalar(item)) {
			obj = item.value;
			let match = tags.filter((t) => t.identify?.(obj));
			if (match.length > 1) {
				const testMatch = match.filter((t) => t.test);
				if (testMatch.length > 0) match = testMatch;
			}
			tagObj = match.find((t) => t.format === item.format) ?? match.find((t) => !t.format);
		} else {
			obj = item;
			tagObj = tags.find((t) => t.nodeClass && obj instanceof t.nodeClass);
		}
		if (!tagObj) {
			const name = obj?.constructor?.name ?? (obj === null ? "null" : typeof obj);
			throw new Error(`Tag not resolved for ${name} value`);
		}
		return tagObj;
	}
	function stringifyProps(node, tagObj, { anchors, doc }) {
		if (!doc.directives) return "";
		const props = [];
		const anchor = (isScalar(node) || isCollection(node)) && node.anchor;
		if (anchor && anchorIsValid(anchor)) {
			anchors.add(anchor);
			props.push(`&${anchor}`);
		}
		const tag = node.tag ?? (tagObj.default ? null : tagObj.tag);
		if (tag) props.push(doc.directives.tagString(tag));
		return props.join(" ");
	}
	function stringify(item, ctx, onComment, onChompKeep) {
		if (isPair(item)) return item.toString(ctx, onComment, onChompKeep);
		if (isAlias(item)) {
			if (ctx.doc.directives) return item.toString(ctx);
			if (ctx.resolvedAliases?.has(item)) throw new TypeError(`Cannot stringify circular structure without alias nodes`);
			else {
				if (ctx.resolvedAliases) ctx.resolvedAliases.add(item);
				else ctx.resolvedAliases = /* @__PURE__ */ new Set([item]);
				item = item.resolve(ctx.doc);
			}
		}
		let tagObj = void 0;
		const node = isNode$1(item) ? item : ctx.doc.createNode(item, { onTagObj: (o) => tagObj = o });
		tagObj ?? (tagObj = getTagObject(ctx.doc.schema.tags, node));
		const props = stringifyProps(node, tagObj, ctx);
		if (props.length > 0) ctx.indentAtStart = (ctx.indentAtStart ?? 0) + props.length + 1;
		const str = typeof tagObj.stringify === "function" ? tagObj.stringify(node, ctx, onComment, onChompKeep) : isScalar(node) ? stringifyString(node, ctx, onComment, onChompKeep) : node.toString(ctx, onComment, onChompKeep);
		if (!props) return str;
		return isScalar(node) || str[0] === "{" || str[0] === "[" ? `${props} ${str}` : `${props}\n${ctx.indent}${str}`;
	}
	function stringifyPair({ key, value }, ctx, onComment, onChompKeep) {
		const { allNullValues, doc, indent, indentStep, options: { commentString, indentSeq, simpleKeys } } = ctx;
		let keyComment = isNode$1(key) && key.comment || null;
		if (simpleKeys) {
			if (keyComment) throw new Error("With simple keys, key nodes cannot have comments");
			if (isCollection(key) || !isNode$1(key) && typeof key === "object") throw new Error("With simple keys, collection cannot be used as a key value");
		}
		let explicitKey = !simpleKeys && (!key || keyComment && value == null && !ctx.inFlow || isCollection(key) || (isScalar(key) ? key.type === Scalar.BLOCK_FOLDED || key.type === Scalar.BLOCK_LITERAL : typeof key === "object"));
		ctx = Object.assign({}, ctx, {
			allNullValues: false,
			implicitKey: !explicitKey && (simpleKeys || !allNullValues),
			indent: indent + indentStep
		});
		let keyCommentDone = false;
		let chompKeep = false;
		let str = stringify(key, ctx, () => keyCommentDone = true, () => chompKeep = true);
		if (!explicitKey && !ctx.inFlow && str.length > 1024) {
			if (simpleKeys) throw new Error("With simple keys, single line scalar must not span more than 1024 characters");
			explicitKey = true;
		}
		if (ctx.inFlow) {
			if (allNullValues || value == null) {
				if (keyCommentDone && onComment) onComment();
				return str === "" ? "?" : explicitKey ? `? ${str}` : str;
			}
		} else if (allNullValues && !simpleKeys || value == null && explicitKey) {
			str = `? ${str}`;
			if (keyComment && !keyCommentDone) str += lineComment(str, ctx.indent, commentString(keyComment));
			else if (chompKeep && onChompKeep) onChompKeep();
			return str;
		}
		if (keyCommentDone) keyComment = null;
		if (explicitKey) {
			if (keyComment) str += lineComment(str, ctx.indent, commentString(keyComment));
			str = `? ${str}\n${indent}:`;
		} else {
			str = `${str}:`;
			if (keyComment) str += lineComment(str, ctx.indent, commentString(keyComment));
		}
		let vsb, vcb, valueComment;
		if (isNode$1(value)) {
			vsb = !!value.spaceBefore;
			vcb = value.commentBefore;
			valueComment = value.comment;
		} else {
			vsb = false;
			vcb = null;
			valueComment = null;
			if (value && typeof value === "object") value = doc.createNode(value);
		}
		ctx.implicitKey = false;
		if (!explicitKey && !keyComment && isScalar(value)) ctx.indentAtStart = str.length + 1;
		chompKeep = false;
		if (!indentSeq && indentStep.length >= 2 && !ctx.inFlow && !explicitKey && isSeq(value) && !value.flow && !value.tag && !value.anchor) ctx.indent = ctx.indent.substring(2);
		let valueCommentDone = false;
		const valueStr = stringify(value, ctx, () => valueCommentDone = true, () => chompKeep = true);
		let ws = " ";
		if (keyComment || vsb || vcb) {
			ws = vsb ? "\n" : "";
			if (vcb) {
				const cs = commentString(vcb);
				ws += `\n${indentComment(cs, ctx.indent)}`;
			}
			if (valueStr === "" && !ctx.inFlow) {
				if (ws === "\n" && valueComment) ws = "\n\n";
			} else ws += `\n${ctx.indent}`;
		} else if (!explicitKey && isCollection(value)) {
			const vs0 = valueStr[0];
			const nl0 = valueStr.indexOf("\n");
			const hasNewline = nl0 !== -1;
			const flow = ctx.inFlow ?? value.flow ?? value.items.length === 0;
			if (hasNewline || !flow) {
				let hasPropsLine = false;
				if (hasNewline && (vs0 === "&" || vs0 === "!")) {
					let sp0 = valueStr.indexOf(" ");
					if (vs0 === "&" && sp0 !== -1 && sp0 < nl0 && valueStr[sp0 + 1] === "!") sp0 = valueStr.indexOf(" ", sp0 + 1);
					if (sp0 === -1 || nl0 < sp0) hasPropsLine = true;
				}
				if (!hasPropsLine) ws = `\n${ctx.indent}`;
			}
		} else if (valueStr === "" || valueStr[0] === "\n") ws = "";
		str += ws + valueStr;
		if (ctx.inFlow) {
			if (valueCommentDone && onComment) onComment();
		} else if (valueComment && !valueCommentDone) str += lineComment(str, ctx.indent, commentString(valueComment));
		else if (chompKeep && onChompKeep) onChompKeep();
		return str;
	}
	function warn(logLevel, warning) {
		if (logLevel === "debug" || logLevel === "warn") console.warn(warning);
	}
	var MERGE_KEY = "<<";
	var merge = {
		identify: (value) => value === MERGE_KEY || typeof value === "symbol" && value.description === MERGE_KEY,
		default: "key",
		tag: "tag:yaml.org,2002:merge",
		test: /^<<$/,
		resolve: () => Object.assign(new Scalar(Symbol(MERGE_KEY)), { addToJSMap: addMergeToJSMap }),
		stringify: () => MERGE_KEY
	};
	var isMergeKey = (ctx, key) => (merge.identify(key) || isScalar(key) && (!key.type || key.type === Scalar.PLAIN) && merge.identify(key.value)) && ctx?.doc.schema.tags.some((tag) => tag.tag === merge.tag && tag.default);
	function addMergeToJSMap(ctx, map, value) {
		const source = resolveAliasValue(ctx, value);
		if (isSeq(source)) for (const it of source.items) mergeValue(ctx, map, it);
		else if (Array.isArray(source)) for (const it of source) mergeValue(ctx, map, it);
		else mergeValue(ctx, map, source);
	}
	function mergeValue(ctx, map, value) {
		const source = resolveAliasValue(ctx, value);
		if (!isMap(source)) throw new Error("Merge sources must be maps or map aliases");
		const srcMap = source.toJSON(null, ctx, Map);
		for (const [key, value] of srcMap) if (map instanceof Map) {
			if (!map.has(key)) map.set(key, value);
		} else if (map instanceof Set) map.add(key);
		else if (!Object.prototype.hasOwnProperty.call(map, key)) Object.defineProperty(map, key, {
			value,
			writable: true,
			enumerable: true,
			configurable: true
		});
		return map;
	}
	function resolveAliasValue(ctx, value) {
		return ctx && isAlias(value) ? value.resolve(ctx.doc, ctx) : value;
	}
	function addPairToJSMap(ctx, map, { key, value }) {
		if (isNode$1(key) && key.addToJSMap) key.addToJSMap(ctx, map, value);
		else if (isMergeKey(ctx, key)) addMergeToJSMap(ctx, map, value);
		else {
			const jsKey = toJS(key, "", ctx);
			if (map instanceof Map) map.set(jsKey, toJS(value, jsKey, ctx));
			else if (map instanceof Set) map.add(jsKey);
			else {
				const stringKey = stringifyKey(key, jsKey, ctx);
				const jsValue = toJS(value, stringKey, ctx);
				if (stringKey in map) Object.defineProperty(map, stringKey, {
					value: jsValue,
					writable: true,
					enumerable: true,
					configurable: true
				});
				else map[stringKey] = jsValue;
			}
		}
		return map;
	}
	function stringifyKey(key, jsKey, ctx) {
		if (jsKey === null) return "";
		if (typeof jsKey !== "object") return String(jsKey);
		if (isNode$1(key) && ctx?.doc) {
			const strCtx = createStringifyContext(ctx.doc, {});
			strCtx.anchors = /* @__PURE__ */ new Set();
			for (const node of ctx.anchors.keys()) strCtx.anchors.add(node.anchor);
			strCtx.inFlow = true;
			strCtx.inStringifyKey = true;
			const strKey = key.toString(strCtx);
			if (!ctx.mapKeyWarned) {
				let jsonStr = JSON.stringify(strKey);
				if (jsonStr.length > 40) jsonStr = jsonStr.substring(0, 36) + "...\"";
				warn(ctx.doc.options.logLevel, `Keys with collection values will be stringified due to JS Object restrictions: ${jsonStr}. Set mapAsMap: true to use object keys.`);
				ctx.mapKeyWarned = true;
			}
			return strKey;
		}
		return JSON.stringify(jsKey);
	}
	function createPair(key, value, ctx) {
		return new Pair(createNode(key, void 0, ctx), createNode(value, void 0, ctx));
	}
	var Pair = class Pair {
		constructor(key, value = null) {
			Object.defineProperty(this, NODE_TYPE, { value: PAIR });
			this.key = key;
			this.value = value;
		}
		clone(schema) {
			let { key, value } = this;
			if (isNode$1(key)) key = key.clone(schema);
			if (isNode$1(value)) value = value.clone(schema);
			return new Pair(key, value);
		}
		toJSON(_, ctx) {
			return addPairToJSMap(ctx, ctx?.mapAsMap ? /* @__PURE__ */ new Map() : {}, this);
		}
		toString(ctx, onComment, onChompKeep) {
			return ctx?.doc ? stringifyPair(this, ctx, onComment, onChompKeep) : JSON.stringify(this);
		}
	};
	function stringifyCollection(collection, ctx, options) {
		return (ctx.inFlow ?? collection.flow ? stringifyFlowCollection : stringifyBlockCollection)(collection, ctx, options);
	}
	function stringifyBlockCollection({ comment, items }, ctx, { blockItemPrefix, flowChars, itemIndent, onChompKeep, onComment }) {
		const { indent, options: { commentString } } = ctx;
		const itemCtx = Object.assign({}, ctx, {
			indent: itemIndent,
			type: null
		});
		let chompKeep = false;
		const lines = [];
		for (let i = 0; i < items.length; ++i) {
			const item = items[i];
			let comment = null;
			if (isNode$1(item)) {
				if (!chompKeep && item.spaceBefore) lines.push("");
				addCommentBefore(ctx, lines, item.commentBefore, chompKeep);
				if (item.comment) comment = item.comment;
			} else if (isPair(item)) {
				const ik = isNode$1(item.key) ? item.key : null;
				if (ik) {
					if (!chompKeep && ik.spaceBefore) lines.push("");
					addCommentBefore(ctx, lines, ik.commentBefore, chompKeep);
				}
			}
			chompKeep = false;
			let str = stringify(item, itemCtx, () => comment = null, () => chompKeep = true);
			if (comment) str += lineComment(str, itemIndent, commentString(comment));
			if (chompKeep && comment) chompKeep = false;
			lines.push(blockItemPrefix + str);
		}
		let str;
		if (lines.length === 0) str = flowChars.start + flowChars.end;
		else {
			str = lines[0];
			for (let i = 1; i < lines.length; ++i) {
				const line = lines[i];
				str += line ? `\n${indent}${line}` : "\n";
			}
		}
		if (comment) {
			str += "\n" + indentComment(commentString(comment), indent);
			if (onComment) onComment();
		} else if (chompKeep && onChompKeep) onChompKeep();
		return str;
	}
	function stringifyFlowCollection({ items }, ctx, { flowChars, itemIndent }) {
		const { indent, indentStep, flowCollectionPadding: fcPadding, options: { commentString } } = ctx;
		itemIndent += indentStep;
		const itemCtx = Object.assign({}, ctx, {
			indent: itemIndent,
			inFlow: true,
			type: null
		});
		let reqNewline = false;
		let linesAtValue = 0;
		const lines = [];
		for (let i = 0; i < items.length; ++i) {
			const item = items[i];
			let comment = null;
			if (isNode$1(item)) {
				if (item.spaceBefore) lines.push("");
				addCommentBefore(ctx, lines, item.commentBefore, false);
				if (item.comment) comment = item.comment;
			} else if (isPair(item)) {
				const ik = isNode$1(item.key) ? item.key : null;
				if (ik) {
					if (ik.spaceBefore) lines.push("");
					addCommentBefore(ctx, lines, ik.commentBefore, false);
					if (ik.comment) reqNewline = true;
				}
				const iv = isNode$1(item.value) ? item.value : null;
				if (iv) {
					if (iv.comment) comment = iv.comment;
					if (iv.commentBefore) reqNewline = true;
				} else if (item.value == null && ik?.comment) comment = ik.comment;
			}
			if (comment) reqNewline = true;
			let str = stringify(item, itemCtx, () => comment = null);
			reqNewline || (reqNewline = lines.length > linesAtValue || str.includes("\n"));
			if (i < items.length - 1) str += ",";
			else if (ctx.options.trailingComma) {
				if (ctx.options.lineWidth > 0) reqNewline || (reqNewline = lines.reduce((sum, line) => sum + line.length + 2, 2) + (str.length + 2) > ctx.options.lineWidth);
				if (reqNewline) str += ",";
			}
			if (comment) str += lineComment(str, itemIndent, commentString(comment));
			lines.push(str);
			linesAtValue = lines.length;
		}
		const { start, end } = flowChars;
		if (lines.length === 0) return start + end;
		else {
			if (!reqNewline) {
				const len = lines.reduce((sum, line) => sum + line.length + 2, 2);
				reqNewline = ctx.options.lineWidth > 0 && len > ctx.options.lineWidth;
			}
			if (reqNewline) {
				let str = start;
				for (const line of lines) str += line ? `\n${indentStep}${indent}${line}` : "\n";
				return `${str}\n${indent}${end}`;
			} else return `${start}${fcPadding}${lines.join(" ")}${fcPadding}${end}`;
		}
	}
	function addCommentBefore({ indent, options: { commentString } }, lines, comment, chompKeep) {
		if (comment && chompKeep) comment = comment.replace(/^\n+/, "");
		if (comment) {
			const ic = indentComment(commentString(comment), indent);
			lines.push(ic.trimStart());
		}
	}
	function findPair(items, key) {
		const k = isScalar(key) ? key.value : key;
		for (const it of items) if (isPair(it)) {
			if (it.key === key || it.key === k) return it;
			if (isScalar(it.key) && it.key.value === k) return it;
		}
	}
	var YAMLMap = class extends Collection {
		static get tagName() {
			return "tag:yaml.org,2002:map";
		}
		constructor(schema) {
			super(MAP, schema);
			this.items = [];
		}
		/**
		* A generic collection parsing method that can be extended
		* to other node classes that inherit from YAMLMap
		*/
		static from(schema, obj, ctx) {
			const { keepUndefined, replacer } = ctx;
			const map = new this(schema);
			const add = (key, value) => {
				if (typeof replacer === "function") value = replacer.call(obj, key, value);
				else if (Array.isArray(replacer) && !replacer.includes(key)) return;
				if (value !== void 0 || keepUndefined) map.items.push(createPair(key, value, ctx));
			};
			if (obj instanceof Map) for (const [key, value] of obj) add(key, value);
			else if (obj && typeof obj === "object") for (const key of Object.keys(obj)) add(key, obj[key]);
			if (typeof schema.sortMapEntries === "function") map.items.sort(schema.sortMapEntries);
			return map;
		}
		/**
		* Adds a value to the collection.
		*
		* @param overwrite - If not set `true`, using a key that is already in the
		*   collection will throw. Otherwise, overwrites the previous value.
		*/
		add(pair, overwrite) {
			let _pair;
			if (isPair(pair)) _pair = pair;
			else if (!pair || typeof pair !== "object" || !("key" in pair)) _pair = new Pair(pair, pair?.value);
			else _pair = new Pair(pair.key, pair.value);
			const prev = findPair(this.items, _pair.key);
			const sortEntries = this.schema?.sortMapEntries;
			if (prev) {
				if (!overwrite) throw new Error(`Key ${_pair.key} already set`);
				if (isScalar(prev.value) && isScalarValue(_pair.value)) prev.value.value = _pair.value;
				else prev.value = _pair.value;
			} else if (sortEntries) {
				const i = this.items.findIndex((item) => sortEntries(_pair, item) < 0);
				if (i === -1) this.items.push(_pair);
				else this.items.splice(i, 0, _pair);
			} else this.items.push(_pair);
		}
		delete(key) {
			const it = findPair(this.items, key);
			if (!it) return false;
			return this.items.splice(this.items.indexOf(it), 1).length > 0;
		}
		get(key, keepScalar) {
			const node = findPair(this.items, key)?.value;
			return (!keepScalar && isScalar(node) ? node.value : node) ?? void 0;
		}
		has(key) {
			return !!findPair(this.items, key);
		}
		set(key, value) {
			this.add(new Pair(key, value), true);
		}
		/**
		* @param ctx - Conversion context, originally set in Document#toJS()
		* @param {Class} Type - If set, forces the returned collection type
		* @returns Instance of Type, Map, or Object
		*/
		toJSON(_, ctx, Type) {
			const map = Type ? new Type() : ctx?.mapAsMap ? /* @__PURE__ */ new Map() : {};
			if (ctx?.onCreate) ctx.onCreate(map);
			for (const item of this.items) addPairToJSMap(ctx, map, item);
			return map;
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			for (const item of this.items) if (!isPair(item)) throw new Error(`Map items must all be pairs; found ${JSON.stringify(item)} instead`);
			if (!ctx.allNullValues && this.hasAllNullValues(false)) ctx = Object.assign({}, ctx, { allNullValues: true });
			return stringifyCollection(this, ctx, {
				blockItemPrefix: "",
				flowChars: {
					start: "{",
					end: "}"
				},
				itemIndent: ctx.indent || "",
				onChompKeep,
				onComment
			});
		}
	};
	var map = {
		collection: "map",
		default: true,
		nodeClass: YAMLMap,
		tag: "tag:yaml.org,2002:map",
		resolve(map, onError) {
			if (!isMap(map)) onError("Expected a mapping for this tag");
			return map;
		},
		createNode: (schema, obj, ctx) => YAMLMap.from(schema, obj, ctx)
	};
	var YAMLSeq = class extends Collection {
		static get tagName() {
			return "tag:yaml.org,2002:seq";
		}
		constructor(schema) {
			super(SEQ, schema);
			this.items = [];
		}
		add(value) {
			this.items.push(value);
		}
		/**
		* Removes a value from the collection.
		*
		* `key` must contain a representation of an integer for this to succeed.
		* It may be wrapped in a `Scalar`.
		*
		* @returns `true` if the item was found and removed.
		*/
		delete(key) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") return false;
			return this.items.splice(idx, 1).length > 0;
		}
		get(key, keepScalar) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") return void 0;
			const it = this.items[idx];
			return !keepScalar && isScalar(it) ? it.value : it;
		}
		/**
		* Checks if the collection includes a value with the key `key`.
		*
		* `key` must contain a representation of an integer for this to succeed.
		* It may be wrapped in a `Scalar`.
		*/
		has(key) {
			const idx = asItemIndex(key);
			return typeof idx === "number" && idx < this.items.length;
		}
		/**
		* Sets a value in this collection. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*
		* If `key` does not contain a representation of an integer, this will throw.
		* It may be wrapped in a `Scalar`.
		*/
		set(key, value) {
			const idx = asItemIndex(key);
			if (typeof idx !== "number") throw new Error(`Expected a valid index, not ${key}.`);
			const prev = this.items[idx];
			if (isScalar(prev) && isScalarValue(value)) prev.value = value;
			else this.items[idx] = value;
		}
		toJSON(_, ctx) {
			const seq = [];
			if (ctx?.onCreate) ctx.onCreate(seq);
			let i = 0;
			for (const item of this.items) seq.push(toJS(item, String(i++), ctx));
			return seq;
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			return stringifyCollection(this, ctx, {
				blockItemPrefix: "- ",
				flowChars: {
					start: "[",
					end: "]"
				},
				itemIndent: (ctx.indent || "") + "  ",
				onChompKeep,
				onComment
			});
		}
		static from(schema, obj, ctx) {
			const { replacer } = ctx;
			const seq = new this(schema);
			if (obj && Symbol.iterator in Object(obj)) {
				let i = 0;
				for (let it of obj) {
					if (typeof replacer === "function") {
						const key = obj instanceof Set ? it : String(i++);
						it = replacer.call(obj, key, it);
					}
					seq.items.push(createNode(it, void 0, ctx));
				}
			}
			return seq;
		}
	};
	function asItemIndex(key) {
		let idx = isScalar(key) ? key.value : key;
		if (idx && typeof idx === "string") idx = Number(idx);
		return typeof idx === "number" && Number.isInteger(idx) && idx >= 0 ? idx : null;
	}
	var seq = {
		collection: "seq",
		default: true,
		nodeClass: YAMLSeq,
		tag: "tag:yaml.org,2002:seq",
		resolve(seq, onError) {
			if (!isSeq(seq)) onError("Expected a sequence for this tag");
			return seq;
		},
		createNode: (schema, obj, ctx) => YAMLSeq.from(schema, obj, ctx)
	};
	var string = {
		identify: (value) => typeof value === "string",
		default: true,
		tag: "tag:yaml.org,2002:str",
		resolve: (str) => str,
		stringify(item, ctx, onComment, onChompKeep) {
			ctx = Object.assign({ actualString: true }, ctx);
			return stringifyString(item, ctx, onComment, onChompKeep);
		}
	};
	var nullTag = {
		identify: (value) => value == null,
		createNode: () => new Scalar(null),
		default: true,
		tag: "tag:yaml.org,2002:null",
		test: /^(?:~|[Nn]ull|NULL)?$/,
		resolve: () => new Scalar(null),
		stringify: ({ source }, ctx) => typeof source === "string" && nullTag.test.test(source) ? source : ctx.options.nullStr
	};
	var boolTag = {
		identify: (value) => typeof value === "boolean",
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:[Tt]rue|TRUE|[Ff]alse|FALSE)$/,
		resolve: (str) => new Scalar(str[0] === "t" || str[0] === "T"),
		stringify({ source, value }, ctx) {
			if (source && boolTag.test.test(source)) {
				if (value === (source[0] === "t" || source[0] === "T")) return source;
			}
			return value ? ctx.options.trueStr : ctx.options.falseStr;
		}
	};
	function stringifyNumber({ format, minFractionDigits, tag, value }) {
		if (typeof value === "bigint") return String(value);
		const num = typeof value === "number" ? value : Number(value);
		if (!isFinite(num)) return isNaN(num) ? ".nan" : num < 0 ? "-.inf" : ".inf";
		let n = Object.is(value, -0) ? "-0" : JSON.stringify(value);
		if (!format && minFractionDigits && (!tag || tag === "tag:yaml.org,2002:float") && /^-?\d/.test(n) && !n.includes("e")) {
			let i = n.indexOf(".");
			if (i < 0) {
				i = n.length;
				n += ".";
			}
			let d = minFractionDigits - (n.length - i - 1);
			while (d-- > 0) n += "0";
		}
		return n;
	}
	var floatNaN$1 = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^(?:[-+]?\.(?:inf|Inf|INF)|\.nan|\.NaN|\.NAN)$/,
		resolve: (str) => str.slice(-3).toLowerCase() === "nan" ? NaN : str[0] === "-" ? Number.NEGATIVE_INFINITY : Number.POSITIVE_INFINITY,
		stringify: stringifyNumber
	};
	var floatExp$1 = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "EXP",
		test: /^[-+]?(?:\.[0-9]+|[0-9]+(?:\.[0-9]*)?)[eE][-+]?[0-9]+$/,
		resolve: (str) => parseFloat(str),
		stringify(node) {
			const num = Number(node.value);
			return isFinite(num) ? num.toExponential() : stringifyNumber(node);
		}
	};
	var float$1 = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^[-+]?(?:\.[0-9]+|[0-9]+\.[0-9]*)$/,
		resolve(str) {
			const node = new Scalar(parseFloat(str));
			const dot = str.indexOf(".");
			if (dot !== -1 && str[str.length - 1] === "0") node.minFractionDigits = str.length - dot - 1;
			return node;
		},
		stringify: stringifyNumber
	};
	var intIdentify$2 = (value) => typeof value === "bigint" || Number.isInteger(value);
	var intResolve$1 = (str, offset, radix, { intAsBigInt }) => intAsBigInt ? BigInt(str) : parseInt(str.substring(offset), radix);
	function intStringify$1(node, radix, prefix) {
		const { value } = node;
		if (intIdentify$2(value) && value >= 0) return prefix + value.toString(radix);
		return stringifyNumber(node);
	}
	var intOct$1 = {
		identify: (value) => intIdentify$2(value) && value >= 0,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "OCT",
		test: /^0o[0-7]+$/,
		resolve: (str, _onError, opt) => intResolve$1(str, 2, 8, opt),
		stringify: (node) => intStringify$1(node, 8, "0o")
	};
	var int$1 = {
		identify: intIdentify$2,
		default: true,
		tag: "tag:yaml.org,2002:int",
		test: /^[-+]?[0-9]+$/,
		resolve: (str, _onError, opt) => intResolve$1(str, 0, 10, opt),
		stringify: stringifyNumber
	};
	var intHex$1 = {
		identify: (value) => intIdentify$2(value) && value >= 0,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "HEX",
		test: /^0x[0-9a-fA-F]+$/,
		resolve: (str, _onError, opt) => intResolve$1(str, 2, 16, opt),
		stringify: (node) => intStringify$1(node, 16, "0x")
	};
	var schema$3 = [
		map,
		seq,
		string,
		nullTag,
		boolTag,
		intOct$1,
		int$1,
		intHex$1,
		floatNaN$1,
		floatExp$1,
		float$1
	];
	function intIdentify$1(value) {
		return typeof value === "bigint" || Number.isInteger(value);
	}
	var stringifyJSON = ({ value }) => JSON.stringify(value);
	var jsonScalars = [
		{
			identify: (value) => typeof value === "string",
			default: true,
			tag: "tag:yaml.org,2002:str",
			resolve: (str) => str,
			stringify: stringifyJSON
		},
		{
			identify: (value) => value == null,
			createNode: () => new Scalar(null),
			default: true,
			tag: "tag:yaml.org,2002:null",
			test: /^null$/,
			resolve: () => null,
			stringify: stringifyJSON
		},
		{
			identify: (value) => typeof value === "boolean",
			default: true,
			tag: "tag:yaml.org,2002:bool",
			test: /^true$|^false$/,
			resolve: (str) => str === "true",
			stringify: stringifyJSON
		},
		{
			identify: intIdentify$1,
			default: true,
			tag: "tag:yaml.org,2002:int",
			test: /^-?(?:0|[1-9][0-9]*)$/,
			resolve: (str, _onError, { intAsBigInt }) => intAsBigInt ? BigInt(str) : parseInt(str, 10),
			stringify: ({ value }) => intIdentify$1(value) ? value.toString() : JSON.stringify(value)
		},
		{
			identify: (value) => typeof value === "number",
			default: true,
			tag: "tag:yaml.org,2002:float",
			test: /^-?(?:0|[1-9][0-9]*)(?:\.[0-9]*)?(?:[eE][-+]?[0-9]+)?$/,
			resolve: (str) => parseFloat(str),
			stringify: stringifyJSON
		}
	];
	var schema$2 = [map, seq].concat(jsonScalars, {
		default: true,
		tag: "",
		test: /^/,
		resolve(str, onError) {
			onError(`Unresolved plain scalar ${JSON.stringify(str)}`);
			return str;
		}
	});
	var binary = {
		identify: (value) => value instanceof Uint8Array,
		default: false,
		tag: "tag:yaml.org,2002:binary",
		/**
		* Returns a Buffer in node and an Uint8Array in browsers
		*
		* To use the resulting buffer as an image, you'll want to do something like:
		*
		*   const blob = new Blob([buffer], { type: 'image/jpeg' })
		*   document.querySelector('#photo').src = URL.createObjectURL(blob)
		*/
		resolve(src, onError) {
			if (typeof atob === "function") {
				const str = atob(src.replace(/[\n\r]/g, ""));
				const buffer = new Uint8Array(str.length);
				for (let i = 0; i < str.length; ++i) buffer[i] = str.charCodeAt(i);
				return buffer;
			} else {
				onError("This environment does not support reading binary tags; either Buffer or atob is required");
				return src;
			}
		},
		stringify({ comment, type, value }, ctx, onComment, onChompKeep) {
			if (!value) return "";
			const buf = value;
			let str;
			if (typeof btoa === "function") {
				let s = "";
				for (let i = 0; i < buf.length; ++i) s += String.fromCharCode(buf[i]);
				str = btoa(s);
			} else throw new Error("This environment does not support writing binary tags; either Buffer or btoa is required");
			type ?? (type = Scalar.BLOCK_LITERAL);
			if (type !== Scalar.QUOTE_DOUBLE) {
				const lineWidth = Math.max(ctx.options.lineWidth - ctx.indent.length, ctx.options.minContentWidth);
				const n = Math.ceil(str.length / lineWidth);
				const lines = new Array(n);
				for (let i = 0, o = 0; i < n; ++i, o += lineWidth) lines[i] = str.substr(o, lineWidth);
				str = lines.join(type === Scalar.BLOCK_LITERAL ? "\n" : " ");
			}
			return stringifyString({
				comment,
				type,
				value: str
			}, ctx, onComment, onChompKeep);
		}
	};
	function resolvePairs(seq, onError) {
		if (isSeq(seq)) for (let i = 0; i < seq.items.length; ++i) {
			let item = seq.items[i];
			if (isPair(item)) continue;
			else if (isMap(item)) {
				if (item.items.length > 1) onError("Each pair must have its own sequence indicator");
				const pair = item.items[0] || new Pair(new Scalar(null));
				if (item.commentBefore) pair.key.commentBefore = pair.key.commentBefore ? `${item.commentBefore}\n${pair.key.commentBefore}` : item.commentBefore;
				if (item.comment) {
					const cn = pair.value ?? pair.key;
					cn.comment = cn.comment ? `${item.comment}\n${cn.comment}` : item.comment;
				}
				item = pair;
			}
			seq.items[i] = isPair(item) ? item : new Pair(item);
		}
		else onError("Expected a sequence for this tag");
		return seq;
	}
	function createPairs(schema, iterable, ctx) {
		const { replacer } = ctx;
		const pairs = new YAMLSeq(schema);
		pairs.tag = "tag:yaml.org,2002:pairs";
		let i = 0;
		if (iterable && Symbol.iterator in Object(iterable)) for (let it of iterable) {
			if (typeof replacer === "function") it = replacer.call(iterable, String(i++), it);
			let key, value;
			if (Array.isArray(it)) {
				if (it.length === 2) {
					key = it[0];
					value = it[1];
				} else throw new TypeError(`Expected [key, value] tuple: ${it}`);
			} else if (it && it instanceof Object) {
				const keys = Object.keys(it);
				if (keys.length === 1) {
					key = keys[0];
					value = it[key];
				} else throw new TypeError(`Expected tuple with one key, not ${keys.length} keys`);
			} else key = it;
			pairs.items.push(createPair(key, value, ctx));
		}
		return pairs;
	}
	var pairs = {
		collection: "seq",
		default: false,
		tag: "tag:yaml.org,2002:pairs",
		resolve: resolvePairs,
		createNode: createPairs
	};
	var YAMLOMap = class YAMLOMap extends YAMLSeq {
		constructor() {
			super();
			this.add = YAMLMap.prototype.add.bind(this);
			this.delete = YAMLMap.prototype.delete.bind(this);
			this.get = YAMLMap.prototype.get.bind(this);
			this.has = YAMLMap.prototype.has.bind(this);
			this.set = YAMLMap.prototype.set.bind(this);
			this.tag = YAMLOMap.tag;
		}
		/**
		* If `ctx` is given, the return type is actually `Map<unknown, unknown>`,
		* but TypeScript won't allow widening the signature of a child method.
		*/
		toJSON(_, ctx) {
			if (!ctx) return super.toJSON(_);
			const map = /* @__PURE__ */ new Map();
			if (ctx?.onCreate) ctx.onCreate(map);
			for (const pair of this.items) {
				let key, value;
				if (isPair(pair)) {
					key = toJS(pair.key, "", ctx);
					value = toJS(pair.value, key, ctx);
				} else key = toJS(pair, "", ctx);
				if (map.has(key)) throw new Error("Ordered maps must not include duplicate keys");
				map.set(key, value);
			}
			return map;
		}
		static from(schema, iterable, ctx) {
			const pairs = createPairs(schema, iterable, ctx);
			const omap = new this();
			omap.items = pairs.items;
			return omap;
		}
	};
	YAMLOMap.tag = "tag:yaml.org,2002:omap";
	var omap = {
		collection: "seq",
		identify: (value) => value instanceof Map,
		nodeClass: YAMLOMap,
		default: false,
		tag: "tag:yaml.org,2002:omap",
		resolve(seq, onError) {
			const pairs = resolvePairs(seq, onError);
			const seenKeys = [];
			for (const { key } of pairs.items) if (isScalar(key)) {
				if (seenKeys.includes(key.value)) onError(`Ordered maps must not include duplicate keys: ${key.value}`);
				else seenKeys.push(key.value);
			}
			return Object.assign(new YAMLOMap(), pairs);
		},
		createNode: (schema, iterable, ctx) => YAMLOMap.from(schema, iterable, ctx)
	};
	function boolStringify({ value, source }, ctx) {
		if (source && (value ? trueTag : falseTag).test.test(source)) return source;
		return value ? ctx.options.trueStr : ctx.options.falseStr;
	}
	var trueTag = {
		identify: (value) => value === true,
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:Y|y|[Yy]es|YES|[Tt]rue|TRUE|[Oo]n|ON)$/,
		resolve: () => new Scalar(true),
		stringify: boolStringify
	};
	var falseTag = {
		identify: (value) => value === false,
		default: true,
		tag: "tag:yaml.org,2002:bool",
		test: /^(?:N|n|[Nn]o|NO|[Ff]alse|FALSE|[Oo]ff|OFF)$/,
		resolve: () => new Scalar(false),
		stringify: boolStringify
	};
	var floatNaN = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^(?:[-+]?\.(?:inf|Inf|INF)|\.nan|\.NaN|\.NAN)$/,
		resolve: (str) => str.slice(-3).toLowerCase() === "nan" ? NaN : str[0] === "-" ? Number.NEGATIVE_INFINITY : Number.POSITIVE_INFINITY,
		stringify: stringifyNumber
	};
	var floatExp = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "EXP",
		test: /^[-+]?(?:[0-9][0-9_]*)?(?:\.[0-9_]*)?[eE][-+]?[0-9]+$/,
		resolve: (str) => parseFloat(str.replace(/_/g, "")),
		stringify(node) {
			const num = Number(node.value);
			return isFinite(num) ? num.toExponential() : stringifyNumber(node);
		}
	};
	var float = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		test: /^[-+]?(?:[0-9][0-9_]*)?\.[0-9_]*$/,
		resolve(str) {
			const node = new Scalar(parseFloat(str.replace(/_/g, "")));
			const dot = str.indexOf(".");
			if (dot !== -1) {
				const f = str.substring(dot + 1).replace(/_/g, "");
				if (f[f.length - 1] === "0") node.minFractionDigits = f.length;
			}
			return node;
		},
		stringify: stringifyNumber
	};
	var intIdentify = (value) => typeof value === "bigint" || Number.isInteger(value);
	function intResolve(str, offset, radix, { intAsBigInt }) {
		const sign = str[0];
		if (sign === "-" || sign === "+") offset += 1;
		str = str.substring(offset).replace(/_/g, "");
		if (intAsBigInt) {
			switch (radix) {
				case 2:
					str = `0b${str}`;
					break;
				case 8:
					str = `0o${str}`;
					break;
				case 16: str = `0x${str}`;
			}
			const n = BigInt(str);
			return sign === "-" ? BigInt(-1) * n : n;
		}
		const n = parseInt(str, radix);
		return sign === "-" ? -1 * n : n;
	}
	function intStringify(node, radix, prefix) {
		const { value } = node;
		if (intIdentify(value)) {
			const str = value.toString(radix);
			return value < 0 ? "-" + prefix + str.substr(1) : prefix + str;
		}
		return stringifyNumber(node);
	}
	var intBin = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "BIN",
		test: /^[-+]?0b[0-1_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 2, opt),
		stringify: (node) => intStringify(node, 2, "0b")
	};
	var intOct = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "OCT",
		test: /^[-+]?0[0-7_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 1, 8, opt),
		stringify: (node) => intStringify(node, 8, "0")
	};
	var int = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		test: /^[-+]?[0-9][0-9_]*$/,
		resolve: (str, _onError, opt) => intResolve(str, 0, 10, opt),
		stringify: stringifyNumber
	};
	var intHex = {
		identify: intIdentify,
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "HEX",
		test: /^[-+]?0x[0-9a-fA-F_]+$/,
		resolve: (str, _onError, opt) => intResolve(str, 2, 16, opt),
		stringify: (node) => intStringify(node, 16, "0x")
	};
	var YAMLSet = class YAMLSet extends YAMLMap {
		constructor(schema) {
			super(schema);
			this.tag = YAMLSet.tag;
		}
		add(key) {
			let pair;
			if (isPair(key)) pair = key;
			else if (key && typeof key === "object" && "key" in key && "value" in key && key.value === null) pair = new Pair(key.key, null);
			else pair = new Pair(key, null);
			if (!findPair(this.items, pair.key)) this.items.push(pair);
		}
		/**
		* If `keepPair` is `true`, returns the Pair matching `key`.
		* Otherwise, returns the value of that Pair's key.
		*/
		get(key, keepPair) {
			const pair = findPair(this.items, key);
			return !keepPair && isPair(pair) ? isScalar(pair.key) ? pair.key.value : pair.key : pair;
		}
		set(key, value) {
			if (typeof value !== "boolean") throw new Error(`Expected boolean value for set(key, value) in a YAML set, not ${typeof value}`);
			const prev = findPair(this.items, key);
			if (prev && !value) this.items.splice(this.items.indexOf(prev), 1);
			else if (!prev && value) this.items.push(new Pair(key));
		}
		toJSON(_, ctx) {
			return super.toJSON(_, ctx, Set);
		}
		toString(ctx, onComment, onChompKeep) {
			if (!ctx) return JSON.stringify(this);
			if (this.hasAllNullValues(true)) return super.toString(Object.assign({}, ctx, { allNullValues: true }), onComment, onChompKeep);
			else throw new Error("Set items must all have null values");
		}
		static from(schema, iterable, ctx) {
			const { replacer } = ctx;
			const set = new this(schema);
			if (iterable && Symbol.iterator in Object(iterable)) for (let value of iterable) {
				if (typeof replacer === "function") value = replacer.call(iterable, value, value);
				set.items.push(createPair(value, null, ctx));
			}
			return set;
		}
	};
	YAMLSet.tag = "tag:yaml.org,2002:set";
	var set = {
		collection: "map",
		identify: (value) => value instanceof Set,
		nodeClass: YAMLSet,
		default: false,
		tag: "tag:yaml.org,2002:set",
		createNode: (schema, iterable, ctx) => YAMLSet.from(schema, iterable, ctx),
		resolve(map, onError) {
			if (isMap(map)) {
				if (map.hasAllNullValues(true)) return Object.assign(new YAMLSet(), map);
				else onError("Set items must all have null values");
			} else onError("Expected a mapping for this tag");
			return map;
		}
	};
	/** Internal types handle bigint as number, because TS can't figure it out. */
	function parseSexagesimal(str, asBigInt) {
		const sign = str[0];
		const parts = sign === "-" || sign === "+" ? str.substring(1) : str;
		const num = (n) => asBigInt ? BigInt(n) : Number(n);
		const res = parts.replace(/_/g, "").split(":").reduce((res, p) => res * num(60) + num(p), num(0));
		return sign === "-" ? num(-1) * res : res;
	}
	/**
	* hhhh:mm:ss.sss
	*
	* Internal types handle bigint as number, because TS can't figure it out.
	*/
	function stringifySexagesimal(node) {
		let { value } = node;
		let num = (n) => n;
		if (typeof value === "bigint") num = (n) => BigInt(n);
		else if (isNaN(value) || !isFinite(value)) return stringifyNumber(node);
		let sign = "";
		if (value < 0) {
			sign = "-";
			value *= num(-1);
		}
		const _60 = num(60);
		const parts = [value % _60];
		if (value < 60) parts.unshift(0);
		else {
			value = (value - parts[0]) / _60;
			parts.unshift(value % _60);
			if (value >= 60) {
				value = (value - parts[0]) / _60;
				parts.unshift(value);
			}
		}
		return sign + parts.map((n) => String(n).padStart(2, "0")).join(":").replace(/000000\d*$/, "");
	}
	var intTime = {
		identify: (value) => typeof value === "bigint" || Number.isInteger(value),
		default: true,
		tag: "tag:yaml.org,2002:int",
		format: "TIME",
		test: /^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+$/,
		resolve: (str, _onError, { intAsBigInt }) => parseSexagesimal(str, intAsBigInt),
		stringify: stringifySexagesimal
	};
	var floatTime = {
		identify: (value) => typeof value === "number",
		default: true,
		tag: "tag:yaml.org,2002:float",
		format: "TIME",
		test: /^[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*$/,
		resolve: (str) => parseSexagesimal(str, false),
		stringify: stringifySexagesimal
	};
	var timestamp = {
		identify: (value) => value instanceof Date,
		default: true,
		tag: "tag:yaml.org,2002:timestamp",
		test: RegExp("^([0-9]{4})-([0-9]{1,2})-([0-9]{1,2})(?:(?:t|T|[ \\t]+)([0-9]{1,2}):([0-9]{1,2}):([0-9]{1,2}(\\.[0-9]+)?)(?:[ \\t]*(Z|[-+][012]?[0-9](?::[0-9]{2})?))?)?$"),
		resolve(str) {
			const match = str.match(timestamp.test);
			if (!match) throw new Error("!!timestamp expects a date, starting with yyyy-mm-dd");
			const [, year, month, day, hour, minute, second] = match.map(Number);
			const millisec = match[7] ? Number((match[7] + "00").substr(1, 3)) : 0;
			let date = Date.UTC(year, month - 1, day, hour || 0, minute || 0, second || 0, millisec);
			const tz = match[8];
			if (tz && tz !== "Z") {
				let d = parseSexagesimal(tz, false);
				if (Math.abs(d) < 30) d *= 60;
				date -= 6e4 * d;
			}
			return new Date(date);
		},
		stringify: ({ value }) => value?.toISOString().replace(/(T00:00:00)?\.000Z$/, "") ?? ""
	};
	var schema$1 = [
		map,
		seq,
		string,
		nullTag,
		trueTag,
		falseTag,
		intBin,
		intOct,
		int,
		intHex,
		floatNaN,
		floatExp,
		float,
		binary,
		merge,
		omap,
		pairs,
		set,
		intTime,
		floatTime,
		timestamp
	];
	var schemas = /* @__PURE__ */ new Map([
		["core", schema$3],
		["failsafe", [
			map,
			seq,
			string
		]],
		["json", schema$2],
		["yaml11", schema$1],
		["yaml-1.1", schema$1]
	]);
	var tagsByName = {
		binary,
		bool: boolTag,
		float: float$1,
		floatExp: floatExp$1,
		floatNaN: floatNaN$1,
		floatTime,
		int: int$1,
		intHex: intHex$1,
		intOct: intOct$1,
		intTime,
		map,
		merge,
		null: nullTag,
		omap,
		pairs,
		seq,
		set,
		timestamp
	};
	var coreKnownTags = {
		"tag:yaml.org,2002:binary": binary,
		"tag:yaml.org,2002:merge": merge,
		"tag:yaml.org,2002:omap": omap,
		"tag:yaml.org,2002:pairs": pairs,
		"tag:yaml.org,2002:set": set,
		"tag:yaml.org,2002:timestamp": timestamp
	};
	function getTags(customTags, schemaName, addMergeTag) {
		const schemaTags = schemas.get(schemaName);
		if (schemaTags && !customTags) return addMergeTag && !schemaTags.includes(merge) ? schemaTags.concat(merge) : schemaTags.slice();
		let tags = schemaTags;
		if (!tags) {
			if (Array.isArray(customTags)) tags = [];
			else {
				const keys = Array.from(schemas.keys()).filter((key) => key !== "yaml11").map((key) => JSON.stringify(key)).join(", ");
				throw new Error(`Unknown schema "${schemaName}"; use one of ${keys} or define customTags array`);
			}
		}
		if (Array.isArray(customTags)) for (const tag of customTags) tags = tags.concat(tag);
		else if (typeof customTags === "function") tags = customTags(tags.slice());
		if (addMergeTag) tags = tags.concat(merge);
		return tags.reduce((tags, tag) => {
			const tagObj = typeof tag === "string" ? tagsByName[tag] : tag;
			if (!tagObj) {
				const tagName = JSON.stringify(tag);
				const keys = Object.keys(tagsByName).map((key) => JSON.stringify(key)).join(", ");
				throw new Error(`Unknown custom tag ${tagName}; use one of ${keys}`);
			}
			if (!tags.includes(tagObj)) tags.push(tagObj);
			return tags;
		}, []);
	}
	var sortMapEntriesByKey = (a, b) => a.key < b.key ? -1 : a.key > b.key ? 1 : 0;
	var Schema = class Schema {
		constructor({ compat, customTags, merge, resolveKnownTags, schema, sortMapEntries, toStringDefaults }) {
			this.compat = Array.isArray(compat) ? getTags(compat, "compat") : compat ? getTags(null, compat) : null;
			this.name = typeof schema === "string" && schema || "core";
			this.knownTags = resolveKnownTags ? coreKnownTags : {};
			this.tags = getTags(customTags, this.name, merge);
			this.toStringOptions = toStringDefaults ?? null;
			Object.defineProperty(this, MAP, { value: map });
			Object.defineProperty(this, SCALAR$1, { value: string });
			Object.defineProperty(this, SEQ, { value: seq });
			this.sortMapEntries = typeof sortMapEntries === "function" ? sortMapEntries : sortMapEntries === true ? sortMapEntriesByKey : null;
		}
		clone() {
			const copy = Object.create(Schema.prototype, Object.getOwnPropertyDescriptors(this));
			copy.tags = this.tags.slice();
			return copy;
		}
	};
	function stringifyDocument(doc, options) {
		const lines = [];
		let hasDirectives = options.directives === true;
		if (options.directives !== false && doc.directives) {
			const dir = doc.directives.toString(doc);
			if (dir) {
				lines.push(dir);
				hasDirectives = true;
			} else if (doc.directives.docStart) hasDirectives = true;
		}
		if (hasDirectives) lines.push("---");
		const ctx = createStringifyContext(doc, options);
		const { commentString } = ctx.options;
		if (doc.commentBefore) {
			if (lines.length !== 1) lines.unshift("");
			const cs = commentString(doc.commentBefore);
			lines.unshift(indentComment(cs, ""));
		}
		let chompKeep = false;
		let contentComment = null;
		if (doc.contents) {
			if (isNode$1(doc.contents)) {
				if (doc.contents.spaceBefore && hasDirectives) lines.push("");
				if (doc.contents.commentBefore) {
					const cs = commentString(doc.contents.commentBefore);
					lines.push(indentComment(cs, ""));
				}
				ctx.forceBlockIndent = !!doc.comment;
				contentComment = doc.contents.comment;
			}
			const onChompKeep = contentComment ? void 0 : () => chompKeep = true;
			let body = stringify(doc.contents, ctx, () => contentComment = null, onChompKeep);
			if (contentComment) body += lineComment(body, "", commentString(contentComment));
			if ((body[0] === "|" || body[0] === ">") && lines[lines.length - 1] === "---") lines[lines.length - 1] = `--- ${body}`;
			else lines.push(body);
		} else lines.push(stringify(doc.contents, ctx));
		if (doc.directives?.docEnd) {
			if (doc.comment) {
				const cs = commentString(doc.comment);
				if (cs.includes("\n")) {
					lines.push("...");
					lines.push(indentComment(cs, ""));
				} else lines.push(`... ${cs}`);
			} else lines.push("...");
		} else {
			let dc = doc.comment;
			if (dc && chompKeep) dc = dc.replace(/^\n+/, "");
			if (dc) {
				if ((!chompKeep || contentComment) && lines[lines.length - 1] !== "") lines.push("");
				lines.push(indentComment(commentString(dc), ""));
			}
		}
		return lines.join("\n") + "\n";
	}
	var Document$1 = class Document$1 {
		constructor(value, replacer, options) {
			/** A comment before this Document */
			this.commentBefore = null;
			/** A comment immediately after this Document */
			this.comment = null;
			/** Errors encountered during parsing. */
			this.errors = [];
			/** Warnings encountered during parsing. */
			this.warnings = [];
			Object.defineProperty(this, NODE_TYPE, { value: DOC });
			let _replacer = null;
			if (typeof replacer === "function" || Array.isArray(replacer)) _replacer = replacer;
			else if (options === void 0 && replacer) {
				options = replacer;
				replacer = void 0;
			}
			const opt = Object.assign({
				intAsBigInt: false,
				keepSourceTokens: false,
				logLevel: "warn",
				prettyErrors: true,
				strict: true,
				stringKeys: false,
				uniqueKeys: true,
				version: "1.2"
			}, options);
			this.options = opt;
			let { version } = opt;
			if (options?._directives) {
				this.directives = options._directives.atDocument();
				if (this.directives.yaml.explicit) version = this.directives.yaml.version;
			} else this.directives = new Directives({ version });
			this.setSchema(version, options);
			this.contents = value === void 0 ? null : this.createNode(value, _replacer, options);
		}
		/**
		* Create a deep copy of this Document and its contents.
		*
		* Custom Node values that inherit from `Object` still refer to their original instances.
		*/
		clone() {
			const copy = Object.create(Document$1.prototype, { [NODE_TYPE]: { value: DOC } });
			copy.commentBefore = this.commentBefore;
			copy.comment = this.comment;
			copy.errors = this.errors.slice();
			copy.warnings = this.warnings.slice();
			copy.options = Object.assign({}, this.options);
			if (this.directives) copy.directives = this.directives.clone();
			copy.schema = this.schema.clone();
			copy.contents = isNode$1(this.contents) ? this.contents.clone(copy.schema) : this.contents;
			if (this.range) copy.range = this.range.slice();
			return copy;
		}
		/** Adds a value to the document. */
		add(value) {
			if (assertCollection(this.contents)) this.contents.add(value);
		}
		/** Adds a value to the document. */
		addIn(path, value) {
			if (assertCollection(this.contents)) this.contents.addIn(path, value);
		}
		/**
		* Create a new `Alias` node, ensuring that the target `node` has the required anchor.
		*
		* If `node` already has an anchor, `name` is ignored.
		* Otherwise, the `node.anchor` value will be set to `name`,
		* or if an anchor with that name is already present in the document,
		* `name` will be used as a prefix for a new unique anchor.
		* If `name` is undefined, the generated anchor will use 'a' as a prefix.
		*/
		createAlias(node, name) {
			if (!node.anchor) {
				const prev = anchorNames(this);
				node.anchor = !name || prev.has(name) ? findNewAnchor(name || "a", prev) : name;
			}
			return new Alias(node.anchor);
		}
		createNode(value, replacer, options) {
			let _replacer = void 0;
			if (typeof replacer === "function") {
				value = replacer.call({ "": value }, "", value);
				_replacer = replacer;
			} else if (Array.isArray(replacer)) {
				const keyToStr = (v) => typeof v === "number" || v instanceof String || v instanceof Number;
				const asStr = replacer.filter(keyToStr).map(String);
				if (asStr.length > 0) replacer = replacer.concat(asStr);
				_replacer = replacer;
			} else if (options === void 0 && replacer) {
				options = replacer;
				replacer = void 0;
			}
			const { aliasDuplicateObjects, anchorPrefix, flow, keepUndefined, onTagObj, tag } = options ?? {};
			const { onAnchor, setAnchors, sourceObjects } = createNodeAnchors(this, anchorPrefix || "a");
			const ctx = {
				aliasDuplicateObjects: aliasDuplicateObjects ?? true,
				keepUndefined: keepUndefined ?? false,
				onAnchor,
				onTagObj,
				replacer: _replacer,
				schema: this.schema,
				sourceObjects
			};
			const node = createNode(value, tag, ctx);
			if (flow && isCollection(node)) node.flow = true;
			setAnchors();
			return node;
		}
		/**
		* Convert a key and a value into a `Pair` using the current schema,
		* recursively wrapping all values as `Scalar` or `Collection` nodes.
		*/
		createPair(key, value, options = {}) {
			return new Pair(this.createNode(key, null, options), this.createNode(value, null, options));
		}
		/**
		* Removes a value from the document.
		* @returns `true` if the item was found and removed.
		*/
		delete(key) {
			return assertCollection(this.contents) ? this.contents.delete(key) : false;
		}
		/**
		* Removes a value from the document.
		* @returns `true` if the item was found and removed.
		*/
		deleteIn(path) {
			if (isEmptyPath(path)) {
				if (this.contents == null) return false;
				this.contents = null;
				return true;
			}
			return assertCollection(this.contents) ? this.contents.deleteIn(path) : false;
		}
		/**
		* Returns item at `key`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		get(key, keepScalar) {
			return isCollection(this.contents) ? this.contents.get(key, keepScalar) : void 0;
		}
		/**
		* Returns item at `path`, or `undefined` if not found. By default unwraps
		* scalar values from their surrounding node; to disable set `keepScalar` to
		* `true` (collections are always returned intact).
		*/
		getIn(path, keepScalar) {
			if (isEmptyPath(path)) return !keepScalar && isScalar(this.contents) ? this.contents.value : this.contents;
			return isCollection(this.contents) ? this.contents.getIn(path, keepScalar) : void 0;
		}
		/**
		* Checks if the document includes a value with the key `key`.
		*/
		has(key) {
			return isCollection(this.contents) ? this.contents.has(key) : false;
		}
		/**
		* Checks if the document includes a value at `path`.
		*/
		hasIn(path) {
			if (isEmptyPath(path)) return this.contents !== void 0;
			return isCollection(this.contents) ? this.contents.hasIn(path) : false;
		}
		/**
		* Sets a value in this document. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		set(key, value) {
			if (this.contents == null) this.contents = collectionFromPath(this.schema, [key], value);
			else if (assertCollection(this.contents)) this.contents.set(key, value);
		}
		/**
		* Sets a value in this document. For `!!set`, `value` needs to be a
		* boolean to add/remove the item from the set.
		*/
		setIn(path, value) {
			if (isEmptyPath(path)) this.contents = value;
			else if (this.contents == null) this.contents = collectionFromPath(this.schema, Array.from(path), value);
			else if (assertCollection(this.contents)) this.contents.setIn(path, value);
		}
		/**
		* Change the YAML version and schema used by the document.
		* A `null` version disables support for directives, explicit tags, anchors, and aliases.
		* It also requires the `schema` option to be given as a `Schema` instance value.
		*
		* Overrides all previously set schema options.
		*/
		setSchema(version, options = {}) {
			if (typeof version === "number") version = String(version);
			let opt;
			switch (version) {
				case "1.1":
					if (this.directives) this.directives.yaml.version = "1.1";
					else this.directives = new Directives({ version: "1.1" });
					opt = {
						resolveKnownTags: false,
						schema: "yaml-1.1"
					};
					break;
				case "1.2":
				case "next":
					if (this.directives) this.directives.yaml.version = version;
					else this.directives = new Directives({ version });
					opt = {
						resolveKnownTags: true,
						schema: "core"
					};
					break;
				case null:
					if (this.directives) delete this.directives;
					opt = null;
					break;
				default: {
					const sv = JSON.stringify(version);
					throw new Error(`Expected '1.1', '1.2' or null as first argument, but found: ${sv}`);
				}
			}
			if (options.schema instanceof Object) this.schema = options.schema;
			else if (opt) this.schema = new Schema(Object.assign(opt, options));
			else throw new Error(`With a null YAML version, the { schema: Schema } option is required`);
		}
		toJS({ json, jsonArg, mapAsMap, maxAliasCount, onAnchor, reviver } = {}) {
			const ctx = {
				anchors: /* @__PURE__ */ new Map(),
				doc: this,
				keep: !json,
				mapAsMap: mapAsMap === true,
				mapKeyWarned: false,
				maxAliasCount: typeof maxAliasCount === "number" ? maxAliasCount : 100
			};
			const res = toJS(this.contents, jsonArg ?? "", ctx);
			if (typeof onAnchor === "function") for (const { count, res } of ctx.anchors.values()) onAnchor(res, count);
			return typeof reviver === "function" ? applyReviver(reviver, { "": res }, "", res) : res;
		}
		/**
		* A JSON representation of the document `contents`.
		*
		* @param jsonArg Used by `JSON.stringify` to indicate the array index or
		*   property name.
		*/
		toJSON(jsonArg, onAnchor) {
			return this.toJS({
				json: true,
				jsonArg,
				mapAsMap: false,
				onAnchor
			});
		}
		/** A YAML representation of the document. */
		toString(options = {}) {
			if (this.errors.length > 0) throw new Error("Document with errors cannot be stringified");
			if ("indent" in options && (!Number.isInteger(options.indent) || Number(options.indent) <= 0)) {
				const s = JSON.stringify(options.indent);
				throw new Error(`"indent" option must be a positive integer, not ${s}`);
			}
			return stringifyDocument(this, options);
		}
	};
	function assertCollection(contents) {
		if (isCollection(contents)) return true;
		throw new Error("Expected a YAML collection as document contents");
	}
	var YAMLError = class extends Error {
		constructor(name, pos, code, message) {
			super();
			this.name = name;
			this.code = code;
			this.message = message;
			this.pos = pos;
		}
	};
	var YAMLParseError = class extends YAMLError {
		constructor(pos, code, message) {
			super("YAMLParseError", pos, code, message);
		}
	};
	var YAMLWarning = class extends YAMLError {
		constructor(pos, code, message) {
			super("YAMLWarning", pos, code, message);
		}
	};
	var prettifyError = (src, lc) => (error) => {
		if (error.pos[0] === -1) return;
		error.linePos = error.pos.map((pos) => lc.linePos(pos));
		const { line, col } = error.linePos[0];
		error.message += ` at line ${line}, column ${col}`;
		let ci = col - 1;
		let lineStr = src.substring(lc.lineStarts[line - 1], lc.lineStarts[line]).replace(/[\n\r]+$/, "");
		if (ci >= 60 && lineStr.length > 80) {
			const trimStart = Math.min(ci - 39, lineStr.length - 79);
			lineStr = "…" + lineStr.substring(trimStart);
			ci -= trimStart - 1;
		}
		if (lineStr.length > 80) lineStr = lineStr.substring(0, 79) + "…";
		if (line > 1 && /^ *$/.test(lineStr.substring(0, ci))) {
			let prev = src.substring(lc.lineStarts[line - 2], lc.lineStarts[line - 1]);
			if (prev.length > 80) prev = prev.substring(0, 79) + "…\n";
			lineStr = prev + lineStr;
		}
		if (/[^ ]/.test(lineStr)) {
			let count = 1;
			const end = error.linePos[1];
			if (end?.line === line && end.col > col) count = Math.max(1, Math.min(end.col - col, 80 - ci));
			const pointer = " ".repeat(ci) + "^".repeat(count);
			error.message += `:\n\n${lineStr}\n${pointer}\n`;
		}
	};
	function resolveProps(tokens, { flow, indicator, next, offset, onError, parentIndent, startOnNewline }) {
		let spaceBefore = false;
		let atNewline = startOnNewline;
		let hasSpace = startOnNewline;
		let comment = "";
		let commentSep = "";
		let hasNewline = false;
		let reqSpace = false;
		let tab = null;
		let anchor = null;
		let tag = null;
		let newlineAfterProp = null;
		let comma = null;
		let found = null;
		let start = null;
		for (const token of tokens) {
			if (reqSpace) {
				if (token.type !== "space" && token.type !== "newline" && token.type !== "comma") onError(token.offset, "MISSING_CHAR", "Tags and anchors must be separated from the next token by white space");
				reqSpace = false;
			}
			if (tab) {
				if (atNewline && token.type !== "comment" && token.type !== "newline") onError(tab, "TAB_AS_INDENT", "Tabs are not allowed as indentation");
				tab = null;
			}
			switch (token.type) {
				case "space":
					if (!flow && (indicator !== "doc-start" || next?.type !== "flow-collection") && token.source.includes("	")) tab = token;
					hasSpace = true;
					break;
				case "comment": {
					if (!hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
					const cb = token.source.substring(1) || " ";
					if (!comment) comment = cb;
					else comment += commentSep + cb;
					commentSep = "";
					atNewline = false;
					break;
				}
				case "newline":
					if (atNewline) {
						if (comment) comment += token.source;
						else if (!found || indicator !== "seq-item-ind") spaceBefore = true;
					} else commentSep += token.source;
					atNewline = true;
					hasNewline = true;
					if (anchor || tag) newlineAfterProp = token;
					hasSpace = true;
					break;
				case "anchor":
					if (anchor) onError(token, "MULTIPLE_ANCHORS", "A node can have at most one anchor");
					if (token.source.endsWith(":")) onError(token.offset + token.source.length - 1, "BAD_ALIAS", "Anchor ending in : is ambiguous", true);
					anchor = token;
					start ?? (start = token.offset);
					atNewline = false;
					hasSpace = false;
					reqSpace = true;
					break;
				case "tag":
					if (tag) onError(token, "MULTIPLE_TAGS", "A node can have at most one tag");
					tag = token;
					start ?? (start = token.offset);
					atNewline = false;
					hasSpace = false;
					reqSpace = true;
					break;
				case indicator:
					if (anchor || tag) onError(token, "BAD_PROP_ORDER", `Anchors and tags must be after the ${token.source} indicator`);
					if (found) onError(token, "UNEXPECTED_TOKEN", `Unexpected ${token.source} in ${flow ?? "collection"}`);
					found = token;
					atNewline = indicator === "seq-item-ind" || indicator === "explicit-key-ind";
					hasSpace = false;
					break;
				case "comma": if (flow) {
					if (comma) onError(token, "UNEXPECTED_TOKEN", `Unexpected , in ${flow}`);
					comma = token;
					atNewline = false;
					hasSpace = false;
					break;
				}
				default:
					onError(token, "UNEXPECTED_TOKEN", `Unexpected ${token.type} token`);
					atNewline = false;
					hasSpace = false;
			}
		}
		const last = tokens[tokens.length - 1];
		const end = last ? last.offset + last.source.length : offset;
		if (reqSpace && next && next.type !== "space" && next.type !== "newline" && next.type !== "comma" && (next.type !== "scalar" || next.source !== "")) onError(next.offset, "MISSING_CHAR", "Tags and anchors must be separated from the next token by white space");
		if (tab && (atNewline && tab.indent <= parentIndent || next?.type === "block-map" || next?.type === "block-seq")) onError(tab, "TAB_AS_INDENT", "Tabs are not allowed as indentation");
		return {
			comma,
			found,
			spaceBefore,
			comment,
			hasNewline,
			anchor,
			tag,
			newlineAfterProp,
			end,
			start: start ?? end
		};
	}
	function containsNewline(key) {
		if (!key) return null;
		switch (key.type) {
			case "alias":
			case "scalar":
			case "double-quoted-scalar":
			case "single-quoted-scalar":
				if (key.source.includes("\n")) return true;
				if (key.end) {
					for (const st of key.end) if (st.type === "newline") return true;
				}
				return false;
			case "flow-collection":
				for (const it of key.items) {
					for (const st of it.start) if (st.type === "newline") return true;
					if (it.sep) {
						for (const st of it.sep) if (st.type === "newline") return true;
					}
					if (containsNewline(it.key) || containsNewline(it.value)) return true;
				}
				return false;
			default: return true;
		}
	}
	function flowIndentCheck(indent, fc, onError) {
		if (fc?.type === "flow-collection") {
			const end = fc.end[0];
			if (end.indent === indent && (end.source === "]" || end.source === "}") && containsNewline(fc)) onError(end, "BAD_INDENT", "Flow end indicator should be more indented than parent", true);
		}
	}
	function mapIncludes(ctx, items, search) {
		const { uniqueKeys } = ctx.options;
		if (uniqueKeys === false) return false;
		const isEqual = typeof uniqueKeys === "function" ? uniqueKeys : (a, b) => a === b || isScalar(a) && isScalar(b) && a.value === b.value;
		return items.some((pair) => isEqual(pair.key, search));
	}
	var startColMsg = "All mapping items must start at the same column";
	function resolveBlockMap({ composeNode, composeEmptyNode }, ctx, bm, onError, tag) {
		const map = new ((tag?.nodeClass) ?? YAMLMap)(ctx.schema);
		if (ctx.atRoot) ctx.atRoot = false;
		let offset = bm.offset;
		let commentEnd = null;
		for (const collItem of bm.items) {
			const { start, key, sep, value } = collItem;
			const keyProps = resolveProps(start, {
				indicator: "explicit-key-ind",
				next: key ?? sep?.[0],
				offset,
				onError,
				parentIndent: bm.indent,
				startOnNewline: true
			});
			const implicitKey = !keyProps.found;
			if (implicitKey) {
				if (key) {
					if (key.type === "block-seq") onError(offset, "BLOCK_AS_IMPLICIT_KEY", "A block sequence may not be used as an implicit map key");
					else if ("indent" in key && key.indent !== bm.indent) onError(offset, "BAD_INDENT", startColMsg);
				}
				if (!keyProps.anchor && !keyProps.tag && !sep) {
					commentEnd = keyProps.end;
					if (keyProps.comment) {
						if (map.comment) map.comment += "\n" + keyProps.comment;
						else map.comment = keyProps.comment;
					}
					continue;
				}
				if (keyProps.newlineAfterProp || containsNewline(key)) onError(key ?? start[start.length - 1], "MULTILINE_IMPLICIT_KEY", "Implicit keys need to be on a single line");
			} else if (keyProps.found?.indent !== bm.indent) onError(offset, "BAD_INDENT", startColMsg);
			ctx.atKey = true;
			const keyStart = keyProps.end;
			const keyNode = key ? composeNode(ctx, key, keyProps, onError) : composeEmptyNode(ctx, keyStart, start, null, keyProps, onError);
			if (ctx.schema.compat) flowIndentCheck(bm.indent, key, onError);
			ctx.atKey = false;
			if (mapIncludes(ctx, map.items, keyNode)) onError(keyStart, "DUPLICATE_KEY", "Map keys must be unique");
			const valueProps = resolveProps(sep ?? [], {
				indicator: "map-value-ind",
				next: value,
				offset: keyNode.range[2],
				onError,
				parentIndent: bm.indent,
				startOnNewline: !key || key.type === "block-scalar"
			});
			offset = valueProps.end;
			if (valueProps.found) {
				if (implicitKey) {
					if (value?.type === "block-map" && !valueProps.hasNewline) onError(offset, "BLOCK_AS_IMPLICIT_KEY", "Nested mappings are not allowed in compact mappings");
					if (ctx.options.strict && keyProps.start < valueProps.found.offset - 1024) onError(keyNode.range, "KEY_OVER_1024_CHARS", "The : indicator must be at most 1024 chars after the start of an implicit block mapping key");
				}
				const valueNode = value ? composeNode(ctx, value, valueProps, onError) : composeEmptyNode(ctx, offset, sep, null, valueProps, onError);
				if (ctx.schema.compat) flowIndentCheck(bm.indent, value, onError);
				offset = valueNode.range[2];
				const pair = new Pair(keyNode, valueNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				map.items.push(pair);
			} else {
				if (implicitKey) onError(keyNode.range, "MISSING_CHAR", "Implicit map keys need to be followed by map values");
				if (valueProps.comment) {
					if (keyNode.comment) keyNode.comment += "\n" + valueProps.comment;
					else keyNode.comment = valueProps.comment;
				}
				const pair = new Pair(keyNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				map.items.push(pair);
			}
		}
		if (commentEnd && commentEnd < offset) onError(commentEnd, "IMPOSSIBLE", "Map comment with trailing content");
		map.range = [
			bm.offset,
			offset,
			commentEnd ?? offset
		];
		return map;
	}
	function resolveBlockSeq({ composeNode, composeEmptyNode }, ctx, bs, onError, tag) {
		const seq = new ((tag?.nodeClass) ?? YAMLSeq)(ctx.schema);
		if (ctx.atRoot) ctx.atRoot = false;
		if (ctx.atKey) ctx.atKey = false;
		let offset = bs.offset;
		let commentEnd = null;
		for (const { start, value } of bs.items) {
			const props = resolveProps(start, {
				indicator: "seq-item-ind",
				next: value,
				offset,
				onError,
				parentIndent: bs.indent,
				startOnNewline: true
			});
			if (!props.found) {
				if (props.anchor || props.tag || value) {
					if (value?.type === "block-seq") onError(props.end, "BAD_INDENT", "All sequence items must start at the same column");
					else onError(offset, "MISSING_CHAR", "Sequence item without - indicator");
				} else {
					commentEnd = props.end;
					if (props.comment) seq.comment = props.comment;
					continue;
				}
			}
			const node = value ? composeNode(ctx, value, props, onError) : composeEmptyNode(ctx, props.end, start, null, props, onError);
			if (ctx.schema.compat) flowIndentCheck(bs.indent, value, onError);
			offset = node.range[2];
			seq.items.push(node);
		}
		seq.range = [
			bs.offset,
			offset,
			commentEnd ?? offset
		];
		return seq;
	}
	function resolveEnd(end, offset, reqSpace, onError) {
		let comment = "";
		if (end) {
			let hasSpace = false;
			let sep = "";
			for (const token of end) {
				const { source, type } = token;
				switch (type) {
					case "space":
						hasSpace = true;
						break;
					case "comment": {
						if (reqSpace && !hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
						const cb = source.substring(1) || " ";
						if (!comment) comment = cb;
						else comment += sep + cb;
						sep = "";
						break;
					}
					case "newline":
						if (comment) sep += source;
						hasSpace = true;
						break;
					default: onError(token, "UNEXPECTED_TOKEN", `Unexpected ${type} at node end`);
				}
				offset += source.length;
			}
		}
		return {
			comment,
			offset
		};
	}
	var blockMsg = "Block collections are not allowed within flow collections";
	var isBlock = (token) => token && (token.type === "block-map" || token.type === "block-seq");
	function resolveFlowCollection({ composeNode, composeEmptyNode }, ctx, fc, onError, tag) {
		const isMap = fc.start.source === "{";
		const fcName = isMap ? "flow map" : "flow sequence";
		const coll = new ((tag?.nodeClass) ?? (isMap ? YAMLMap : YAMLSeq))(ctx.schema);
		coll.flow = true;
		const atRoot = ctx.atRoot;
		if (atRoot) ctx.atRoot = false;
		if (ctx.atKey) ctx.atKey = false;
		let offset = fc.offset + fc.start.source.length;
		for (let i = 0; i < fc.items.length; ++i) {
			const collItem = fc.items[i];
			const { start, key, sep, value } = collItem;
			const props = resolveProps(start, {
				flow: fcName,
				indicator: "explicit-key-ind",
				next: key ?? sep?.[0],
				offset,
				onError,
				parentIndent: fc.indent,
				startOnNewline: false
			});
			if (!props.found) {
				if (!props.anchor && !props.tag && !sep && !value) {
					if (i === 0 && props.comma) onError(props.comma, "UNEXPECTED_TOKEN", `Unexpected , in ${fcName}`);
					else if (i < fc.items.length - 1) onError(props.start, "UNEXPECTED_TOKEN", `Unexpected empty item in ${fcName}`);
					if (props.comment) {
						if (coll.comment) coll.comment += "\n" + props.comment;
						else coll.comment = props.comment;
					}
					offset = props.end;
					continue;
				}
				if (!isMap && ctx.options.strict && containsNewline(key)) onError(key, "MULTILINE_IMPLICIT_KEY", "Implicit keys of flow sequence pairs need to be on a single line");
			}
			if (i === 0) {
				if (props.comma) onError(props.comma, "UNEXPECTED_TOKEN", `Unexpected , in ${fcName}`);
			} else {
				if (!props.comma) onError(props.start, "MISSING_CHAR", `Missing , between ${fcName} items`);
				if (props.comment) {
					let prevItemComment = "";
					loop: for (const st of start) switch (st.type) {
						case "comma":
						case "space": break;
						case "comment":
							prevItemComment = st.source.substring(1);
							break loop;
						default: break loop;
					}
					if (prevItemComment) {
						let prev = coll.items[coll.items.length - 1];
						if (isPair(prev)) prev = prev.value ?? prev.key;
						if (prev.comment) prev.comment += "\n" + prevItemComment;
						else prev.comment = prevItemComment;
						props.comment = props.comment.substring(prevItemComment.length + 1);
					}
				}
			}
			if (!isMap && !sep && !props.found) {
				const valueNode = value ? composeNode(ctx, value, props, onError) : composeEmptyNode(ctx, props.end, sep, null, props, onError);
				coll.items.push(valueNode);
				offset = valueNode.range[2];
				if (isBlock(value)) onError(valueNode.range, "BLOCK_IN_FLOW", blockMsg);
			} else {
				ctx.atKey = true;
				const keyStart = props.end;
				const keyNode = key ? composeNode(ctx, key, props, onError) : composeEmptyNode(ctx, keyStart, start, null, props, onError);
				if (isBlock(key)) onError(keyNode.range, "BLOCK_IN_FLOW", blockMsg);
				ctx.atKey = false;
				const valueProps = resolveProps(sep ?? [], {
					flow: fcName,
					indicator: "map-value-ind",
					next: value,
					offset: keyNode.range[2],
					onError,
					parentIndent: fc.indent,
					startOnNewline: false
				});
				if (valueProps.found) {
					if (!isMap && !props.found && ctx.options.strict) {
						if (sep) for (const st of sep) {
							if (st === valueProps.found) break;
							if (st.type === "newline") {
								onError(st, "MULTILINE_IMPLICIT_KEY", "Implicit keys of flow sequence pairs need to be on a single line");
								break;
							}
						}
						if (props.start < valueProps.found.offset - 1024) onError(valueProps.found, "KEY_OVER_1024_CHARS", "The : indicator must be at most 1024 chars after the start of an implicit flow sequence key");
					}
				} else if (value) {
					if ("source" in value && value.source?.[0] === ":") onError(value, "MISSING_CHAR", `Missing space after : in ${fcName}`);
					else onError(valueProps.start, "MISSING_CHAR", `Missing , or : between ${fcName} items`);
				}
				const valueNode = value ? composeNode(ctx, value, valueProps, onError) : valueProps.found ? composeEmptyNode(ctx, valueProps.end, sep, null, valueProps, onError) : null;
				if (valueNode) {
					if (isBlock(value)) onError(valueNode.range, "BLOCK_IN_FLOW", blockMsg);
				} else if (valueProps.comment) {
					if (keyNode.comment) keyNode.comment += "\n" + valueProps.comment;
					else keyNode.comment = valueProps.comment;
				}
				const pair = new Pair(keyNode, valueNode);
				if (ctx.options.keepSourceTokens) pair.srcToken = collItem;
				if (isMap) {
					const map = coll;
					if (mapIncludes(ctx, map.items, keyNode)) onError(keyStart, "DUPLICATE_KEY", "Map keys must be unique");
					map.items.push(pair);
				} else {
					const map = new YAMLMap(ctx.schema);
					map.flow = true;
					map.items.push(pair);
					const endRange = (valueNode ?? keyNode).range;
					map.range = [
						keyNode.range[0],
						endRange[1],
						endRange[2]
					];
					coll.items.push(map);
				}
				offset = valueNode ? valueNode.range[2] : valueProps.end;
			}
		}
		const expectedEnd = isMap ? "}" : "]";
		const [ce, ...ee] = fc.end;
		let cePos = offset;
		if (ce?.source === expectedEnd) cePos = ce.offset + ce.source.length;
		else {
			const name = fcName[0].toUpperCase() + fcName.substring(1);
			const msg = atRoot ? `${name} must end with a ${expectedEnd}` : `${name} in block collection must be sufficiently indented and end with a ${expectedEnd}`;
			onError(offset, atRoot ? "MISSING_CHAR" : "BAD_INDENT", msg);
			if (ce && ce.source.length !== 1) ee.unshift(ce);
		}
		if (ee.length > 0) {
			const end = resolveEnd(ee, cePos, ctx.options.strict, onError);
			if (end.comment) {
				if (coll.comment) coll.comment += "\n" + end.comment;
				else coll.comment = end.comment;
			}
			coll.range = [
				fc.offset,
				cePos,
				end.offset
			];
		} else coll.range = [
			fc.offset,
			cePos,
			cePos
		];
		return coll;
	}
	function resolveCollection(CN, ctx, token, onError, tagName, tag) {
		const coll = token.type === "block-map" ? resolveBlockMap(CN, ctx, token, onError, tag) : token.type === "block-seq" ? resolveBlockSeq(CN, ctx, token, onError, tag) : resolveFlowCollection(CN, ctx, token, onError, tag);
		const Coll = coll.constructor;
		if (tagName === "!" || tagName === Coll.tagName) {
			coll.tag = Coll.tagName;
			return coll;
		}
		if (tagName) coll.tag = tagName;
		return coll;
	}
	function composeCollection(CN, ctx, token, props, onError) {
		const tagToken = props.tag;
		const tagName = !tagToken ? null : ctx.directives.tagName(tagToken.source, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg));
		if (token.type === "block-seq") {
			const { anchor, newlineAfterProp: nl } = props;
			const lastProp = anchor && tagToken ? anchor.offset > tagToken.offset ? anchor : tagToken : anchor ?? tagToken;
			if (lastProp && (!nl || nl.offset < lastProp.offset)) onError(lastProp, "MISSING_CHAR", "Missing newline after block sequence props");
		}
		const expType = token.type === "block-map" ? "map" : token.type === "block-seq" ? "seq" : token.start.source === "{" ? "map" : "seq";
		if (!tagToken || !tagName || tagName === "!" || tagName === YAMLMap.tagName && expType === "map" || tagName === YAMLSeq.tagName && expType === "seq") return resolveCollection(CN, ctx, token, onError, tagName);
		let tag = ctx.schema.tags.find((t) => t.tag === tagName && t.collection === expType);
		if (!tag) {
			const kt = ctx.schema.knownTags[tagName];
			if (kt?.collection === expType) {
				ctx.schema.tags.push(Object.assign({}, kt, { default: false }));
				tag = kt;
			} else {
				if (kt) onError(tagToken, "BAD_COLLECTION_TYPE", `${kt.tag} used for ${expType} collection, but expects ${kt.collection ?? "scalar"}`, true);
				else onError(tagToken, "TAG_RESOLVE_FAILED", `Unresolved tag: ${tagName}`, true);
				return resolveCollection(CN, ctx, token, onError, tagName);
			}
		}
		const coll = resolveCollection(CN, ctx, token, onError, tagName, tag);
		const res = tag.resolve?.(coll, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg), ctx.options) ?? coll;
		const node = isNode$1(res) ? res : new Scalar(res);
		node.range = coll.range;
		node.tag = tagName;
		if (tag?.format) node.format = tag.format;
		return node;
	}
	function resolveBlockScalar(ctx, scalar, onError) {
		const start = scalar.offset;
		const header = parseBlockScalarHeader(scalar, ctx.options.strict, onError);
		if (!header) return {
			value: "",
			type: null,
			comment: "",
			range: [
				start,
				start,
				start
			]
		};
		const type = header.mode === ">" ? Scalar.BLOCK_FOLDED : Scalar.BLOCK_LITERAL;
		const lines = scalar.source ? splitLines(scalar.source) : [];
		let chompStart = lines.length;
		for (let i = lines.length - 1; i >= 0; --i) {
			const content = lines[i][1];
			if (content === "" || content === "\r") chompStart = i;
			else break;
		}
		if (chompStart === 0) {
			const value = header.chomp === "+" && lines.length > 0 ? "\n".repeat(Math.max(1, lines.length - 1)) : "";
			let end = start + header.length;
			if (scalar.source) end += scalar.source.length;
			return {
				value,
				type,
				comment: header.comment,
				range: [
					start,
					end,
					end
				]
			};
		}
		let trimIndent = scalar.indent + header.indent;
		let offset = scalar.offset + header.length;
		let contentStart = 0;
		for (let i = 0; i < chompStart; ++i) {
			const [indent, content] = lines[i];
			if (content === "" || content === "\r") {
				if (header.indent === 0 && indent.length > trimIndent) trimIndent = indent.length;
			} else {
				if (indent.length < trimIndent) onError(offset + indent.length, "MISSING_CHAR", "Block scalars with more-indented leading empty lines must use an explicit indentation indicator");
				if (header.indent === 0) trimIndent = indent.length;
				contentStart = i;
				if (trimIndent === 0 && !ctx.atRoot) onError(offset, "BAD_INDENT", "Block scalar values in collections must be indented");
				break;
			}
			offset += indent.length + content.length + 1;
		}
		for (let i = lines.length - 1; i >= chompStart; --i) if (lines[i][0].length > trimIndent) chompStart = i + 1;
		let value = "";
		let sep = "";
		let prevMoreIndented = false;
		for (let i = 0; i < contentStart; ++i) value += lines[i][0].slice(trimIndent) + "\n";
		for (let i = contentStart; i < chompStart; ++i) {
			let [indent, content] = lines[i];
			offset += indent.length + content.length + 1;
			const crlf = content[content.length - 1] === "\r";
			if (crlf) content = content.slice(0, -1);
			/* istanbul ignore if already caught in lexer */
			if (content && indent.length < trimIndent) {
				const message = `Block scalar lines must not be less indented than their ${header.indent ? "explicit indentation indicator" : "first line"}`;
				onError(offset - content.length - (crlf ? 2 : 1), "BAD_INDENT", message);
				indent = "";
			}
			if (type === Scalar.BLOCK_LITERAL) {
				value += sep + indent.slice(trimIndent) + content;
				sep = "\n";
			} else if (indent.length > trimIndent || content[0] === "	") {
				if (sep === " ") sep = "\n";
				else if (!prevMoreIndented && sep === "\n") sep = "\n\n";
				value += sep + indent.slice(trimIndent) + content;
				sep = "\n";
				prevMoreIndented = true;
			} else if (content === "") {
				if (sep === "\n") value += "\n";
				else sep = "\n";
			} else {
				value += sep + content;
				sep = " ";
				prevMoreIndented = false;
			}
		}
		switch (header.chomp) {
			case "-": break;
			case "+":
				for (let i = chompStart; i < lines.length; ++i) value += "\n" + lines[i][0].slice(trimIndent);
				if (value[value.length - 1] !== "\n") value += "\n";
				break;
			default: value += "\n";
		}
		const end = start + header.length + scalar.source.length;
		return {
			value,
			type,
			comment: header.comment,
			range: [
				start,
				end,
				end
			]
		};
	}
	function parseBlockScalarHeader({ offset, props }, strict, onError) {
		/* istanbul ignore if should not happen */
		if (props[0].type !== "block-scalar-header") {
			onError(props[0], "IMPOSSIBLE", "Block scalar header not found");
			return null;
		}
		const { source } = props[0];
		const mode = source[0];
		let indent = 0;
		let chomp = "";
		let error = -1;
		for (let i = 1; i < source.length; ++i) {
			const ch = source[i];
			if (!chomp && (ch === "-" || ch === "+")) chomp = ch;
			else {
				const n = Number(ch);
				if (!indent && n) indent = n;
				else if (error === -1) error = offset + i;
			}
		}
		if (error !== -1) onError(error, "UNEXPECTED_TOKEN", `Block scalar header includes extra characters: ${source}`);
		let hasSpace = false;
		let comment = "";
		let length = source.length;
		for (let i = 1; i < props.length; ++i) {
			const token = props[i];
			switch (token.type) {
				case "space": hasSpace = true;
				case "newline":
					length += token.source.length;
					break;
				case "comment":
					if (strict && !hasSpace) onError(token, "MISSING_CHAR", "Comments must be separated from other tokens by white space characters");
					length += token.source.length;
					comment = token.source.substring(1);
					break;
				case "error":
					onError(token, "UNEXPECTED_TOKEN", token.message);
					length += token.source.length;
					break;
				/* istanbul ignore next should not happen */
				default: {
					onError(token, "UNEXPECTED_TOKEN", `Unexpected token in block scalar header: ${token.type}`);
					const ts = token.source;
					if (ts && typeof ts === "string") length += ts.length;
				}
			}
		}
		return {
			mode,
			indent,
			chomp,
			comment,
			length
		};
	}
	/** @returns Array of lines split up as `[indent, content]` */
	function splitLines(source) {
		const split = source.split(/\n( *)/);
		const first = split[0];
		const m = first.match(/^( *)/);
		const lines = [m?.[1] ? [m[1], first.slice(m[1].length)] : ["", first]];
		for (let i = 1; i < split.length; i += 2) lines.push([split[i], split[i + 1]]);
		return lines;
	}
	function resolveFlowScalar(scalar, strict, onError) {
		const { offset, type, source, end } = scalar;
		let _type;
		let value;
		const _onError = (rel, code, msg) => onError(offset + rel, code, msg);
		switch (type) {
			case "scalar":
				_type = Scalar.PLAIN;
				value = plainValue(source, _onError);
				break;
			case "single-quoted-scalar":
				_type = Scalar.QUOTE_SINGLE;
				value = singleQuotedValue(source, _onError);
				break;
			case "double-quoted-scalar":
				_type = Scalar.QUOTE_DOUBLE;
				value = doubleQuotedValue(source, _onError);
				break;
			/* istanbul ignore next should not happen */
			default:
				onError(scalar, "UNEXPECTED_TOKEN", `Expected a flow scalar value, but found: ${type}`);
				return {
					value: "",
					type: null,
					comment: "",
					range: [
						offset,
						offset + source.length,
						offset + source.length
					]
				};
		}
		const valueEnd = offset + source.length;
		const re = resolveEnd(end, valueEnd, strict, onError);
		return {
			value,
			type: _type,
			comment: re.comment,
			range: [
				offset,
				valueEnd,
				re.offset
			]
		};
	}
	function plainValue(source, onError) {
		let badChar = "";
		switch (source[0]) {
			/* istanbul ignore next should not happen */
			case "	":
				badChar = "a tab character";
				break;
			case ",":
				badChar = "flow indicator character ,";
				break;
			case "%":
				badChar = "directive indicator character %";
				break;
			case "|":
			case ">":
				badChar = `block scalar indicator ${source[0]}`;
				break;
			case "@":
			case "`": badChar = `reserved character ${source[0]}`;
		}
		if (badChar) onError(0, "BAD_SCALAR_START", `Plain value cannot start with ${badChar}`);
		return unfoldLines(source);
	}
	function singleQuotedValue(source, onError) {
		if (source[source.length - 1] !== "'" || source.length === 1) onError(source.length, "MISSING_CHAR", "Missing closing 'quote");
		return unfoldLines(source.slice(1, -1)).replace(/''/g, "'");
	}
	function unfoldLines(source) {
		const line = /(.*?)\r?\n/sy;
		let match = line.exec(source);
		if (!match) return source;
		/**
		* The negative lookbehinds in these RegExps are to
		* prevent causing a polynomial search time in certain cases.
		*
		* The try-catch is for Safari < 16.4 and other old browsers:
		* https://caniuse.com/js-regexp-lookbehind
		*/
		let trimEnd, trimBoth;
		try {
			trimEnd = /* @__PURE__ */ new RegExp("(?<![ 	])[ 	]+$");
			trimBoth = /* @__PURE__ */ new RegExp("^[ 	]+|(?<![ 	])[ 	]+$", "g");
		} catch {
			trimEnd = /[ \t]+$/;
			trimBoth = /^[ \t]+|[ \t]+$/g;
		}
		let res = match[1].replace(trimEnd, "");
		let sep = " ";
		let pos = line.lastIndex;
		while (match = line.exec(source)) {
			const lm = match[1].replace(trimBoth, "");
			if (lm === "") {
				if (sep === "\n") res += sep;
				else sep = "\n";
			} else {
				res += sep + lm;
				sep = " ";
			}
			pos = line.lastIndex;
		}
		const last = /[ \t]*(.*)/sy;
		last.lastIndex = pos;
		match = last.exec(source);
		return res + sep + (match?.[1] ?? "");
	}
	function doubleQuotedValue(source, onError) {
		let res = "";
		for (let i = 1; i < source.length - 1; ++i) {
			const ch = source[i];
			if (ch === "\r" && source[i + 1] === "\n") continue;
			if (ch === "\n") {
				const { fold, offset } = foldNewline(source, i);
				res += fold;
				i = offset;
			} else if (ch === "\\") {
				let next = source[++i];
				const cc = escapeCodes[next];
				if (cc) res += cc;
				else if (next === "\n") {
					next = source[i + 1];
					while (next === " " || next === "	") next = source[++i + 1];
				} else if (next === "\r" && source[i + 1] === "\n") {
					next = source[++i + 1];
					while (next === " " || next === "	") next = source[++i + 1];
				} else if (next === "x" || next === "u" || next === "U") {
					const length = next === "x" ? 2 : next === "u" ? 4 : 8;
					res += parseCharCode(source, i + 1, length, onError);
					i += length;
				} else {
					const raw = source.substr(i - 1, 2);
					onError(i - 1, "BAD_DQ_ESCAPE", `Invalid escape sequence ${raw}`);
					res += raw;
				}
			} else if (ch === " " || ch === "	") {
				const wsStart = i;
				let next = source[i + 1];
				while (next === " " || next === "	") next = source[++i + 1];
				if (next !== "\n" && !(next === "\r" && source[i + 2] === "\n")) res += i > wsStart ? source.slice(wsStart, i + 1) : ch;
			} else res += ch;
		}
		if (source[source.length - 1] !== "\"" || source.length === 1) onError(source.length, "MISSING_CHAR", "Missing closing \"quote");
		return res;
	}
	/**
	* Fold a single newline into a space, multiple newlines to N - 1 newlines.
	* Presumes `source[offset] === '\n'`
	*/
	function foldNewline(source, offset) {
		let fold = "";
		let ch = source[offset + 1];
		while (ch === " " || ch === "	" || ch === "\n" || ch === "\r") {
			if (ch === "\r" && source[offset + 2] !== "\n") break;
			if (ch === "\n") fold += "\n";
			offset += 1;
			ch = source[offset + 1];
		}
		if (!fold) fold = " ";
		return {
			fold,
			offset
		};
	}
	var escapeCodes = {
		"0": "\0",
		a: "\x07",
		b: "\b",
		e: "\x1B",
		f: "\f",
		n: "\n",
		r: "\r",
		t: "	",
		v: "\v",
		N: "",
		_: "\xA0",
		L: "\u2028",
		P: "\u2029",
		" ": " ",
		"\"": "\"",
		"/": "/",
		"\\": "\\",
		"	": "	"
	};
	function parseCharCode(source, offset, length, onError) {
		const cc = source.substr(offset, length);
		const code = cc.length === length && /^[0-9a-fA-F]+$/.test(cc) ? parseInt(cc, 16) : NaN;
		try {
			return String.fromCodePoint(code);
		} catch {
			const raw = source.substr(offset - 2, length + 2);
			onError(offset - 2, "BAD_DQ_ESCAPE", `Invalid escape sequence ${raw}`);
			return raw;
		}
	}
	function composeScalar(ctx, token, tagToken, onError) {
		const { value, type, comment, range } = token.type === "block-scalar" ? resolveBlockScalar(ctx, token, onError) : resolveFlowScalar(token, ctx.options.strict, onError);
		const tagName = tagToken ? ctx.directives.tagName(tagToken.source, (msg) => onError(tagToken, "TAG_RESOLVE_FAILED", msg)) : null;
		let tag;
		if (ctx.options.stringKeys && ctx.atKey) tag = ctx.schema[SCALAR$1];
		else if (tagName) tag = findScalarTagByName(ctx.schema, value, tagName, tagToken, onError);
		else if (token.type === "scalar") tag = findScalarTagByTest(ctx, value, token, onError);
		else tag = ctx.schema[SCALAR$1];
		let scalar;
		try {
			const res = tag.resolve(value, (msg) => onError(tagToken ?? token, "TAG_RESOLVE_FAILED", msg), ctx.options);
			scalar = isScalar(res) ? res : new Scalar(res);
		} catch (error) {
			const msg = error instanceof Error ? error.message : String(error);
			onError(tagToken ?? token, "TAG_RESOLVE_FAILED", msg);
			scalar = new Scalar(value);
		}
		scalar.range = range;
		scalar.source = value;
		if (type) scalar.type = type;
		if (tagName) scalar.tag = tagName;
		if (tag.format) scalar.format = tag.format;
		if (comment) scalar.comment = comment;
		return scalar;
	}
	function findScalarTagByName(schema, value, tagName, tagToken, onError) {
		if (tagName === "!") return schema[SCALAR$1];
		const matchWithTest = [];
		for (const tag of schema.tags) if (!tag.collection && tag.tag === tagName) {
			if (tag.default && tag.test) matchWithTest.push(tag);
			else return tag;
		}
		for (const tag of matchWithTest) if (tag.test?.test(value)) return tag;
		const kt = schema.knownTags[tagName];
		if (kt && !kt.collection) {
			schema.tags.push(Object.assign({}, kt, {
				default: false,
				test: void 0
			}));
			return kt;
		}
		onError(tagToken, "TAG_RESOLVE_FAILED", `Unresolved tag: ${tagName}`, tagName !== "tag:yaml.org,2002:str");
		return schema[SCALAR$1];
	}
	function findScalarTagByTest({ atKey, directives, schema }, value, token, onError) {
		const tag = schema.tags.find((tag) => (tag.default === true || atKey && tag.default === "key") && tag.test?.test(value)) || schema[SCALAR$1];
		if (schema.compat) {
			const compat = schema.compat.find((tag) => tag.default && tag.test?.test(value)) ?? schema[SCALAR$1];
			if (tag.tag !== compat.tag) onError(token, "TAG_RESOLVE_FAILED", `Value may be parsed as either ${directives.tagString(tag.tag)} or ${directives.tagString(compat.tag)}`, true);
		}
		return tag;
	}
	function emptyScalarPosition(offset, before, pos) {
		if (before) {
			pos ?? (pos = before.length);
			for (let i = pos - 1; i >= 0; --i) {
				let st = before[i];
				switch (st.type) {
					case "space":
					case "comment":
					case "newline":
						offset -= st.source.length;
						continue;
				}
				st = before[++i];
				while (st?.type === "space") {
					offset += st.source.length;
					st = before[++i];
				}
				break;
			}
		}
		return offset;
	}
	var CN = {
		composeNode,
		composeEmptyNode
	};
	function composeNode(ctx, token, props, onError) {
		const atKey = ctx.atKey;
		const { spaceBefore, comment, anchor, tag } = props;
		let node;
		let isSrcToken = true;
		switch (token.type) {
			case "alias":
				node = composeAlias(ctx, token, onError);
				if (anchor || tag) onError(token, "ALIAS_PROPS", "An alias node must not specify any properties");
				break;
			case "scalar":
			case "single-quoted-scalar":
			case "double-quoted-scalar":
			case "block-scalar":
				node = composeScalar(ctx, token, tag, onError);
				if (anchor) node.anchor = anchor.source.substring(1);
				break;
			case "block-map":
			case "block-seq":
			case "flow-collection":
				try {
					node = composeCollection(CN, ctx, token, props, onError);
					if (anchor) node.anchor = anchor.source.substring(1);
				} catch (error) {
					onError(token, "RESOURCE_EXHAUSTION", error instanceof Error ? error.message : String(error));
				}
				break;
			default:
				onError(token, "UNEXPECTED_TOKEN", token.type === "error" ? token.message : `Unsupported token (type: ${token.type})`);
				isSrcToken = false;
		}
		node ?? (node = composeEmptyNode(ctx, token.offset, void 0, null, props, onError));
		if (anchor && node.anchor === "") onError(anchor, "BAD_ALIAS", "Anchor cannot be an empty string");
		if (atKey && ctx.options.stringKeys && (!isScalar(node) || typeof node.value !== "string" || node.tag && node.tag !== "tag:yaml.org,2002:str")) onError(tag ?? token, "NON_STRING_KEY", "With stringKeys, all keys must be strings");
		if (spaceBefore) node.spaceBefore = true;
		if (comment) {
			if (token.type === "scalar" && token.source === "") node.comment = comment;
			else node.commentBefore = comment;
		}
		if (ctx.options.keepSourceTokens && isSrcToken) node.srcToken = token;
		return node;
	}
	function composeEmptyNode(ctx, offset, before, pos, { spaceBefore, comment, anchor, tag, end }, onError) {
		const node = composeScalar(ctx, {
			type: "scalar",
			offset: emptyScalarPosition(offset, before, pos),
			indent: -1,
			source: ""
		}, tag, onError);
		if (anchor) {
			node.anchor = anchor.source.substring(1);
			if (node.anchor === "") onError(anchor, "BAD_ALIAS", "Anchor cannot be an empty string");
		}
		if (spaceBefore) node.spaceBefore = true;
		if (comment) {
			node.comment = comment;
			node.range[2] = end;
		}
		return node;
	}
	function composeAlias({ options }, { offset, source, end }, onError) {
		const alias = new Alias(source.substring(1));
		if (alias.source === "") onError(offset, "BAD_ALIAS", "Alias cannot be an empty string");
		if (alias.source.endsWith(":")) onError(offset + source.length - 1, "BAD_ALIAS", "Alias ending in : is ambiguous", true);
		const valueEnd = offset + source.length;
		const re = resolveEnd(end, valueEnd, options.strict, onError);
		alias.range = [
			offset,
			valueEnd,
			re.offset
		];
		if (re.comment) alias.comment = re.comment;
		return alias;
	}
	function composeDoc(options, directives, { offset, start, value, end }, onError) {
		const doc = new Document$1(void 0, Object.assign({ _directives: directives }, options));
		const ctx = {
			atKey: false,
			atRoot: true,
			directives: doc.directives,
			options: doc.options,
			schema: doc.schema
		};
		const props = resolveProps(start, {
			indicator: "doc-start",
			next: value ?? end?.[0],
			offset,
			onError,
			parentIndent: 0,
			startOnNewline: true
		});
		if (props.found) {
			doc.directives.docStart = true;
			if (value && (value.type === "block-map" || value.type === "block-seq") && !props.hasNewline) onError(props.end, "MISSING_CHAR", "Block collection cannot start on same line with directives-end marker");
		}
		doc.contents = value ? composeNode(ctx, value, props, onError) : composeEmptyNode(ctx, props.end, start, null, props, onError);
		const contentEnd = doc.contents.range[2];
		const re = resolveEnd(end, contentEnd, false, onError);
		if (re.comment) doc.comment = re.comment;
		doc.range = [
			offset,
			contentEnd,
			re.offset
		];
		return doc;
	}
	function getErrorPos(src) {
		if (typeof src === "number") return [src, src + 1];
		if (Array.isArray(src)) return src.length === 2 ? src : [src[0], src[1]];
		const { offset, source } = src;
		return [offset, offset + (typeof source === "string" ? source.length : 1)];
	}
	function parsePrelude(prelude) {
		let comment = "";
		let atComment = false;
		let afterEmptyLine = false;
		for (let i = 0; i < prelude.length; ++i) {
			const source = prelude[i];
			switch (source[0]) {
				case "#":
					comment += (comment === "" ? "" : afterEmptyLine ? "\n\n" : "\n") + (source.substring(1) || " ");
					atComment = true;
					afterEmptyLine = false;
					break;
				case "%":
					if (prelude[i + 1]?.[0] !== "#") i += 1;
					atComment = false;
					break;
				default:
					if (!atComment) afterEmptyLine = true;
					atComment = false;
			}
		}
		return {
			comment,
			afterEmptyLine
		};
	}
	/**
	* Compose a stream of CST nodes into a stream of YAML Documents.
	*
	* ```ts
	* import { Composer, Parser } from 'yaml'
	*
	* const src: string = ...
	* const tokens = new Parser().parse(src)
	* const docs = new Composer().compose(tokens)
	* ```
	*/
	var Composer = class {
		constructor(options = {}) {
			this.doc = null;
			this.atDirectives = false;
			this.prelude = [];
			this.errors = [];
			this.warnings = [];
			this.onError = (source, code, message, warning) => {
				const pos = getErrorPos(source);
				if (warning) this.warnings.push(new YAMLWarning(pos, code, message));
				else this.errors.push(new YAMLParseError(pos, code, message));
			};
			this.directives = new Directives({ version: options.version || "1.2" });
			this.options = options;
		}
		decorate(doc, afterDoc) {
			const { comment, afterEmptyLine } = parsePrelude(this.prelude);
			if (comment) {
				const dc = doc.contents;
				if (afterDoc) doc.comment = doc.comment ? `${doc.comment}\n${comment}` : comment;
				else if (afterEmptyLine || doc.directives.docStart || !dc) doc.commentBefore = comment;
				else if (isCollection(dc) && !dc.flow && dc.items.length > 0) {
					let it = dc.items[0];
					if (isPair(it)) it = it.key;
					const cb = it.commentBefore;
					it.commentBefore = cb ? `${comment}\n${cb}` : comment;
				} else {
					const cb = dc.commentBefore;
					dc.commentBefore = cb ? `${comment}\n${cb}` : comment;
				}
			}
			if (afterDoc) {
				for (let i = 0; i < this.errors.length; ++i) doc.errors.push(this.errors[i]);
				for (let i = 0; i < this.warnings.length; ++i) doc.warnings.push(this.warnings[i]);
			} else {
				doc.errors = this.errors;
				doc.warnings = this.warnings;
			}
			this.prelude = [];
			this.errors = [];
			this.warnings = [];
		}
		/**
		* Current stream status information.
		*
		* Mostly useful at the end of input for an empty stream.
		*/
		streamInfo() {
			return {
				comment: parsePrelude(this.prelude).comment,
				directives: this.directives,
				errors: this.errors,
				warnings: this.warnings
			};
		}
		/**
		* Compose tokens into documents.
		*
		* @param forceDoc - If the stream contains no document, still emit a final document including any comments and directives that would be applied to a subsequent document.
		* @param endOffset - Should be set if `forceDoc` is also set, to set the document range end and to indicate errors correctly.
		*/
		*compose(tokens, forceDoc = false, endOffset = -1) {
			for (const token of tokens) yield* this.next(token);
			yield* this.end(forceDoc, endOffset);
		}
		/** Advance the composer by one CST token. */
		*next(token) {
			switch (token.type) {
				case "directive":
					this.directives.add(token.source, (offset, message, warning) => {
						const pos = getErrorPos(token);
						pos[0] += offset;
						this.onError(pos, "BAD_DIRECTIVE", message, warning);
					});
					this.prelude.push(token.source);
					this.atDirectives = true;
					break;
				case "document": {
					const doc = composeDoc(this.options, this.directives, token, this.onError);
					if (this.atDirectives && !doc.directives.docStart) this.onError(token, "MISSING_CHAR", "Missing directives-end/doc-start indicator line");
					this.decorate(doc, false);
					if (this.doc) yield this.doc;
					this.doc = doc;
					this.atDirectives = false;
					break;
				}
				case "byte-order-mark":
				case "space": break;
				case "comment":
				case "newline":
					this.prelude.push(token.source);
					break;
				case "error": {
					const msg = token.source ? `${token.message}: ${JSON.stringify(token.source)}` : token.message;
					const error = new YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", msg);
					if (this.atDirectives || !this.doc) this.errors.push(error);
					else this.doc.errors.push(error);
					break;
				}
				case "doc-end": {
					if (!this.doc) {
						this.errors.push(new YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", "Unexpected doc-end without preceding document"));
						break;
					}
					this.doc.directives.docEnd = true;
					const end = resolveEnd(token.end, token.offset + token.source.length, this.doc.options.strict, this.onError);
					this.decorate(this.doc, true);
					if (end.comment) {
						const dc = this.doc.comment;
						this.doc.comment = dc ? `${dc}\n${end.comment}` : end.comment;
					}
					this.doc.range[2] = end.offset;
					break;
				}
				default: this.errors.push(new YAMLParseError(getErrorPos(token), "UNEXPECTED_TOKEN", `Unsupported token ${token.type}`));
			}
		}
		/**
		* Call at end of input to yield any remaining document.
		*
		* @param forceDoc - If the stream contains no document, still emit a final document including any comments and directives that would be applied to a subsequent document.
		* @param endOffset - Should be set if `forceDoc` is also set, to set the document range end and to indicate errors correctly.
		*/
		*end(forceDoc = false, endOffset = -1) {
			if (this.doc) {
				this.decorate(this.doc, true);
				yield this.doc;
				this.doc = null;
			} else if (forceDoc) {
				const doc = new Document$1(void 0, Object.assign({ _directives: this.directives }, this.options));
				if (this.atDirectives) this.onError(endOffset, "MISSING_CHAR", "Missing directives-end indicator line");
				doc.range = [
					0,
					endOffset,
					endOffset
				];
				this.decorate(doc, false);
				yield doc;
			}
		}
	};
	var BREAK = Symbol("break visit");
	var SKIP = Symbol("skip children");
	var REMOVE = Symbol("remove item");
	/**
	* Apply a visitor to a CST document or item.
	*
	* Walks through the tree (depth-first) starting from the root, calling a
	* `visitor` function with two arguments when entering each item:
	*   - `item`: The current item, which included the following members:
	*     - `start: SourceToken[]` – Source tokens before the key or value,
	*       possibly including its anchor or tag.
	*     - `key?: Token | null` – Set for pair values. May then be `null`, if
	*       the key before the `:` separator is empty.
	*     - `sep?: SourceToken[]` – Source tokens between the key and the value,
	*       which should include the `:` map value indicator if `value` is set.
	*     - `value?: Token` – The value of a sequence item, or of a map pair.
	*   - `path`: The steps from the root to the current node, as an array of
	*     `['key' | 'value', number]` tuples.
	*
	* The return value of the visitor may be used to control the traversal:
	*   - `undefined` (default): Do nothing and continue
	*   - `visit.SKIP`: Do not visit the children of this token, continue with
	*      next sibling
	*   - `visit.BREAK`: Terminate traversal completely
	*   - `visit.REMOVE`: Remove the current item, then continue with the next one
	*   - `number`: Set the index of the next step. This is useful especially if
	*     the index of the current token has changed.
	*   - `function`: Define the next visitor for this item. After the original
	*     visitor is called on item entry, next visitors are called after handling
	*     a non-empty `key` and when exiting the item.
	*/
	function visit(cst, visitor) {
		if ("type" in cst && cst.type === "document") cst = {
			start: cst.start,
			value: cst.value
		};
		_visit(Object.freeze([]), cst, visitor);
	}
	/** Terminate visit traversal completely */
	visit.BREAK = BREAK;
	/** Do not visit the children of the current item */
	visit.SKIP = SKIP;
	/** Remove the current item */
	visit.REMOVE = REMOVE;
	/** Find the item at `path` from `cst` as the root */
	visit.itemAtPath = (cst, path) => {
		let item = cst;
		for (const [field, index] of path) {
			const tok = item?.[field];
			if (tok && "items" in tok) item = tok.items[index];
			else return void 0;
		}
		return item;
	};
	/**
	* Get the immediate parent collection of the item at `path` from `cst` as the root.
	*
	* Throws an error if the collection is not found, which should never happen if the item itself exists.
	*/
	visit.parentCollection = (cst, path) => {
		const parent = visit.itemAtPath(cst, path.slice(0, -1));
		const field = path[path.length - 1][0];
		const coll = parent?.[field];
		if (coll && "items" in coll) return coll;
		throw new Error("Parent collection not found");
	};
	function _visit(path, item, visitor) {
		let ctrl = visitor(item, path);
		if (typeof ctrl === "symbol") return ctrl;
		for (const field of ["key", "value"]) {
			const token = item[field];
			if (token && "items" in token) {
				for (let i = 0; i < token.items.length; ++i) {
					const ci = _visit(Object.freeze(path.concat([[field, i]])), token.items[i], visitor);
					if (typeof ci === "number") i = ci - 1;
					else if (ci === BREAK) return BREAK;
					else if (ci === REMOVE) {
						token.items.splice(i, 1);
						i -= 1;
					}
				}
				if (typeof ctrl === "function" && field === "key") ctrl = ctrl(item, path);
			}
		}
		return typeof ctrl === "function" ? ctrl(item, path) : ctrl;
	}
	/** Identify the type of a lexer token. May return `null` for unknown tokens. */
	function tokenType(source) {
		switch (source) {
			case "﻿": return "byte-order-mark";
			case "": return "doc-mode";
			case "": return "flow-error-end";
			case "": return "scalar";
			case "---": return "doc-start";
			case "...": return "doc-end";
			case "":
			case "\n":
			case "\r\n": return "newline";
			case "-": return "seq-item-ind";
			case "?": return "explicit-key-ind";
			case ":": return "map-value-ind";
			case "{": return "flow-map-start";
			case "}": return "flow-map-end";
			case "[": return "flow-seq-start";
			case "]": return "flow-seq-end";
			case ",": return "comma";
		}
		switch (source[0]) {
			case " ":
			case "	": return "space";
			case "#": return "comment";
			case "%": return "directive-line";
			case "*": return "alias";
			case "&": return "anchor";
			case "!": return "tag";
			case "'": return "single-quoted-scalar";
			case "\"": return "double-quoted-scalar";
			case "|":
			case ">": return "block-scalar-header";
		}
		return null;
	}
	function isEmpty(ch) {
		switch (ch) {
			case void 0:
			case " ":
			case "\n":
			case "\r":
			case "	": return true;
			default: return false;
		}
	}
	var hexDigits = /* @__PURE__ */ new Set("0123456789ABCDEFabcdef");
	var tagChars = /* @__PURE__ */ new Set("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-#;/?:@&=+$_.!~*'()");
	var flowIndicatorChars = /* @__PURE__ */ new Set(",[]{}");
	var invalidAnchorChars = /* @__PURE__ */ new Set(" ,[]{}\n\r	");
	var isNotAnchorChar = (ch) => !ch || invalidAnchorChars.has(ch);
	/**
	* Splits an input string into lexical tokens, i.e. smaller strings that are
	* easily identifiable by `tokens.tokenType()`.
	*
	* Lexing starts always in a "stream" context. Incomplete input may be buffered
	* until a complete token can be emitted.
	*
	* In addition to slices of the original input, the following control characters
	* may also be emitted:
	*
	* - `\x02` (Start of Text): A document starts with the next token
	* - `\x18` (Cancel): Unexpected end of flow-mode (indicates an error)
	* - `\x1f` (Unit Separator): Next token is a scalar value
	* - `\u{FEFF}` (Byte order mark): Emitted separately outside documents
	*/
	var Lexer = class {
		constructor() {
			/**
			* Flag indicating whether the end of the current buffer marks the end of
			* all input
			*/
			this.atEnd = false;
			/**
			* Explicit indent set in block scalar header, as an offset from the current
			* minimum indent, so e.g. set to 1 from a header `|2+`. Set to -1 if not
			* explicitly set.
			*/
			this.blockScalarIndent = -1;
			/**
			* Block scalars that include a + (keep) chomping indicator in their header
			* include trailing empty lines, which are otherwise excluded from the
			* scalar's contents.
			*/
			this.blockScalarKeep = false;
			/** Current input */
			this.buffer = "";
			/**
			* Flag noting whether the map value indicator : can immediately follow this
			* node within a flow context.
			*/
			this.flowKey = false;
			/** Count of surrounding flow collection levels. */
			this.flowLevel = 0;
			/**
			* Minimum level of indentation required for next lines to be parsed as a
			* part of the current scalar value.
			*/
			this.indentNext = 0;
			/** Indentation level of the current line. */
			this.indentValue = 0;
			/** Position of the next \n character. */
			this.lineEndPos = null;
			/** Stores the state of the lexer if reaching the end of incpomplete input */
			this.next = null;
			/** A pointer to `buffer`; the current position of the lexer. */
			this.pos = 0;
		}
		/**
		* Generate YAML tokens from the `source` string. If `incomplete`,
		* a part of the last line may be left as a buffer for the next call.
		*
		* @returns A generator of lexical tokens
		*/
		*lex(source, incomplete = false) {
			if (source) {
				if (typeof source !== "string") throw TypeError("source is not a string");
				this.buffer = this.buffer ? this.buffer + source : source;
				this.lineEndPos = null;
			}
			this.atEnd = !incomplete;
			let next = this.next ?? "stream";
			while (next && (incomplete || this.hasChars(1))) next = yield* this.parseNext(next);
		}
		atLineEnd() {
			let i = this.pos;
			let ch = this.buffer[i];
			while (ch === " " || ch === "	") ch = this.buffer[++i];
			if (!ch || ch === "#" || ch === "\n") return true;
			if (ch === "\r") return this.buffer[i + 1] === "\n";
			return false;
		}
		charAt(n) {
			return this.buffer[this.pos + n];
		}
		continueScalar(offset) {
			let ch = this.buffer[offset];
			if (this.indentNext > 0) {
				let indent = 0;
				while (ch === " ") ch = this.buffer[++indent + offset];
				if (ch === "\r") {
					const next = this.buffer[indent + offset + 1];
					if (next === "\n" || !next && !this.atEnd) return offset + indent + 1;
				}
				return ch === "\n" || indent >= this.indentNext || !ch && !this.atEnd ? offset + indent : -1;
			}
			if (ch === "-" || ch === ".") {
				const dt = this.buffer.substr(offset, 3);
				if ((dt === "---" || dt === "...") && isEmpty(this.buffer[offset + 3])) return -1;
			}
			return offset;
		}
		getLine() {
			let end = this.lineEndPos;
			if (typeof end !== "number" || end !== -1 && end < this.pos) {
				end = this.buffer.indexOf("\n", this.pos);
				this.lineEndPos = end;
			}
			if (end === -1) return this.atEnd ? this.buffer.substring(this.pos) : null;
			if (this.buffer[end - 1] === "\r") end -= 1;
			return this.buffer.substring(this.pos, end);
		}
		hasChars(n) {
			return this.pos + n <= this.buffer.length;
		}
		setNext(state) {
			this.buffer = this.buffer.substring(this.pos);
			this.pos = 0;
			this.lineEndPos = null;
			this.next = state;
			return null;
		}
		peek(n) {
			return this.buffer.substr(this.pos, n);
		}
		*parseNext(next) {
			switch (next) {
				case "stream": return yield* this.parseStream();
				case "line-start": return yield* this.parseLineStart();
				case "block-start": return yield* this.parseBlockStart();
				case "doc": return yield* this.parseDocument();
				case "flow": return yield* this.parseFlowCollection();
				case "quoted-scalar": return yield* this.parseQuotedScalar();
				case "block-scalar": return yield* this.parseBlockScalar();
				case "plain-scalar": return yield* this.parsePlainScalar();
			}
		}
		*parseStream() {
			let line = this.getLine();
			if (line === null) return this.setNext("stream");
			if (line[0] === "﻿") {
				yield* this.pushCount(1);
				line = line.substring(1);
			}
			if (line[0] === "%") {
				let dirEnd = line.length;
				let cs = line.indexOf("#");
				while (cs !== -1) {
					const ch = line[cs - 1];
					if (ch === " " || ch === "	") {
						dirEnd = cs - 1;
						break;
					} else cs = line.indexOf("#", cs + 1);
				}
				while (true) {
					const ch = line[dirEnd - 1];
					if (ch === " " || ch === "	") dirEnd -= 1;
					else break;
				}
				const n = (yield* this.pushCount(dirEnd)) + (yield* this.pushSpaces(true));
				yield* this.pushCount(line.length - n);
				this.pushNewline();
				return "stream";
			}
			if (this.atLineEnd()) {
				const sp = yield* this.pushSpaces(true);
				yield* this.pushCount(line.length - sp);
				yield* this.pushNewline();
				return "stream";
			}
			yield "";
			return yield* this.parseLineStart();
		}
		*parseLineStart() {
			const ch = this.charAt(0);
			if (!ch && !this.atEnd) return this.setNext("line-start");
			if (ch === "-" || ch === ".") {
				if (!this.atEnd && !this.hasChars(4)) return this.setNext("line-start");
				const s = this.peek(3);
				if ((s === "---" || s === "...") && isEmpty(this.charAt(3))) {
					yield* this.pushCount(3);
					this.indentValue = 0;
					this.indentNext = 0;
					return s === "---" ? "doc" : "stream";
				}
			}
			this.indentValue = yield* this.pushSpaces(false);
			if (this.indentNext > this.indentValue && !isEmpty(this.charAt(1))) this.indentNext = this.indentValue;
			return yield* this.parseBlockStart();
		}
		*parseBlockStart() {
			const [ch0, ch1] = this.peek(2);
			if (!ch1 && !this.atEnd) return this.setNext("block-start");
			if ((ch0 === "-" || ch0 === "?" || ch0 === ":") && isEmpty(ch1)) {
				const n = (yield* this.pushCount(1)) + (yield* this.pushSpaces(true));
				this.indentNext = this.indentValue + 1;
				this.indentValue += n;
				return "block-start";
			}
			return "doc";
		}
		*parseDocument() {
			yield* this.pushSpaces(true);
			const line = this.getLine();
			if (line === null) return this.setNext("doc");
			let n = yield* this.pushIndicators();
			switch (line[n]) {
				case "#": yield* this.pushCount(line.length - n);
				case void 0:
					yield* this.pushNewline();
					return yield* this.parseLineStart();
				case "{":
				case "[":
					yield* this.pushCount(1);
					this.flowKey = false;
					this.flowLevel = 1;
					return "flow";
				case "}":
				case "]":
					yield* this.pushCount(1);
					return "doc";
				case "*":
					yield* this.pushUntil(isNotAnchorChar);
					return "doc";
				case "\"":
				case "'": return yield* this.parseQuotedScalar();
				case "|":
				case ">":
					n += yield* this.parseBlockScalarHeader();
					n += yield* this.pushSpaces(true);
					yield* this.pushCount(line.length - n);
					yield* this.pushNewline();
					return yield* this.parseBlockScalar();
				default: return yield* this.parsePlainScalar();
			}
		}
		*parseFlowCollection() {
			let nl, sp;
			let indent = -1;
			do {
				nl = yield* this.pushNewline();
				if (nl > 0) {
					sp = yield* this.pushSpaces(false);
					this.indentValue = indent = sp;
				} else sp = 0;
				sp += yield* this.pushSpaces(true);
			} while (nl + sp > 0);
			const line = this.getLine();
			if (line === null) return this.setNext("flow");
			if (indent !== -1 && indent < this.indentNext && line[0] !== "#" || indent === 0 && (line.startsWith("---") || line.startsWith("...")) && isEmpty(line[3])) {
				if (!(indent === this.indentNext - 1 && this.flowLevel === 1 && (line[0] === "]" || line[0] === "}"))) {
					this.flowLevel = 0;
					yield "";
					return yield* this.parseLineStart();
				}
			}
			let n = 0;
			while (line[n] === ",") {
				n += yield* this.pushCount(1);
				n += yield* this.pushSpaces(true);
				this.flowKey = false;
			}
			n += yield* this.pushIndicators();
			switch (line[n]) {
				case void 0: return "flow";
				case "#":
					yield* this.pushCount(line.length - n);
					return "flow";
				case "{":
				case "[":
					yield* this.pushCount(1);
					this.flowKey = false;
					this.flowLevel += 1;
					return "flow";
				case "}":
				case "]":
					yield* this.pushCount(1);
					this.flowKey = true;
					this.flowLevel -= 1;
					return this.flowLevel ? "flow" : "doc";
				case "*":
					yield* this.pushUntil(isNotAnchorChar);
					return "flow";
				case "\"":
				case "'":
					this.flowKey = true;
					return yield* this.parseQuotedScalar();
				case ":": {
					const next = this.charAt(1);
					if (this.flowKey || isEmpty(next) || next === ",") {
						this.flowKey = false;
						yield* this.pushCount(1);
						yield* this.pushSpaces(true);
						return "flow";
					}
				}
				default:
					this.flowKey = false;
					return yield* this.parsePlainScalar();
			}
		}
		*parseQuotedScalar() {
			const quote = this.charAt(0);
			let end = this.buffer.indexOf(quote, this.pos + 1);
			if (quote === "'") while (end !== -1 && this.buffer[end + 1] === "'") end = this.buffer.indexOf("'", end + 2);
			else while (end !== -1) {
				let n = 0;
				while (this.buffer[end - 1 - n] === "\\") n += 1;
				if (n % 2 === 0) break;
				end = this.buffer.indexOf("\"", end + 1);
			}
			const qb = this.buffer.substring(0, end);
			let nl = qb.indexOf("\n", this.pos);
			if (nl !== -1) {
				while (nl !== -1) {
					const cs = this.continueScalar(nl + 1);
					if (cs === -1) break;
					nl = qb.indexOf("\n", cs);
				}
				if (nl !== -1) end = nl - (qb[nl - 1] === "\r" ? 2 : 1);
			}
			if (end === -1) {
				if (!this.atEnd) return this.setNext("quoted-scalar");
				end = this.buffer.length;
			}
			yield* this.pushToIndex(end + 1, false);
			return this.flowLevel ? "flow" : "doc";
		}
		*parseBlockScalarHeader() {
			this.blockScalarIndent = -1;
			this.blockScalarKeep = false;
			let i = this.pos;
			while (true) {
				const ch = this.buffer[++i];
				if (ch === "+") this.blockScalarKeep = true;
				else if (ch > "0" && ch <= "9") this.blockScalarIndent = Number(ch) - 1;
				else if (ch !== "-") break;
			}
			return yield* this.pushUntil((ch) => isEmpty(ch) || ch === "#");
		}
		*parseBlockScalar() {
			let nl = this.pos - 1;
			let indent = 0;
			let ch;
			loop: for (let i = this.pos; ch = this.buffer[i]; ++i) switch (ch) {
				case " ":
					indent += 1;
					break;
				case "\n":
					nl = i;
					indent = 0;
					break;
				case "\r": {
					const next = this.buffer[i + 1];
					if (!next && !this.atEnd) return this.setNext("block-scalar");
					if (next === "\n") break;
				}
				default: break loop;
			}
			if (!ch && !this.atEnd) return this.setNext("block-scalar");
			if (indent >= this.indentNext) {
				if (this.blockScalarIndent === -1) this.indentNext = indent;
				else this.indentNext = this.blockScalarIndent + (this.indentNext === 0 ? 1 : this.indentNext);
				do {
					const cs = this.continueScalar(nl + 1);
					if (cs === -1) break;
					nl = this.buffer.indexOf("\n", cs);
				} while (nl !== -1);
				if (nl === -1) {
					if (!this.atEnd) return this.setNext("block-scalar");
					nl = this.buffer.length;
				}
			}
			let i = nl + 1;
			ch = this.buffer[i];
			while (ch === " ") ch = this.buffer[++i];
			if (ch === "	") {
				while (ch === "	" || ch === " " || ch === "\r" || ch === "\n") ch = this.buffer[++i];
				nl = i - 1;
			} else if (!this.blockScalarKeep) do {
				let i = nl - 1;
				let ch = this.buffer[i];
				if (ch === "\r") ch = this.buffer[--i];
				const lastChar = i;
				while (ch === " ") ch = this.buffer[--i];
				if (ch === "\n" && i >= this.pos && i + 1 + indent > lastChar) nl = i;
				else break;
			} while (true);
			yield "";
			yield* this.pushToIndex(nl + 1, true);
			return yield* this.parseLineStart();
		}
		*parsePlainScalar() {
			const inFlow = this.flowLevel > 0;
			let end = this.pos - 1;
			let i = this.pos - 1;
			let ch;
			while (ch = this.buffer[++i]) if (ch === ":") {
				const next = this.buffer[i + 1];
				if (isEmpty(next) || inFlow && flowIndicatorChars.has(next)) break;
				end = i;
			} else if (isEmpty(ch)) {
				let next = this.buffer[i + 1];
				if (ch === "\r") {
					if (next === "\n") {
						i += 1;
						ch = "\n";
						next = this.buffer[i + 1];
					} else end = i;
				}
				if (next === "#" || inFlow && flowIndicatorChars.has(next)) break;
				if (ch === "\n") {
					const cs = this.continueScalar(i + 1);
					if (cs === -1) break;
					i = Math.max(i, cs - 2);
				}
			} else {
				if (inFlow && flowIndicatorChars.has(ch)) break;
				end = i;
			}
			if (!ch && !this.atEnd) return this.setNext("plain-scalar");
			yield "";
			yield* this.pushToIndex(end + 1, true);
			return inFlow ? "flow" : "doc";
		}
		*pushCount(n) {
			if (n > 0) {
				yield this.buffer.substr(this.pos, n);
				this.pos += n;
				return n;
			}
			return 0;
		}
		*pushToIndex(i, allowEmpty) {
			const s = this.buffer.slice(this.pos, i);
			if (s) {
				yield s;
				this.pos += s.length;
				return s.length;
			} else if (allowEmpty) yield "";
			return 0;
		}
		*pushIndicators() {
			let n = 0;
			loop: while (true) {
				switch (this.charAt(0)) {
					case "!":
						n += yield* this.pushTag();
						n += yield* this.pushSpaces(true);
						continue loop;
					case "&":
						n += yield* this.pushUntil(isNotAnchorChar);
						n += yield* this.pushSpaces(true);
						continue loop;
					case "-":
					case "?":
					case ":": {
						const inFlow = this.flowLevel > 0;
						const ch1 = this.charAt(1);
						if (isEmpty(ch1) || inFlow && flowIndicatorChars.has(ch1)) {
							if (!inFlow) this.indentNext = this.indentValue + 1;
							else if (this.flowKey) this.flowKey = false;
							n += yield* this.pushCount(1);
							n += yield* this.pushSpaces(true);
							continue loop;
						}
					}
				}
				break loop;
			}
			return n;
		}
		*pushTag() {
			if (this.charAt(1) === "<") {
				let i = this.pos + 2;
				let ch = this.buffer[i];
				while (!isEmpty(ch) && ch !== ">") ch = this.buffer[++i];
				return yield* this.pushToIndex(ch === ">" ? i + 1 : i, false);
			} else {
				let i = this.pos + 1;
				let ch = this.buffer[i];
				while (ch) if (tagChars.has(ch)) ch = this.buffer[++i];
				else if (ch === "%" && hexDigits.has(this.buffer[i + 1]) && hexDigits.has(this.buffer[i + 2])) ch = this.buffer[i += 3];
				else break;
				return yield* this.pushToIndex(i, false);
			}
		}
		*pushNewline() {
			const ch = this.buffer[this.pos];
			if (ch === "\n") return yield* this.pushCount(1);
			else if (ch === "\r" && this.charAt(1) === "\n") return yield* this.pushCount(2);
			else return 0;
		}
		*pushSpaces(allowTabs) {
			let i = this.pos - 1;
			let ch;
			do
				ch = this.buffer[++i];
			while (ch === " " || allowTabs && ch === "	");
			const n = i - this.pos;
			if (n > 0) {
				yield this.buffer.substr(this.pos, n);
				this.pos = i;
			}
			return n;
		}
		*pushUntil(test) {
			let i = this.pos;
			let ch = this.buffer[i];
			while (!test(ch)) ch = this.buffer[++i];
			return yield* this.pushToIndex(i, false);
		}
	};
	/**
	* Tracks newlines during parsing in order to provide an efficient API for
	* determining the one-indexed `{ line, col }` position for any offset
	* within the input.
	*/
	var LineCounter = class {
		constructor() {
			this.lineStarts = [];
			/**
			* Should be called in ascending order. Otherwise, call
			* `lineCounter.lineStarts.sort()` before calling `linePos()`.
			*/
			this.addNewLine = (offset) => this.lineStarts.push(offset);
			/**
			* Performs a binary search and returns the 1-indexed { line, col }
			* position of `offset`. If `line === 0`, `addNewLine` has never been
			* called or `offset` is before the first known newline.
			*/
			this.linePos = (offset) => {
				let low = 0;
				let high = this.lineStarts.length;
				while (low < high) {
					const mid = low + high >> 1;
					if (this.lineStarts[mid] < offset) low = mid + 1;
					else high = mid;
				}
				if (this.lineStarts[low] === offset) return {
					line: low + 1,
					col: 1
				};
				if (low === 0) return {
					line: 0,
					col: offset
				};
				const start = this.lineStarts[low - 1];
				return {
					line: low,
					col: offset - start + 1
				};
			};
		}
	};
	function includesToken(list, type) {
		for (let i = 0; i < list.length; ++i) if (list[i].type === type) return true;
		return false;
	}
	function findNonEmptyIndex(list) {
		for (let i = 0; i < list.length; ++i) switch (list[i].type) {
			case "space":
			case "comment":
			case "newline": break;
			default: return i;
		}
		return -1;
	}
	function isFlowToken(token) {
		switch (token?.type) {
			case "alias":
			case "scalar":
			case "single-quoted-scalar":
			case "double-quoted-scalar":
			case "flow-collection": return true;
			default: return false;
		}
	}
	function getPrevProps(parent) {
		switch (parent.type) {
			case "document": return parent.start;
			case "block-map": {
				const it = parent.items[parent.items.length - 1];
				return it.sep ?? it.start;
			}
			case "block-seq": return parent.items[parent.items.length - 1].start;
			/* istanbul ignore next should not happen */
			default: return [];
		}
	}
	/** Note: May modify input array */
	function getFirstKeyStartProps(prev) {
		if (prev.length === 0) return [];
		let i = prev.length;
		loop: while (--i >= 0) switch (prev[i].type) {
			case "doc-start":
			case "explicit-key-ind":
			case "map-value-ind":
			case "seq-item-ind":
			case "newline": break loop;
		}
		while (prev[++i]?.type === "space");
		return prev.splice(i, prev.length);
	}
	function arrayPushArray(target, source) {
		if (source.length < 1e5) Array.prototype.push.apply(target, source);
		else for (let i = 0; i < source.length; ++i) target.push(source[i]);
	}
	function fixFlowSeqItems(fc) {
		if (fc.start.type === "flow-seq-start") {
			for (const it of fc.items) if (it.sep && !it.value && !includesToken(it.start, "explicit-key-ind") && !includesToken(it.sep, "map-value-ind")) {
				if (it.key) it.value = it.key;
				delete it.key;
				if (isFlowToken(it.value)) {
					if (it.value.end) arrayPushArray(it.value.end, it.sep);
					else it.value.end = it.sep;
				} else arrayPushArray(it.start, it.sep);
				delete it.sep;
			}
		}
	}
	/**
	* A YAML concrete syntax tree (CST) parser
	*
	* ```ts
	* const src: string = ...
	* for (const token of new Parser().parse(src)) {
	*   // token: Token
	* }
	* ```
	*
	* To use the parser with a user-provided lexer:
	*
	* ```ts
	* function* parse(source: string, lexer: Lexer) {
	*   const parser = new Parser()
	*   for (const lexeme of lexer.lex(source))
	*     yield* parser.next(lexeme)
	*   yield* parser.end()
	* }
	*
	* const src: string = ...
	* const lexer = new Lexer()
	* for (const token of parse(src, lexer)) {
	*   // token: Token
	* }
	* ```
	*/
	var Parser = class {
		/**
		* @param onNewLine - If defined, called separately with the start position of
		*   each new line (in `parse()`, including the start of input).
		*/
		constructor(onNewLine) {
			/** If true, space and sequence indicators count as indentation */
			this.atNewLine = true;
			/** If true, next token is a scalar value */
			this.atScalar = false;
			/** Current indentation level */
			this.indent = 0;
			/** Current offset since the start of parsing */
			this.offset = 0;
			/** On the same line with a block map key */
			this.onKeyLine = false;
			/** Top indicates the node that's currently being built */
			this.stack = [];
			/** The source of the current token, set in parse() */
			this.source = "";
			/** The type of the current token, set in parse() */
			this.type = "";
			this.lexer = new Lexer();
			this.onNewLine = onNewLine;
		}
		/**
		* Parse `source` as a YAML stream.
		* If `incomplete`, a part of the last line may be left as a buffer for the next call.
		*
		* Errors are not thrown, but yielded as `{ type: 'error', message }` tokens.
		*
		* @returns A generator of tokens representing each directive, document, and other structure.
		*/
		*parse(source, incomplete = false) {
			if (this.onNewLine && this.offset === 0) this.onNewLine(0);
			for (const lexeme of this.lexer.lex(source, incomplete)) yield* this.next(lexeme);
			if (!incomplete) yield* this.end();
		}
		/**
		* Advance the parser by the `source` of one lexical token.
		*/
		*next(source) {
			this.source = source;
			if (this.atScalar) {
				this.atScalar = false;
				yield* this.step();
				this.offset += source.length;
				return;
			}
			const type = tokenType(source);
			if (!type) {
				const message = `Not a YAML token: ${source}`;
				yield* this.pop({
					type: "error",
					offset: this.offset,
					message,
					source
				});
				this.offset += source.length;
			} else if (type === "scalar") {
				this.atNewLine = false;
				this.atScalar = true;
				this.type = "scalar";
			} else {
				this.type = type;
				yield* this.step();
				switch (type) {
					case "newline":
						this.atNewLine = true;
						this.indent = 0;
						if (this.onNewLine) this.onNewLine(this.offset + source.length);
						break;
					case "space":
						if (this.atNewLine && source[0] === " ") this.indent += source.length;
						break;
					case "explicit-key-ind":
					case "map-value-ind":
					case "seq-item-ind":
						if (this.atNewLine) this.indent += source.length;
						break;
					case "doc-mode":
					case "flow-error-end": return;
					default: this.atNewLine = false;
				}
				this.offset += source.length;
			}
		}
		/** Call at end of input to push out any remaining constructions */
		*end() {
			while (this.stack.length > 0) yield* this.pop();
		}
		get sourceToken() {
			return {
				type: this.type,
				offset: this.offset,
				indent: this.indent,
				source: this.source
			};
		}
		*step() {
			const top = this.peek(1);
			if (this.type === "doc-end" && top?.type !== "doc-end") {
				while (this.stack.length > 0) yield* this.pop();
				this.stack.push({
					type: "doc-end",
					offset: this.offset,
					source: this.source
				});
				return;
			}
			if (!top) return yield* this.stream();
			switch (top.type) {
				case "document": return yield* this.document(top);
				case "alias":
				case "scalar":
				case "single-quoted-scalar":
				case "double-quoted-scalar": return yield* this.scalar(top);
				case "block-scalar": return yield* this.blockScalar(top);
				case "block-map": return yield* this.blockMap(top);
				case "block-seq": return yield* this.blockSequence(top);
				case "flow-collection": return yield* this.flowCollection(top);
				case "doc-end": return yield* this.documentEnd(top);
			}
			/* istanbul ignore next should not happen */
			yield* this.pop();
		}
		peek(n) {
			return this.stack[this.stack.length - n];
		}
		*pop(error) {
			const token = error ?? this.stack.pop();
			/* istanbul ignore if should not happen */
			if (!token) yield {
				type: "error",
				offset: this.offset,
				source: "",
				message: "Tried to pop an empty stack"
			};
			else if (this.stack.length === 0) yield token;
			else {
				const top = this.peek(1);
				if (token.type === "block-scalar") token.indent = "indent" in top ? top.indent : 0;
				else if (token.type === "flow-collection" && top.type === "document") token.indent = 0;
				if (token.type === "flow-collection") fixFlowSeqItems(token);
				switch (top.type) {
					case "document":
						top.value = token;
						break;
					case "block-scalar":
						top.props.push(token);
						break;
					case "block-map": {
						const it = top.items[top.items.length - 1];
						if (it.value) {
							top.items.push({
								start: [],
								key: token,
								sep: []
							});
							this.onKeyLine = true;
							return;
						} else if (it.sep) it.value = token;
						else {
							Object.assign(it, {
								key: token,
								sep: []
							});
							this.onKeyLine = !it.explicitKey;
							return;
						}
						break;
					}
					case "block-seq": {
						const it = top.items[top.items.length - 1];
						if (it.value) top.items.push({
							start: [],
							value: token
						});
						else it.value = token;
						break;
					}
					case "flow-collection": {
						const it = top.items[top.items.length - 1];
						if (!it || it.value) top.items.push({
							start: [],
							key: token,
							sep: []
						});
						else if (it.sep) it.value = token;
						else Object.assign(it, {
							key: token,
							sep: []
						});
						return;
					}
					/* istanbul ignore next should not happen */
					default:
						yield* this.pop();
						yield* this.pop(token);
				}
				if ((top.type === "document" || top.type === "block-map" || top.type === "block-seq") && (token.type === "block-map" || token.type === "block-seq")) {
					const last = token.items[token.items.length - 1];
					if (last && !last.sep && !last.value && last.start.length > 0 && findNonEmptyIndex(last.start) === -1 && (token.indent === 0 || last.start.every((st) => st.type !== "comment" || st.indent < token.indent))) {
						if (top.type === "document") top.end = last.start;
						else top.items.push({ start: last.start });
						token.items.splice(-1, 1);
					}
				}
			}
		}
		*stream() {
			switch (this.type) {
				case "directive-line":
					yield {
						type: "directive",
						offset: this.offset,
						source: this.source
					};
					return;
				case "byte-order-mark":
				case "space":
				case "comment":
				case "newline":
					yield this.sourceToken;
					return;
				case "doc-mode":
				case "doc-start": {
					const doc = {
						type: "document",
						offset: this.offset,
						start: []
					};
					if (this.type === "doc-start") doc.start.push(this.sourceToken);
					this.stack.push(doc);
					return;
				}
			}
			yield {
				type: "error",
				offset: this.offset,
				message: `Unexpected ${this.type} token in YAML stream`,
				source: this.source
			};
		}
		*document(doc) {
			if (doc.value) return yield* this.lineEnd(doc);
			switch (this.type) {
				case "doc-start":
					if (findNonEmptyIndex(doc.start) !== -1) {
						yield* this.pop();
						yield* this.step();
					} else doc.start.push(this.sourceToken);
					return;
				case "anchor":
				case "tag":
				case "space":
				case "comment":
				case "newline":
					doc.start.push(this.sourceToken);
					return;
			}
			const bv = this.startBlockValue(doc);
			if (bv) this.stack.push(bv);
			else yield {
				type: "error",
				offset: this.offset,
				message: `Unexpected ${this.type} token in YAML document`,
				source: this.source
			};
		}
		*scalar(scalar) {
			if (this.type === "map-value-ind") {
				const start = getFirstKeyStartProps(getPrevProps(this.peek(2)));
				let sep;
				if (scalar.end) {
					sep = scalar.end;
					sep.push(this.sourceToken);
					delete scalar.end;
				} else sep = [this.sourceToken];
				const map = {
					type: "block-map",
					offset: scalar.offset,
					indent: scalar.indent,
					items: [{
						start,
						key: scalar,
						sep
					}]
				};
				this.onKeyLine = true;
				this.stack[this.stack.length - 1] = map;
			} else yield* this.lineEnd(scalar);
		}
		*blockScalar(scalar) {
			switch (this.type) {
				case "space":
				case "comment":
				case "newline":
					scalar.props.push(this.sourceToken);
					return;
				case "scalar":
					scalar.source = this.source;
					this.atNewLine = true;
					this.indent = 0;
					if (this.onNewLine) {
						let nl = this.source.indexOf("\n") + 1;
						while (nl !== 0) {
							this.onNewLine(this.offset + nl);
							nl = this.source.indexOf("\n", nl) + 1;
						}
					}
					yield* this.pop();
					break;
				/* istanbul ignore next should not happen */
				default:
					yield* this.pop();
					yield* this.step();
			}
		}
		*blockMap(map) {
			const it = map.items[map.items.length - 1];
			switch (this.type) {
				case "newline":
					this.onKeyLine = false;
					if (it.value) {
						const end = "end" in it.value ? it.value.end : void 0;
						if ((Array.isArray(end) ? end[end.length - 1] : void 0)?.type === "comment") end?.push(this.sourceToken);
						else map.items.push({ start: [this.sourceToken] });
					} else if (it.sep) it.sep.push(this.sourceToken);
					else it.start.push(this.sourceToken);
					return;
				case "space":
				case "comment":
					if (it.value) map.items.push({ start: [this.sourceToken] });
					else if (it.sep) it.sep.push(this.sourceToken);
					else {
						if (this.atIndentedComment(it.start, map.indent)) {
							const end = map.items[map.items.length - 2]?.value?.end;
							if (Array.isArray(end)) {
								arrayPushArray(end, it.start);
								end.push(this.sourceToken);
								map.items.pop();
								return;
							}
						}
						it.start.push(this.sourceToken);
					}
					return;
			}
			if (this.indent >= map.indent) {
				const atMapIndent = !this.onKeyLine && this.indent === map.indent;
				const atNextItem = atMapIndent && (it.sep || it.explicitKey) && this.type !== "seq-item-ind";
				let start = [];
				if (atNextItem && it.sep && !it.value) {
					const nl = [];
					for (let i = 0; i < it.sep.length; ++i) {
						const st = it.sep[i];
						switch (st.type) {
							case "newline":
								nl.push(i);
								break;
							case "space": break;
							case "comment":
								if (st.indent > map.indent) nl.length = 0;
								break;
							default: nl.length = 0;
						}
					}
					if (nl.length >= 2) start = it.sep.splice(nl[1]);
				}
				switch (this.type) {
					case "anchor":
					case "tag":
						if (atNextItem || it.value) {
							start.push(this.sourceToken);
							map.items.push({ start });
							this.onKeyLine = true;
						} else if (it.sep) it.sep.push(this.sourceToken);
						else it.start.push(this.sourceToken);
						return;
					case "explicit-key-ind":
						if (!it.sep && !it.explicitKey) {
							it.start.push(this.sourceToken);
							it.explicitKey = true;
						} else if (atNextItem || it.value) {
							start.push(this.sourceToken);
							map.items.push({
								start,
								explicitKey: true
							});
						} else this.stack.push({
							type: "block-map",
							offset: this.offset,
							indent: this.indent,
							items: [{
								start: [this.sourceToken],
								explicitKey: true
							}]
						});
						this.onKeyLine = true;
						return;
					case "map-value-ind":
						if (it.explicitKey) {
							if (!it.sep) {
								if (includesToken(it.start, "newline")) Object.assign(it, {
									key: null,
									sep: [this.sourceToken]
								});
								else {
									const start = getFirstKeyStartProps(it.start);
									this.stack.push({
										type: "block-map",
										offset: this.offset,
										indent: this.indent,
										items: [{
											start,
											key: null,
											sep: [this.sourceToken]
										}]
									});
								}
							} else if (it.value) map.items.push({
								start: [],
								key: null,
								sep: [this.sourceToken]
							});
							else if (includesToken(it.sep, "map-value-ind")) this.stack.push({
								type: "block-map",
								offset: this.offset,
								indent: this.indent,
								items: [{
									start,
									key: null,
									sep: [this.sourceToken]
								}]
							});
							else if (isFlowToken(it.key) && !includesToken(it.sep, "newline")) {
								const start = getFirstKeyStartProps(it.start);
								const key = it.key;
								const sep = it.sep;
								sep.push(this.sourceToken);
								delete it.key;
								delete it.sep;
								this.stack.push({
									type: "block-map",
									offset: this.offset,
									indent: this.indent,
									items: [{
										start,
										key,
										sep
									}]
								});
							} else if (start.length > 0) it.sep = it.sep.concat(start, this.sourceToken);
							else it.sep.push(this.sourceToken);
						} else if (!it.sep) Object.assign(it, {
							key: null,
							sep: [this.sourceToken]
						});
						else if (it.value || atNextItem) map.items.push({
							start,
							key: null,
							sep: [this.sourceToken]
						});
						else if (includesToken(it.sep, "map-value-ind")) this.stack.push({
							type: "block-map",
							offset: this.offset,
							indent: this.indent,
							items: [{
								start: [],
								key: null,
								sep: [this.sourceToken]
							}]
						});
						else it.sep.push(this.sourceToken);
						this.onKeyLine = true;
						return;
					case "alias":
					case "scalar":
					case "single-quoted-scalar":
					case "double-quoted-scalar": {
						const fs = this.flowScalar(this.type);
						if (atNextItem || it.value) {
							map.items.push({
								start,
								key: fs,
								sep: []
							});
							this.onKeyLine = true;
						} else if (it.sep) this.stack.push(fs);
						else {
							Object.assign(it, {
								key: fs,
								sep: []
							});
							this.onKeyLine = true;
						}
						return;
					}
					default: {
						const bv = this.startBlockValue(map);
						if (bv) {
							if (bv.type === "block-seq") {
								if (!it.explicitKey && it.sep && !includesToken(it.sep, "newline")) {
									yield* this.pop({
										type: "error",
										offset: this.offset,
										message: "Unexpected block-seq-ind on same line with key",
										source: this.source
									});
									return;
								}
							} else if (atMapIndent) map.items.push({ start });
							this.stack.push(bv);
							return;
						}
					}
				}
			}
			yield* this.pop();
			yield* this.step();
		}
		*blockSequence(seq) {
			const it = seq.items[seq.items.length - 1];
			switch (this.type) {
				case "newline":
					if (it.value) {
						const end = "end" in it.value ? it.value.end : void 0;
						if ((Array.isArray(end) ? end[end.length - 1] : void 0)?.type === "comment") end?.push(this.sourceToken);
						else seq.items.push({ start: [this.sourceToken] });
					} else it.start.push(this.sourceToken);
					return;
				case "space":
				case "comment":
					if (it.value) seq.items.push({ start: [this.sourceToken] });
					else {
						if (this.atIndentedComment(it.start, seq.indent)) {
							const end = seq.items[seq.items.length - 2]?.value?.end;
							if (Array.isArray(end)) {
								arrayPushArray(end, it.start);
								end.push(this.sourceToken);
								seq.items.pop();
								return;
							}
						}
						it.start.push(this.sourceToken);
					}
					return;
				case "anchor":
				case "tag":
					if (it.value || this.indent <= seq.indent) break;
					it.start.push(this.sourceToken);
					return;
				case "seq-item-ind":
					if (this.indent !== seq.indent) break;
					if (it.value || includesToken(it.start, "seq-item-ind")) seq.items.push({ start: [this.sourceToken] });
					else it.start.push(this.sourceToken);
					return;
			}
			if (this.indent > seq.indent) {
				const bv = this.startBlockValue(seq);
				if (bv) {
					this.stack.push(bv);
					return;
				}
			}
			yield* this.pop();
			yield* this.step();
		}
		*flowCollection(fc) {
			const it = fc.items[fc.items.length - 1];
			if (this.type === "flow-error-end") {
				let top;
				do {
					yield* this.pop();
					top = this.peek(1);
				} while (top?.type === "flow-collection");
			} else if (fc.end.length === 0) {
				switch (this.type) {
					case "comma":
					case "explicit-key-ind":
						if (!it || it.sep) fc.items.push({ start: [this.sourceToken] });
						else it.start.push(this.sourceToken);
						return;
					case "map-value-ind":
						if (!it || it.value) fc.items.push({
							start: [],
							key: null,
							sep: [this.sourceToken]
						});
						else if (it.sep) it.sep.push(this.sourceToken);
						else Object.assign(it, {
							key: null,
							sep: [this.sourceToken]
						});
						return;
					case "space":
					case "comment":
					case "newline":
					case "anchor":
					case "tag":
						if (!it || it.value) fc.items.push({ start: [this.sourceToken] });
						else if (it.sep) it.sep.push(this.sourceToken);
						else it.start.push(this.sourceToken);
						return;
					case "alias":
					case "scalar":
					case "single-quoted-scalar":
					case "double-quoted-scalar": {
						const fs = this.flowScalar(this.type);
						if (!it || it.value) fc.items.push({
							start: [],
							key: fs,
							sep: []
						});
						else if (it.sep) this.stack.push(fs);
						else Object.assign(it, {
							key: fs,
							sep: []
						});
						return;
					}
					case "flow-map-end":
					case "flow-seq-end":
						fc.end.push(this.sourceToken);
						return;
				}
				const bv = this.startBlockValue(fc);
				/* istanbul ignore else should not happen */
				if (bv) this.stack.push(bv);
				else {
					yield* this.pop();
					yield* this.step();
				}
			} else {
				const parent = this.peek(2);
				if (parent.type === "block-map" && (this.type === "map-value-ind" && parent.indent === fc.indent || this.type === "newline" && !parent.items[parent.items.length - 1].sep)) {
					yield* this.pop();
					yield* this.step();
				} else if (this.type === "map-value-ind" && parent.type !== "flow-collection") {
					const start = getFirstKeyStartProps(getPrevProps(parent));
					fixFlowSeqItems(fc);
					const sep = fc.end.splice(1, fc.end.length);
					sep.push(this.sourceToken);
					const map = {
						type: "block-map",
						offset: fc.offset,
						indent: fc.indent,
						items: [{
							start,
							key: fc,
							sep
						}]
					};
					this.onKeyLine = true;
					this.stack[this.stack.length - 1] = map;
				} else yield* this.lineEnd(fc);
			}
		}
		flowScalar(type) {
			if (this.onNewLine) {
				let nl = this.source.indexOf("\n") + 1;
				while (nl !== 0) {
					this.onNewLine(this.offset + nl);
					nl = this.source.indexOf("\n", nl) + 1;
				}
			}
			return {
				type,
				offset: this.offset,
				indent: this.indent,
				source: this.source
			};
		}
		startBlockValue(parent) {
			switch (this.type) {
				case "alias":
				case "scalar":
				case "single-quoted-scalar":
				case "double-quoted-scalar": return this.flowScalar(this.type);
				case "block-scalar-header": return {
					type: "block-scalar",
					offset: this.offset,
					indent: this.indent,
					props: [this.sourceToken],
					source: ""
				};
				case "flow-map-start":
				case "flow-seq-start": return {
					type: "flow-collection",
					offset: this.offset,
					indent: this.indent,
					start: this.sourceToken,
					items: [],
					end: []
				};
				case "seq-item-ind": return {
					type: "block-seq",
					offset: this.offset,
					indent: this.indent,
					items: [{ start: [this.sourceToken] }]
				};
				case "explicit-key-ind": {
					this.onKeyLine = true;
					const start = getFirstKeyStartProps(getPrevProps(parent));
					start.push(this.sourceToken);
					return {
						type: "block-map",
						offset: this.offset,
						indent: this.indent,
						items: [{
							start,
							explicitKey: true
						}]
					};
				}
				case "map-value-ind": {
					this.onKeyLine = true;
					const start = getFirstKeyStartProps(getPrevProps(parent));
					return {
						type: "block-map",
						offset: this.offset,
						indent: this.indent,
						items: [{
							start,
							key: null,
							sep: [this.sourceToken]
						}]
					};
				}
			}
			return null;
		}
		atIndentedComment(start, indent) {
			if (this.type !== "comment") return false;
			if (this.indent <= indent) return false;
			return start.every((st) => st.type === "newline" || st.type === "space");
		}
		*documentEnd(docEnd) {
			if (this.type !== "doc-mode") {
				if (docEnd.end) docEnd.end.push(this.sourceToken);
				else docEnd.end = [this.sourceToken];
				if (this.type === "newline") yield* this.pop();
			}
		}
		*lineEnd(token) {
			switch (this.type) {
				case "comma":
				case "doc-start":
				case "doc-end":
				case "flow-seq-end":
				case "flow-map-end":
				case "map-value-ind":
					yield* this.pop();
					yield* this.step();
					break;
				case "newline": this.onKeyLine = false;
				default:
					if (token.end) token.end.push(this.sourceToken);
					else token.end = [this.sourceToken];
					if (this.type === "newline") yield* this.pop();
			}
		}
	};
	function parseOptions(options) {
		const prettyErrors = options.prettyErrors !== false;
		return {
			lineCounter: options.lineCounter || prettyErrors && new LineCounter() || null,
			prettyErrors
		};
	}
	/** Parse an input string into a single YAML.Document */
	function parseDocument(source, options = {}) {
		const { lineCounter, prettyErrors } = parseOptions(options);
		const parser = new Parser(lineCounter?.addNewLine);
		const composer = new Composer(options);
		let doc = null;
		for (const _doc of composer.compose(parser.parse(source), true, source.length)) if (!doc) doc = _doc;
		else if (doc.options.logLevel !== "silent") {
			doc.errors.push(new YAMLParseError(_doc.range.slice(0, 2), "MULTIPLE_DOCS", "Source contains multiple documents; please use YAML.parseAllDocuments()"));
			break;
		}
		if (prettyErrors && lineCounter) {
			doc.errors.forEach(prettifyError(source, lineCounter));
			doc.warnings.forEach(prettifyError(source, lineCounter));
		}
		return doc;
	}
	function parse(src, reviver, options) {
		let _reviver = void 0;
		if (typeof reviver === "function") _reviver = reviver;
		else if (options === void 0 && reviver && typeof reviver === "object") options = reviver;
		const doc = parseDocument(src, options);
		if (!doc) return null;
		doc.warnings.forEach((warning) => warn(doc.options.logLevel, warning));
		if (doc.errors.length > 0) {
			if (doc.options.logLevel !== "silent") throw doc.errors[0];
			else doc.errors = [];
		}
		return doc.toJS(Object.assign({ reviver: _reviver }, options));
	}
	var schema = parse(`
required:
  - name
  - colocated
`);
	async function getSite() {
		const site = await getSite$1();
		for (const field of schema.required) if (!site[field]) throw new Error(`site remote omitted ${field}`);
		return site;
	}
	function _page$58($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			/**
			* What this app is for.
			*
			* Every capability skgo has is a page in this app, and a visitor who has
			* never seen skgo should be able to start here and reach all of them. Each
			* entry says what to look for once it is open, because a page that renders
			* is not the same as a page that proves anything: "the list is there" and
			* "the list was answered by a Go function while Go rendered the document"
			* look identical until somebody says which one they meant.
			*/
			const capabilities = [
				{
					name: "Pages rendered in the Go process",
					href: "/items/42",
					look: "The name \"Widget 42\" is in the bytes Go sent, put there by a Go function called while the document was being built."
				},
				{
					name: "Route parameters",
					href: "/items/7",
					look: "The id in the URL is the argument the Go function was given: /items/7 is a different widget from /items/42."
				},
				{
					name: "Rest parameters",
					href: "/docs/guide/getting-started",
					look: "The title is the path after /docs, and the depth is how many segments it had."
				},
				{
					name: "Remote functions: queries, commands and forms",
					href: "/todos",
					look: "Adding, completing and deleting a todo are Go functions; the count above the list is a live query that follows along."
				},
				{
					name: "Refreshing a query after a command",
					href: "/todos/gate",
					look: "The gate opens and closes from Go, and the query behind it re-runs because the command said which query to refresh."
				},
				{
					name: "One query, one answer per argument",
					href: "/todos/pair",
					look: "Both panels call the same Go function, told apart only by the id they pass. Retitle p1 and ask for p1 back: p2 does not move, because each argument has its own cached answer."
				},
				{
					name: "A query that goes on answering",
					href: "/live",
					look: "The number was already in the document before any script ran, and Go pushes every later value down the same open stream."
				},
				{
					name: "Many calls answered by one",
					href: "/batch",
					look: "Four rows each asked for one symbol on their own. One Go function was called once holding all four, and every row says how many were in the call that answered it."
				},
				{
					name: "Server loads, section-wide",
					href: "/account",
					look: "The layout and the page were both loaded in Go. Move between Overview and Orders: the serial does not change, because kit did not re-run the layout."
				},
				{
					name: "Typed locals and layout request events",
					href: "/event-layout/red/first/one?route=untracked",
					look: "The Go hook supplies the signed-in user. Sibling and unread item changes reuse the layout; a tracked section, item or route change advances its serial."
				},
				{
					name: "Universal load over Go invoice data",
					href: "/invoices?overdue=1",
					look: "Go supplies invoices; the authored universal load filters, sorts and totals them during the first render and again in Kit navigation."
				},
				{
					name: "A nested error page",
					href: "/account/statement",
					look: "A load refuses with 402, and the error page renders inside the account layout rather than replacing it."
				},
				{
					name: "Values a load promises but does not have yet",
					href: "/stream",
					look: "The total is in the document; the rows arrive after it, in the order they settle rather than the order they were asked for."
				},
				{
					name: "Custom types that keep their methods",
					href: "/pricing",
					look: "Every price is a method call on a Go type. The wire carried whole cents — \"$7.50\" only exists because Money.format() ran."
				},
				{
					name: "A form that works with JavaScript switched off",
					href: "/contact",
					look: "The form posts to Go and the page comes back with the answer in it. Disable JavaScript and it still works."
				},
				{
					name: "Page form actions",
					href: "/actions",
					look: "Save a profile, reject an invalid edit, and follow a redirect. Try every outcome with and without JavaScript."
				},
				{
					name: "HTTP endpoints written in Go",
					href: "/api",
					look: "Kit 3 identifies the route through $app/manifest and sends its new HTTP QUERY method to an ordinary net/http function."
				},
				{
					name: "Links and asset URLs worked out while the page renders",
					href: "/render-paths",
					look: "Every href on the page was computed during the render by kit's own $app/paths, and match() asked Go's route table which route /items/77 belongs to."
				},
				{
					name: "A page with no client-side JavaScript",
					href: "/plain",
					look: "csr = false, so the document carries no script at all. Everything on it had to be rendered on the server to be there."
				},
				{
					name: "A page rendered only in the browser",
					href: "/spa",
					look: "ssr = false, so Go answers with kit's SPA fallback and the page appears once the client has booted."
				},
				{
					name: "An empty list is still a list",
					href: "/empty",
					look: "Go's nil slice reaches the browser as [], so the page renders an empty list instead of failing on null."
				},
				{
					name: "What the renderer writes reaches Go's log",
					href: "/console",
					look: "The page calls console.error while it renders, and the line comes out of the Go process."
				},
				{
					name: "A load that refuses",
					href: "/error/expected",
					look: "The load throws 418 and the visitor gets an error page at that status, rendered before it was sent."
				},
				{
					name: "A failure the page catches itself",
					href: "/error/boundary",
					look: "The query refuses, the boundary beside it renders what it said, and the rest of the page is untouched — at the status the error carried."
				},
				{
					name: "An error that is a bug says nothing about itself",
					href: "/error/unexpected",
					look: "The Go load fails with a database connection string in its message. The visitor gets the message and support id chosen by handleError, and no part of the original leaves the process."
				},
				{
					name: "A failure no error page can catch",
					href: "/?boom=root-layout",
					reload: true,
					look: "The root layout's load refuses. Nothing above it can render an error page, so kit's static error.html answers instead — no app, no script, nothing that tries again."
				}
			];
			$$renderer.push(`<header class="showcase-hero svelte-1uha8ag"><p class="eyebrow svelte-1uha8ag">The working reference</p> <h1 data-testid="title">Home</h1> `);
			Greeting($$renderer, { name: "skgo" });
			$$renderer.push(`<!----> <p class="intro svelte-1uha8ag">Every server answer in this ordinary SvelteKit app comes from Go. Pick a
    capability below, try it, then follow the colocated <code>.go</code> and <code>.svelte</code> files.</p> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="site-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let site;
						var promises = $$renderer.run([async () => site = (await save(getSite()))()]);
						$$renderer.push(`<div class="provenance svelte-1uha8ag"><p data-testid="site-name">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(site.name)));
						$$renderer.push(`</p> <p data-testid="colocated">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(site.colocated)));
						$$renderer.push(`</p></div>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(`</header> <div class="capability-heading svelte-1uha8ag"><div><p class="eyebrow svelte-1uha8ag">The complete tour</p> <h2 class="svelte-1uha8ag">What this app demonstrates</h2></div> <a href="https://github.com/tylergannon/skgo/tree/main/example" target="_blank" class="svelte-1uha8ag">Read the source ↗</a></div> <ul data-testid="capabilities" class="svelte-1uha8ag"><!--[-->`);
			const each_array = ensure_array_like(capabilities);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let capability = each_array[$$index];
				$$renderer.push(`<li data-testid="capability" class="svelte-1uha8ag"><a data-testid="capability-link"${attr("href", capability.href)}${attr("data-sveltekit-reload", capability.reload ? "" : void 0)} class="svelte-1uha8ag">${escape_html$1(capability.name)}</a> <p data-testid="capability-look" class="svelte-1uha8ag">${escape_html$1(capability.look)}</p></li>`);
			}
			$$renderer.push(`<!--]--></ul>`);
		});
	}
	function _page$57($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Pricing</h1> <p data-testid="featured">${escape_html$1(data.featured.name)} — ${escape_html$1(data.featured.price.format())}</p> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="spotlight-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let spotlight;
						var promises = $$renderer.run([async () => spotlight = (await save(getSpotlight()))()]);
						$$renderer.push(`<p data-testid="spotlight">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(spotlight.name)));
						$$renderer.push(` — `);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(spotlight.price.format())));
						$$renderer.push(`</p>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="plans-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="plans-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` <button data-testid="ask">Ask Go about $20.00</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	async function load$6({ data }) {
		return {
			...data,
			remoteReceipt: await buildReceipt("atlas")
		};
	}
	function _page$56($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">About</h1> <p>skgo serves this SvelteKit app from a single Go binary.</p> <p data-testid="prerender-parent">${escape_html$1(data.parentDeployment)}</p> <p data-testid="prerender-price">${escape_html$1(data.price.format())}</p> `);
			await_block($$renderer, data.later, () => {}, (value) => {
				$$renderer.push(`<p data-testid="prerender-deferred">${escape_html$1(value)}</p>`);
			});
			$$renderer.push(`<!--]--> <p data-testid="prerender-remote">${escape_html$1(data.remoteReceipt)}</p>`);
		});
	}
	function _page$55($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Overview</h1> <p data-testid="parent-user">The layout loaded ${escape_html$1(data.parentUser)}</p>`);
		});
	}
	function _page$54($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Orders</h1> <p data-testid="order-total">${escape_html$1(data.orderTotal)} orders</p> `);
			await_block($$renderer, data.orders, () => {
				$$renderer.push(`<p data-testid="orders-pending">fetching the orders…</p>`);
			}, (orders) => {
				$$renderer.push(`<ul data-testid="orders"><!--[-->`);
				const each_array = ensure_array_like(orders);
				for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
					let order = each_array[$$index];
					$$renderer.push(`<li data-testid="order">${escape_html$1(order.item)}</li>`);
				}
				$$renderer.push(`<!--]--></ul>`);
			});
			$$renderer.push(`<!--]-->`);
		});
	}
	function _page$53($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Load reruns</h1> <p data-testid="rerun-slug">${escape_html$1(data.slug)}</p> <p data-testid="rerun-filter">${escape_html$1(data.filter)}</p> <p data-testid="rerun-serial">${escape_html$1(data.serial)}</p> <p data-testid="rerun-local-count">${escape_html$1(0)}</p> <button data-testid="rerun-local-increment">Increment local count</button> <button data-testid="rerun-refresh-all">Refresh all loads</button> <button data-testid="rerun-invalidate-predicate">Invalidate matching dependency</button> <nav><a href="/account/reruns/two?x=1">Slug two</a> <a href="/account/reruns/two?x=2">Tracked x</a> <a href="/account/reruns/two?x=2&amp;y=1">Untracked y</a></nav>`);
		});
	}
	function _page$52($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Statement</h1> <p data-testid="balance">${escape_html$1(data.balance)}</p>`);
		});
	}
	function _page$51($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			let endpointAnswer = "No endpoint POST yet.";
			const remoteFields = sendRemoteNote.fields;
			let rejected = derived(() => form && "emailError" in form ? form : null);
			head("11zl00k", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Actions | skgo example</title>`);
				});
			});
			$$renderer.push(`<div class="actions-page svelte-11zl00k"><header class="svelte-11zl00k"><p class="eyebrow svelte-11zl00k">Page form actions</p> <h1 data-testid="title" class="svelte-11zl00k">Actions</h1> <p class="svelte-11zl00k">Save a profile, reject an invalid edit, archive without a receipt, sign
      in, or try an error. The same editor works as a native form or with Kit
      enhancement.</p> <p class="svelte-11zl00k"><a href="/actions/default">Default action without a load</a> · <a href="/actions/profiles/ada">Two profile editor</a> · <a href="/actions/cross/send">Submit to another page</a> · <a href="/actions/options/no-client">No client JavaScript</a> · <a href="/actions/options/no-ssr">Client rendering</a> · <a href="#shared-route">Shared route and remote form</a></p></header> <div class="columns svelte-11zl00k"><section class="card svelte-11zl00k" aria-labelledby="editor-title"><h2 id="editor-title" class="svelte-11zl00k">Profile editor</h2> <p>Choose a native document POST or Kit enhancement. Try each outcome.</p> <noscript><p data-testid="actions-noscript">JavaScript is disabled. Native actions still work.</p></noscript> <!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<form${attr("data-testid", mode + "-save-form")} method="POST" action="?/save" novalidate="" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native save" : "Enhanced save")}</h3> <label class="svelte-11zl00k">Name <input name="name"${attr("value", rejected() ? rejected().name : data.profile.name)} required="" class="svelte-11zl00k"/></label> <label class="svelte-11zl00k">Email <input name="email" type="email"${attr("value", rejected() ? rejected().email : data.profile.email)} required="" class="svelte-11zl00k"/></label> `);
				if (rejected()) $$renderer.push(`<!--[0--><p class="field-error svelte-11zl00k"${attr("data-testid", mode + "-email-error")}>${escape_html$1(rejected().emailError)}</p>`);
				else $$renderer.push("<!--[-1-->");
				$$renderer.push(`<!--]--> <label class="svelte-11zl00k">Biography <textarea name="biography" required="" class="svelte-11zl00k">`);
				const $$body = escape_html$1(rejected() ? rejected().biography : data.profile.biography);
				if ($$body) $$renderer.push(`${$$body}`);
				$$renderer.push(`</textarea></label> <button type="submit" class="svelte-11zl00k">Save profile</button></form> <form${attr("data-testid", mode + "-upload-form")} method="POST" action="?/upload" enctype="multipart/form-data" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native upload" : "Enhanced upload")}</h3> <label class="svelte-11zl00k">Poem <input type="file" name="upload" required="" class="svelte-11zl00k"/></label> <label class="check svelte-11zl00k"><input type="checkbox" name="interest" value="math" class="svelte-11zl00k"/> Math</label> <label class="check svelte-11zl00k"><input type="checkbox" name="interest" value="computing" class="svelte-11zl00k"/> Computing</label> <button type="submit" name="uploadButton" value="send-poem" class="svelte-11zl00k">Send poem</button></form> <form${attr("data-testid", mode + "-encoding-form")} method="POST" action="?/inspect" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native submitter encoding" : "Enhanced submitter encoding")}</h3> <input type="hidden" name="interest" value="math" class="svelte-11zl00k"/> <input type="hidden" name="interest" value="computing" class="svelte-11zl00k"/> <button type="submit" name="encodingButton" value="standard" class="svelte-11zl00k">Send urlencoded</button> <button type="submit" name="encodingButton" value="multipart" formenctype="multipart/form-data" class="svelte-11zl00k">Send multipart override</button></form> <form${attr("data-testid", mode + "-archive-form")} method="POST" action="?/archive" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native archive" : "Enhanced archive")}</h3> <button type="submit" class="svelte-11zl00k">Archive profile</button></form> <form${attr("data-testid", mode + "-signin-form")} method="POST" action="?/signIn" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native sign-in" : "Enhanced sign-in")}</h3> <input type="hidden" name="username" value="ada" class="svelte-11zl00k"/> <button type="submit" class="svelte-11zl00k">Sign in as ada</button></form> <form${attr("data-testid", mode + "-forbidden-form")} method="POST" action="?/forbidden" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native permission error" : "Enhanced permission error")}</h3> <button type="submit" class="svelte-11zl00k">Attempt forbidden edit</button></form> <form${attr("data-testid", mode + "-unavailable-form")} method="POST" action="?/unavailable" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native unexpected error" : "Enhanced unexpected error")}</h3> <button type="submit" class="svelte-11zl00k">Trigger demo service failure</button></form> <form${attr("data-testid", mode + "-choice-form")} method="POST" novalidate="" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native chosen action" : "Enhanced chosen action")}</h3> <label class="svelte-11zl00k">Name <input name="name"${attr("value", data.profile.name)} required="" class="svelte-11zl00k"/></label> <label class="svelte-11zl00k">Email <input name="email" type="email"${attr("value", data.profile.email)} required="" class="svelte-11zl00k"/></label> <label class="svelte-11zl00k">Biography <textarea name="biography" required="" class="svelte-11zl00k">`);
				const $$body_1 = escape_html$1(data.profile.biography);
				if ($$body_1) $$renderer.push(`${$$body_1}`);
				$$renderer.push(`</textarea></label> <button type="submit" name="choice" value="save" formaction="?/save" class="svelte-11zl00k">Save selected profile</button> <button type="submit" name="choice" value="archive" formaction="?/archive" class="svelte-11zl00k">Archive selected profile</button></form> <form${attr("data-testid", mode + "-cookie-form")} method="POST" action="?/remember" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native immediate cookie" : "Enhanced immediate cookie")}</h3> <button type="submit" class="svelte-11zl00k">Remember violet-42</button></form> <form${attr("data-testid", mode + "-hook-redirect-form")} method="POST" action="?hook=sign-in&amp;/save" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native hook redirect" : "Enhanced hook redirect")}</h3> <input type="hidden" name="name" value="Grace Hopper" class="svelte-11zl00k"/><input type="hidden" name="email" value="grace@example.test" class="svelte-11zl00k"/><input type="hidden" name="biography" value="Compiler pioneer" class="svelte-11zl00k"/> <button type="submit" class="svelte-11zl00k">Try guarded sign-in</button></form> <form${attr("data-testid", mode + "-hook-error-form")} method="POST" action="?hook=forbidden&amp;/save" class="svelte-11zl00k"><h3 class="svelte-11zl00k">${escape_html$1(mode === "native" ? "Native hook refusal" : "Enhanced hook refusal")}</h3> <input type="hidden" name="name" value="Grace Hopper" class="svelte-11zl00k"/><input type="hidden" name="email" value="grace@example.test" class="svelte-11zl00k"/><input type="hidden" name="biography" value="Compiler pioneer" class="svelte-11zl00k"/> <button type="submit" class="svelte-11zl00k">Try guarded edit</button></form>`);
			}
			$$renderer.push(`<!--]--></section> <div class="results svelte-11zl00k"><section class="card svelte-11zl00k" aria-labelledby="saved-title"><h2 id="saved-title" class="svelte-11zl00k">Saved profile</h2> <p data-testid="saved-name" class="svelte-11zl00k">${escape_html$1(data.profile.name)}</p> <p data-testid="saved-email" class="svelte-11zl00k">${escape_html$1(data.profile.email)}</p> <p data-testid="saved-biography" class="svelte-11zl00k">${escape_html$1(data.profile.biography)}</p> <p data-testid="saved-state" class="svelte-11zl00k">${escape_html$1(data.profile.state)}</p></section> <section class="card svelte-11zl00k" aria-labelledby="receipt-title"><h2 id="receipt-title" class="svelte-11zl00k">Action receipt</h2> <p data-testid="page-status" class="svelte-11zl00k">Page status ${escape_html$1(page.status)}</p> `);
			if (form && "receipt" in form && form.price) $$renderer.push(`<!--[0--><p data-testid="action-status" class="svelte-11zl00k">Status ${escape_html$1(page.status)}</p> <p data-testid="action-receipt" class="svelte-11zl00k">${escape_html$1(form.receipt)}</p> <p data-testid="action-money" class="svelte-11zl00k">${escape_html$1(form.price.format())}</p>`);
			else if (form && "filename" in form) $$renderer.push(`<!--[1--><p data-testid="upload-name" class="svelte-11zl00k">${escape_html$1(form.filename)}</p> <p data-testid="upload-bytes" class="svelte-11zl00k">${escape_html$1(form.bytes)} bytes</p> <p data-testid="upload-hash" class="svelte-11zl00k">SHA-256 prefix ${escape_html$1(form.sha256)}</p> <p data-testid="upload-interests" class="svelte-11zl00k">${escape_html$1(form.interests?.join(", "))}</p> <p data-testid="upload-submitter" class="svelte-11zl00k">uploadButton=${escape_html$1(form.submitter)}</p> <p data-testid="action-money" class="svelte-11zl00k">${escape_html$1(form.price?.format())}</p>`);
			else if (form && "encoding" in form) $$renderer.push(`<!--[2--><p data-testid="encoding-interests" class="svelte-11zl00k">${escape_html$1(form.interests?.join(", "))}</p> <p data-testid="encoding-submitter" class="svelte-11zl00k">encodingButton=${escape_html$1(form.submitter)}</p> <p data-testid="encoding-type" class="svelte-11zl00k">${escape_html$1(form.encoding)}</p> <p data-testid="action-money" class="svelte-11zl00k">${escape_html$1(form.price?.format())}</p>`);
			else if (form && "receipt" in form) $$renderer.push(`<!--[3--><p data-testid="action-receipt" class="svelte-11zl00k">${escape_html$1(form.receipt)}</p>`);
			else if (rejected()) $$renderer.push(`<!--[4--><p data-testid="action-status" class="svelte-11zl00k">Status ${escape_html$1(page.status)}</p> <p data-testid="validation-summary" class="svelte-11zl00k">Edit rejected; correct the email and try again.</p>`);
			else $$renderer.push(`<!--[-1--><p data-testid="no-receipt" class="svelte-11zl00k">No action receipt is present.</p>`);
			$$renderer.push(`<!--]--></section> <section class="card svelte-11zl00k" aria-labelledby="cookie-title"><h2 id="cookie-title" class="svelte-11zl00k">Action cookie read by Go load</h2> <p data-testid="action-cookie-value" class="svelte-11zl00k">${escape_html$1(data.actionCookie || "No action cookie yet")}</p></section> <section class="card svelte-11zl00k" id="shared-route" aria-labelledby="shared-title"><h2 id="shared-title" class="svelte-11zl00k">Shared route and remote form</h2> <p class="svelte-11zl00k">The classic save above, the endpoint POST, and this remote form have
          separate Go handlers.</p> <button data-testid="endpoint-post" type="button" class="svelte-11zl00k">Ask the sibling endpoint</button> <p data-testid="endpoint-answer" class="svelte-11zl00k">${escape_html$1(endpointAnswer)}</p> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><p class="svelte-11zl00k">Classic result: ${escape_html$1(form.receipt)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <noscript><p data-testid="coexist-noscript" class="svelte-11zl00k">JavaScript is disabled. Both native forms still work.</p></noscript> <form data-testid="native-remote-note" method="POST"${attr("action", sendRemoteNote.action)} class="svelte-11zl00k"><h3 class="svelte-11zl00k">Native remote form</h3> <label class="svelte-11zl00k">Name <input${attributes({
				...remoteFields.name.as("text"),
				value: "Grace Hopper"
			}, "svelte-11zl00k", void 0, void 0, 4)}/></label> <button type="submit" class="svelte-11zl00k">Send native remote note</button></form> <form${attributes({
				"data-testid": "enhanced-remote-note",
				...sendRemoteNote.enhance(async (submission) => {
					await submission.submit();
				})
			}, "svelte-11zl00k")}><h3 class="svelte-11zl00k">Enhanced remote form</h3> <label class="svelte-11zl00k">Name <input${attributes({
				...remoteFields.name.as("text"),
				value: "Grace Hopper"
			}, "svelte-11zl00k", void 0, void 0, 4)}/></label> <button type="submit"${attr("disabled", sendRemoteNote.pending > 0, true)} class="svelte-11zl00k">Send enhanced remote note</button></form> `);
			if (sendRemoteNote.result) $$renderer.push(`<!--[0--><p data-testid="remote-note-receipt" class="svelte-11zl00k">${escape_html$1(sendRemoteNote.result.message)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section> <button data-testid="client-interaction" type="button" class="svelte-11zl00k">Check client interaction</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></div></div></div>`);
		});
	}
	function _page$50($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			let rejected = derived(() => form && "emailError" in form ? form : null);
			head("z1qaf1", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Cross-page result | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-z1qaf1"><p><a href="/actions/cross/send">Return to cross-page forms</a> · <a href="/actions">Actions</a></p> <h1 data-testid="cross-destination-title">Cross-page destination</h1> <p data-testid="cross-source-query">Source ${escape_html$1(page.url.searchParams.get("source") ?? "none")}</p> <noscript><p data-testid="cross-noscript">JavaScript is disabled. The destination is rendered by Go.</p></noscript> <div class="columns svelte-z1qaf1"><section aria-label="Destination action result" class="svelte-z1qaf1"><h2>Destination result</h2> <p data-testid="cross-status" class="svelte-z1qaf1">Page status ${escape_html$1(page.status)}</p> `);
			if (form && "receipt" in form && form.price) $$renderer.push(`<!--[0--><p data-testid="cross-receipt" class="svelte-z1qaf1">${escape_html$1(form.receipt)}</p> <p data-testid="cross-money" class="svelte-z1qaf1">${escape_html$1(form.price.format())}</p>`);
			else if (rejected()) $$renderer.push(`<!--[1--><p data-testid="cross-validation" class="svelte-z1qaf1">Edit rejected; correct the email and try again.</p> <p data-testid="cross-email-error" class="svelte-z1qaf1">${escape_html$1(rejected().emailError)}</p>`);
			else $$renderer.push(`<!--[-1--><p class="svelte-z1qaf1">No action result on this GET.</p>`);
			$$renderer.push(`<!--]--> `);
			if (rejected()) $$renderer.push(`<!--[0--><form method="POST" action="?/save&amp;source=invite" novalidate="" class="svelte-z1qaf1"><h3 class="svelte-z1qaf1">Correct the rejected edit</h3> <label class="svelte-z1qaf1">Name <input name="name"${attr("value", rejected().name)} class="svelte-z1qaf1"/></label> <label class="svelte-z1qaf1">Email <input name="email"${attr("value", rejected().email)} class="svelte-z1qaf1"/></label> <label class="svelte-z1qaf1">Biography <input name="biography"${attr("value", rejected().biography)} class="svelte-z1qaf1"/></label> <button type="submit" class="svelte-z1qaf1">Try the edit again</button></form>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section> <section aria-label="Saved destination profile" class="svelte-z1qaf1"><h2>Saved destination profile</h2> <p data-testid="cross-saved-name" class="svelte-z1qaf1">${escape_html$1(data.profile.name)}</p> <p data-testid="cross-saved-email" class="svelte-z1qaf1">${escape_html$1(data.profile.email)}</p> <p data-testid="cross-saved-biography" class="svelte-z1qaf1">${escape_html$1(data.profile.biography)}</p> <p data-testid="cross-saved-state" class="svelte-z1qaf1">${escape_html$1(data.profile.state)}</p></section></div> <button data-testid="client-interaction" type="button" class="svelte-z1qaf1">Check client interaction</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$49($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			head("3mw3v4", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Cross-page actions | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-3mw3v4"><p><a href="/actions">Actions</a> · <a href="/actions/cross/receive">View destination profile</a></p> <h1 data-testid="cross-source-title">Submit to another page</h1> <p>These forms post to a different Go page. The destination displays its own
    load and the action result.</p> <noscript><p data-testid="cross-noscript">JavaScript is disabled. Native cross-page actions still work.</p></noscript> <!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<section${attr("aria-label", mode + " cross-page actions")} class="svelte-3mw3v4"><h2>${escape_html$1(mode === "native" ? "Native forms" : "Enhanced forms")}</h2> <form method="POST" action="/actions/cross/receive?/save&amp;source=invite" novalidate=""${attr("data-testid", mode + "-cross-save")} class="svelte-3mw3v4"><label class="svelte-3mw3v4">Name <input name="name" value="Grace Hopper" class="svelte-3mw3v4"/></label> <label class="svelte-3mw3v4">Email <input name="email" value="grace@example.test" class="svelte-3mw3v4"/></label> <label class="svelte-3mw3v4">Biography <input name="biography" value="Compiler pioneer" class="svelte-3mw3v4"/></label> <button type="submit" class="svelte-3mw3v4">Save Grace on destination</button></form> <form method="POST" action="/actions/cross/receive?/save&amp;source=invite" novalidate=""${attr("data-testid", mode + "-cross-invalid")} class="svelte-3mw3v4"><label class="svelte-3mw3v4">Name <input name="name" value="Grace Hopper" class="svelte-3mw3v4"/></label> <label class="svelte-3mw3v4">Email <input name="email" value="grace-at-example" class="svelte-3mw3v4"/></label> <label class="svelte-3mw3v4">Biography <input name="biography" value="Keep this biography" class="svelte-3mw3v4"/></label> <button type="submit" class="svelte-3mw3v4">Reject invalid email on destination</button></form> <form method="POST" action="/actions/cross/receive?/signIn&amp;source=invite"${attr("data-testid", mode + "-cross-signin")} class="svelte-3mw3v4"><input type="hidden" name="username" value="ada" class="svelte-3mw3v4"/> <button type="submit" class="svelte-3mw3v4">Sign in from another page</button></form> <form method="POST" action="/actions/cross/receive?/forbidden&amp;source=invite"${attr("data-testid", mode + "-cross-forbidden")} class="svelte-3mw3v4"><button type="submit" class="svelte-3mw3v4">Attempt forbidden cross-page edit</button></form> <form method="POST" action="/actions/cross/receive?/unavailable&amp;source=invite"${attr("data-testid", mode + "-cross-unavailable")} class="svelte-3mw3v4"><button type="submit" class="svelte-3mw3v4">Trigger cross-page service failure</button></form></section>`);
			}
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$48($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { form } = $$props;
			head("1flso3e", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Default action | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-1flso3e"><p><a href="/actions">Actions</a></p> <h1 data-testid="default-title">Default action, no page load</h1> <p>This page declares only an unnamed Go action. Its receipt is the typed form
    result.</p> <p><a href="/actions/default/saved">Read the stored default profile</a></p> <noscript><p data-testid="default-noscript">JavaScript is disabled. The form still works.</p></noscript> <section aria-label="Action result"><p data-testid="default-status">Page status ${escape_html$1(page.status)}</p> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><h2 data-testid="default-receipt">${escape_html$1(form.receipt)}</h2> <p data-testid="default-saved-name">${escape_html$1(form.name)}</p> <p data-testid="default-saved-email">${escape_html$1(form.email)}</p> <p data-testid="default-saved-biography">${escape_html$1(form.biography)}</p> <p data-testid="default-saved-state">${escape_html$1(form.state)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section> <!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<form method="POST"${attr("data-testid", mode + "-default-form")} class="svelte-1flso3e"><h2>${escape_html$1(mode === "native" ? "Native submission" : "Enhanced submission")}</h2> <label class="svelte-1flso3e">Name <input name="name" value="Grace Hopper" class="svelte-1flso3e"/></label> <label class="svelte-1flso3e">Email <input name="email" value="grace@example.test" class="svelte-1flso3e"/></label> <label class="svelte-1flso3e">Biography <input name="biography" value="Compiler pioneer" class="svelte-1flso3e"/></label> <button type="submit" class="svelte-1flso3e">Save Grace</button></form>`);
			}
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$47($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			head("i191a6", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Stored default profile | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-i191a6"><p><a href="/actions/default">Default action</a></p> <h1 data-testid="default-stored-title">Stored default profile</h1> `);
			if (data.saved) $$renderer.push(`<!--[0--><p data-testid="default-stored-name">${escape_html$1(data.profile.name)}</p> <p data-testid="default-stored-email">${escape_html$1(data.profile.email)}</p> <p data-testid="default-stored-biography">${escape_html$1(data.profile.biography)}</p> <p data-testid="default-stored-state">${escape_html$1(data.profile.state)}</p>`);
			else $$renderer.push(`<!--[-1--><p data-testid="default-stored-empty">No default profile has been saved.</p>`);
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$46($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			let rejected = derived(() => form && "emailError" in form ? form : null);
			head("1e8zm8j", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Action without client JavaScript | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-1e8zm8j"><p><a href="/actions">Actions</a></p> <h1 data-testid="no-client-title">Actions without client JavaScript</h1> <p>The Go action and page load render this document. This page sets csr=false.</p> <noscript><p data-testid="no-client-noscript">JavaScript is disabled; the native form still works.</p></noscript> <div class="columns svelte-1e8zm8j"><div><form data-testid="no-client-form" method="POST" action="?/save" novalidate="" class="svelte-1e8zm8j"><label class="svelte-1e8zm8j">Name <input name="name"${attr("value", rejected() ? rejected().name : data.profile.name)} class="svelte-1e8zm8j"/></label> <label class="svelte-1e8zm8j">Email <input name="email"${attr("value", rejected() ? rejected().email : data.profile.email)} class="svelte-1e8zm8j"/></label> `);
			if (rejected()) $$renderer.push(`<!--[0--><p data-testid="no-client-email-error" class="error svelte-1e8zm8j">${escape_html$1(rejected().emailError)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <label class="svelte-1e8zm8j">Biography <textarea name="biography" class="svelte-1e8zm8j">`);
			const $$body = escape_html$1(rejected() ? rejected().biography : data.profile.biography);
			if ($$body) $$renderer.push(`${$$body}`);
			$$renderer.push(`</textarea></label> <button type="submit" class="svelte-1e8zm8j">Save profile</button></form> <form method="POST" action="?/forbidden" class="svelte-1e8zm8j"><button type="submit" class="svelte-1e8zm8j">Try forbidden action</button></form> <form method="POST" action="?/unavailable" class="svelte-1e8zm8j"><button type="submit" class="svelte-1e8zm8j">Try unavailable action</button></form></div> <aside aria-label="Action result and saved profile" class="svelte-1e8zm8j"><h2 class="svelte-1e8zm8j">Action result</h2> <p data-testid="no-client-status" class="svelte-1e8zm8j">Page status ${escape_html$1(page.status)}</p> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><p data-testid="no-client-receipt" class="svelte-1e8zm8j">${escape_html$1(form.receipt)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <h2 class="svelte-1e8zm8j">Saved profile</h2> <p data-testid="no-client-saved-name" class="svelte-1e8zm8j">${escape_html$1(data.profile.name)}</p> <p data-testid="no-client-saved-email" class="svelte-1e8zm8j">${escape_html$1(data.profile.email)}</p> <p data-testid="no-client-saved-biography" class="svelte-1e8zm8j">${escape_html$1(data.profile.biography)}</p> <p data-testid="no-client-saved-state" class="svelte-1e8zm8j">${escape_html$1(data.profile.state)}</p></aside></div> <section aria-label="Client rendering probe"><h2>Try a client-rendered destination</h2> <form method="POST" action="/actions/options/no-ssr?/save" novalidate="" class="svelte-1e8zm8j"><input type="hidden" name="name" value="Grace Hopper" class="svelte-1e8zm8j"/> <input type="hidden" name="email" value="grace@example.test" class="svelte-1e8zm8j"/> <input type="hidden" name="biography" value="Compiler pioneer" class="svelte-1e8zm8j"/> <button type="submit" class="svelte-1e8zm8j">Post valid client-rendered edit</button></form> <form method="POST" action="/actions/options/no-ssr?/save" novalidate="" class="svelte-1e8zm8j"><input type="hidden" name="name" value="Grace Hopper" class="svelte-1e8zm8j"/> <input type="hidden" name="email" value="grace-at-example" class="svelte-1e8zm8j"/> <input type="hidden" name="biography" value="Keep this biography" class="svelte-1e8zm8j"/> <button type="submit" class="svelte-1e8zm8j">Post invalid client-rendered edit</button></form> <form method="POST" action="/actions/options/no-ssr?/forbidden" class="svelte-1e8zm8j"><button type="submit" class="svelte-1e8zm8j">Post forbidden client-rendered edit</button></form> <form method="POST" action="/actions/options/no-ssr?/unavailable" class="svelte-1e8zm8j"><button type="submit" class="svelte-1e8zm8j">Post unavailable client-rendered edit</button></form></section></main>`);
		});
	}
	function _page$45($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			let rejected = derived(() => form && "emailError" in form ? form : null);
			head("4kkqqu", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Action with client rendering | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-4kkqqu"><p><a href="/actions">Actions</a></p> <h1 data-testid="no-ssr-title">Client-rendered actions</h1> <p>This page sets ssr=false. A native POST returns a shell; Kit draws the
    ordinary page after boot.</p> <noscript><p data-testid="no-ssr-noscript">JavaScript is disabled; this page needs client rendering.</p></noscript> <div class="columns svelte-4kkqqu"><div><!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<form${attr("data-testid", mode + "-no-ssr-form")} method="POST" action="?/save" novalidate="" class="svelte-4kkqqu"><h2>${escape_html$1(mode === "native" ? "Native save" : "Enhanced save")}</h2> <label class="svelte-4kkqqu">Name <input name="name"${attr("value", rejected() ? rejected().name : data.profile.name)} class="svelte-4kkqqu"/></label> <label class="svelte-4kkqqu">Email <input name="email"${attr("value", rejected() ? rejected().email : data.profile.email)} class="svelte-4kkqqu"/></label> <label class="svelte-4kkqqu">Biography <textarea name="biography" class="svelte-4kkqqu">`);
				const $$body = escape_html$1(rejected() ? rejected().biography : data.profile.biography);
				if ($$body) $$renderer.push(`${$$body}`);
				$$renderer.push(`</textarea></label> <button type="submit" class="svelte-4kkqqu">Save profile</button></form>`);
			}
			$$renderer.push(`<!--]--> <form method="POST" action="?/signIn" class="svelte-4kkqqu"><input type="hidden" name="username" value="ada" class="svelte-4kkqqu"/><button type="submit" class="svelte-4kkqqu">Native sign in</button></form> <form method="POST" action="?/forbidden" class="svelte-4kkqqu"><button type="submit" class="svelte-4kkqqu">Try forbidden action</button></form> <form method="POST" action="?/unavailable" class="svelte-4kkqqu"><button type="submit" class="svelte-4kkqqu">Try unavailable action</button></form></div> <aside aria-label="Action result and saved profile" class="svelte-4kkqqu"><h2 class="svelte-4kkqqu">Action result</h2> <p data-testid="no-ssr-status" class="svelte-4kkqqu">Page status ${escape_html$1(page.status)}</p> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><p data-testid="no-ssr-receipt" class="svelte-4kkqqu">${escape_html$1(form.receipt)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if (rejected()) $$renderer.push(`<!--[0--><p data-testid="no-ssr-email-error" class="error svelte-4kkqqu">${escape_html$1(rejected().emailError)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <h2 class="svelte-4kkqqu">Saved profile</h2> <p data-testid="no-ssr-saved-name" class="svelte-4kkqqu">${escape_html$1(data.profile.name)}</p> <p data-testid="no-ssr-saved-email" class="svelte-4kkqqu">${escape_html$1(data.profile.email)}</p> <p data-testid="no-ssr-saved-biography" class="svelte-4kkqqu">${escape_html$1(data.profile.biography)}</p> <p data-testid="no-ssr-saved-state" class="svelte-4kkqqu">${escape_html$1(data.profile.state)}</p></aside></div></main>`);
		});
	}
	function _page$44($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			head("1pj8lt", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Two profiles | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-1pj8lt"><p><a href="/actions">Actions</a></p> <h1 data-testid="profiles-title">Two visitor profiles</h1> <p>Choose a profile. Saving changes only the selected record.</p> <nav><a href="/actions/profiles/ada">Edit Ada</a> · <a href="/actions/profiles/grace">Edit Grace</a></nav> <p data-testid="selected-profile">Editing ${escape_html$1(data.selected)}</p> <noscript><p data-testid="profiles-noscript">JavaScript is disabled. Native actions still work.</p></noscript> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><p data-testid="profile-receipt">${escape_html$1(form.receipt)}</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <div class="pair svelte-1pj8lt"><section aria-label="Ada profile" class="svelte-1pj8lt"><h2>Ada record</h2> <p data-testid="ada-name">${escape_html$1(data.ada.name)}</p> <p data-testid="ada-email">${escape_html$1(data.ada.email)}</p> <p data-testid="ada-biography">${escape_html$1(data.ada.biography)}</p> <p data-testid="ada-state">${escape_html$1(data.ada.state)}</p></section> <section aria-label="Grace profile" class="svelte-1pj8lt"><h2>Grace record</h2> <p data-testid="grace-name">${escape_html$1(data.grace.name)}</p> <p data-testid="grace-email">${escape_html$1(data.grace.email)}</p> <p data-testid="grace-biography">${escape_html$1(data.grace.biography)}</p> <p data-testid="grace-state">${escape_html$1(data.grace.state)}</p></section></div> <!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<form method="POST" action="?/save"${attr("data-testid", mode + "-profile-form")} class="svelte-1pj8lt"><h2>${escape_html$1(mode === "native" ? "Native edit" : "Enhanced edit")}</h2> <label class="svelte-1pj8lt">Name <input name="name"${attr("value", data.selected === "ada" ? data.ada.name : data.grace.name)} class="svelte-1pj8lt"/></label> <label class="svelte-1pj8lt">Email <input name="email"${attr("value", data.selected === "ada" ? data.ada.email : data.grace.email)} class="svelte-1pj8lt"/></label> <label class="svelte-1pj8lt">Biography <input name="biography"${attr("value", data.selected === "ada" ? data.ada.biography : data.grace.biography)} class="svelte-1pj8lt"/></label> <button type="submit" class="svelte-1pj8lt">Save selected profile</button></form>`);
			}
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$43($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			head("1xokh7v", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Signed in | skgo example</title>`);
				});
			});
			$$renderer.push(`<section class="card svelte-1xokh7v">`);
			if (data.required) $$renderer.push(`<!--[0--><h1 data-testid="hook-sign-in-title">Sign in required</h1> <p data-testid="hook-sign-in-message">The request was intercepted before the profile action ran.</p>`);
			else $$renderer.push(`<!--[-1--><h1 data-testid="signed-in-title">Signed in as ${escape_html$1(data.user)}</h1>`);
			$$renderer.push(`<!--]--> <noscript><p data-testid="signed-in-noscript">JavaScript is disabled. The Go session still works.</p></noscript> `);
			if (!data.required) $$renderer.push(`<!--[0--><p data-testid="signed-in-cookie">The session cookie was read by the Go page load.</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <p><a href="/actions">Return to the profile editor</a></p> <button data-testid="client-interaction" type="button" class="svelte-1xokh7v">Check client interaction</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section>`);
		});
	}
	var routes = [
		{
			"id": "/",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/about",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/account",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/account/orders",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/account/reruns/[slug]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/account/statement",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions",
			"page": true,
			"endpoint": true
		},
		{
			"id": "/actions/cross/receive",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/cross/send",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/default",
			"page": true,
			"endpoint": true
		},
		{
			"id": "/actions/default/saved",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/options/no-client",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/options/no-ssr",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/profiles/[profile]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/actions/signed-in",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/api",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/api/fatal",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/api/replay-count",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/api/replay/[kind]",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/api/request-fetch",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/api/todos",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/async-ssr/control/[group]",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/async-ssr/[mode]/[group]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/batch",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/bits-ui",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/console",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/contact",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/destinations",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/docs/[...rest]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/empty",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/boundary",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/command",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/expected",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/redirect",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/render",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/server-only",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/error/unexpected",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/event-layout/[section]/[item]/one",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/event-layout/[section]/[item]/two",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/fetch",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/invoices",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/items/[id]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/live",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/middleware",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/middleware/[slug]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/nested-universal",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/optional",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/plain",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/prerender-consumer",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/prerender/[slug]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/(marketing)/pricing",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/render-paths",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/request-fetch",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/runes-class",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/shadow/[slug]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/shadow/[...rest]",
			"page": false,
			"endpoint": true
		},
		{
			"id": "/spa",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/stream",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/todos",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/todos/gate",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/todos/pair",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/todos/[id]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/typed-dependencies/[a=Order]/[b=Order]/[ignored]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/typed-load/[number=Order]",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/universal-fetch",
			"page": true,
			"endpoint": false
		},
		{
			"id": "/universal-fetch/no-script",
			"page": true,
			"endpoint": false
		}
	];
	function _page$42($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let draft = "";
			const apiRoutes = routes.filter((route) => route.id.startsWith("/api"));
			$$renderer.push(`<h1 data-testid="title">API</h1> <p>Everything below came from <code>/api/todos</code>, an ordinary Go HTTP
  handler written in <code>src/routes/api/todos/server.go</code> beside the route it serves.</p> <h2>SvelteKit 3 route manifest</h2> <ul data-testid="api-manifest"><!--[-->`);
			const each_array = ensure_array_like(apiRoutes);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let route = each_array[$$index];
				$$renderer.push(`<li data-testid="api-manifest-route"><code>${escape_html$1(route.id)}</code> — page: ${escape_html$1(route.page)}, endpoint: ${escape_html$1(route.endpoint)}</li>`);
			}
			$$renderer.push(`<!--]--></ul> `);
			$$renderer.push(`<!--[-1--><p data-testid="api-pending">calling the endpoint…</p>`);
			$$renderer.push(`<!--]--> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <form><label>New todo <input data-testid="api-draft"${attr("value", draft)}/></label> <button data-testid="api-post" type="submit">POST it</button></form> <button data-testid="api-delete" type="button">DELETE it (a method this route does not declare)</button> <button data-testid="api-query" type="button">QUERY for “adapter”</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	function _page$41($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { params } = $$props;
			$$renderer.push(`<h1>Concurrent server rendering</h1> <p>Three independent Go operations must start before any can finish.</p> <p data-testid="probe-group">Request: ${escape_html$1(params.group)}</p> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="probe-failure">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					if (params.mode === "abandoned" || params.mode === "current") {
						$$renderer.push(`<!--[0--><p data-testid="held-value">`);
						$$renderer.push(async () => escape_html$1((await save(getHeld({
							group: params.group,
							key: params.mode
						})))()));
						$$renderer.push(`</p>`);
					} else if (params.mode === "dependent") {
						$$renderer.push(`<!--[1--><p data-testid="dependent-value">`);
						$$renderer.push(async () => escape_html$1((await save(getDependent((await save(getSeed(params.group)))())))()));
						$$renderer.push(`</p>`);
					} else if (params.mode === "mixed") {
						$$renderer.push(`<!--[2--><ul data-testid="probe-values"><li>`);
						$$renderer.push(async () => escape_html$1((await save(getValue({
							group: params.group,
							key: "amber"
						})))()));
						$$renderer.push(`</li> <li>`);
						$$renderer.push(async () => escape_html$1((await save(getBatch({
							group: params.group,
							key: "one"
						})))()));
						$$renderer.push(`</li> <li>`);
						$$renderer.push(async () => escape_html$1((await save(getBatch({
							group: params.group,
							key: "two"
						})))()));
						$$renderer.push(`</li> <li>`);
						$$renderer.push(async () => escape_html$1((await save(watchValue({
							group: params.group,
							key: "live"
						})))()));
						$$renderer.push(`</li></ul>`);
					} else {
						$$renderer.push(`<!--[-1--><ul data-testid="probe-values"><li>`);
						$$renderer.push(async () => escape_html$1((await save(getValue({
							group: params.group,
							key: "amber"
						})))()));
						$$renderer.push(`</li> <li>`);
						$$renderer.push(async () => escape_html$1((await save(getValue({
							group: params.group,
							key: "birch"
						})))()));
						$$renderer.push(`</li> <li>`);
						$$renderer.push(async () => escape_html$1((await save(getValue({
							group: params.group,
							key: "cobalt"
						})))()));
						$$renderer.push(`</li></ul>`);
					}
					$$renderer.push(`<!--]-->`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function Ticker($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { symbol } = $$props;
			var quote;
			var $$promises = $$renderer.run([async () => quote = await async_derived(() => getQuotes(symbol))]);
			$$renderer.async([$$promises[0]], ($$renderer) => {
				$$renderer.push(`<tr data-testid="quote"${attr("data-symbol", symbol)}><td data-testid="quote-symbol">`);
				$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(quote().symbol)));
				$$renderer.push(`</td><td data-testid="quote-price">`);
				$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1((quote().cents / 100).toFixed(2))));
				$$renderer.push(`</td><td data-testid="quote-batch">batch of `);
				$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(quote().batchSize)));
				$$renderer.push(`</td></tr>`);
			});
		});
	}
	function _page$40($$renderer) {
		const symbols = [
			"SKGO",
			"GOJA",
			"KITX",
			"SVLT"
		];
		$$renderer.push(`<h1 data-testid="title">Batch</h1> <p>Each row below asked for one symbol on its own. They were answered by a single
  call to one Go function, holding every symbol at once — which is what each row
  reports.</p> `);
		{
			function failed($$renderer, error) {
				$$renderer.push(`<p data-testid="quotes-failed">${escape_html$1(error.message)}</p>`);
			}
			$$renderer.boundary({ failed }, ($$renderer) => {
				$$renderer.push(`<!--[-->`);
				{
					$$renderer.push(`<table data-testid="quotes"><thead><tr><th class="svelte-12kugq3">Symbol</th><th class="svelte-12kugq3">Price</th><th class="svelte-12kugq3">Answered in a call of</th></tr></thead><tbody><!--[-->`);
					const each_array = ensure_array_like(symbols);
					for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
						let symbol = each_array[$$index];
						Ticker($$renderer, { symbol });
					}
					$$renderer.push(`<!--]--></tbody></table>`);
				}
				$$renderer.push(`<!--]-->`);
			});
		}
	}
	function isObject(value) {
		return value !== null && typeof value === "object";
	}
	var CLASS_VALUE_PRIMITIVE_TYPES = [
		"string",
		"number",
		"bigint",
		"boolean"
	];
	function isClassValue(value) {
		if (value === null || value === void 0) return true;
		if (CLASS_VALUE_PRIMITIVE_TYPES.includes(typeof value)) return true;
		if (Array.isArray(value)) return value.every((item) => isClassValue(item));
		if (typeof value === "object") {
			if (Object.getPrototypeOf(value) !== Object.prototype) return false;
			return true;
		}
		return false;
	}
	var BoxSymbol = Symbol("box");
	var isWritableSymbol = Symbol("is-writable");
	function boxWith(getter, setter) {
		const derived$1 = derived(getter);
		if (setter) return {
			[BoxSymbol]: true,
			[isWritableSymbol]: true,
			get current() {
				return derived$1();
			},
			set current(v) {
				setter(v);
			}
		};
		return {
			[BoxSymbol]: true,
			get current() {
				return getter();
			}
		};
	}
	/**
	* @returns Whether the value is a Box
	*
	* @see {@link https://runed.dev/docs/functions/box}
	*/
	function isBox(value) {
		return isObject(value) && BoxSymbol in value;
	}
	function simpleBox(initialValue) {
		let current = initialValue;
		return {
			[BoxSymbol]: true,
			[isWritableSymbol]: true,
			get current() {
				return current;
			},
			set current(v) {
				current = v;
			}
		};
	}
	/**
	* Composes event handlers into a single function that can be called with an event.
	* If the previous handler cancels the event using `event.preventDefault()`, the handlers
	* that follow will not be called.
	*/
	function composeHandlers(...handlers) {
		return function(e) {
			for (const handler of handlers) {
				if (!handler) continue;
				if (e.defaultPrevented) return;
				if (typeof handler === "function") handler.call(this, e);
				else handler.current?.call(this, e);
			}
		};
	}
	var COMMENT_REGEX = /\/\*[^*]*\*+([^/*][^*]*\*+)*\//g;
	var NEWLINE_REGEX = /\n/g;
	var WHITESPACE_REGEX = /^\s*/;
	var PROPERTY_REGEX = /^(\*?[-#/*\\\w]+(\[[0-9a-z_-]+\])?)\s*/;
	var COLON_REGEX = /^:\s*/;
	var VALUE_REGEX = /^((?:'(?:\\'|.)*?'|"(?:\\"|.)*?"|\([^)]*?\)|[^};])+)/;
	var SEMICOLON_REGEX = /^[;\s]*/;
	var TRIM_REGEX = /^\s+|\s+$/g;
	var NEWLINE = "\n";
	var FORWARD_SLASH = "/";
	var ASTERISK = "*";
	var EMPTY_STRING = "";
	var TYPE_COMMENT = "comment";
	var TYPE_DECLARATION = "declaration";
	/**
	* @param {String} style
	* @param {Object} [options]
	* @return {Object[]}
	* @throws {TypeError}
	* @throws {Error}
	*/
	function index(style, options) {
		if (typeof style !== "string") throw new TypeError("First argument must be a string");
		if (!style) return [];
		options = options || {};
		/**
		* Positional.
		*/
		var lineno = 1;
		var column = 1;
		/**
		* Update lineno and column based on `str`.
		*
		* @param {String} str
		*/
		function updatePosition(str) {
			var lines = str.match(NEWLINE_REGEX);
			if (lines) lineno += lines.length;
			var i = str.lastIndexOf(NEWLINE);
			column = ~i ? str.length - i : column + str.length;
		}
		/**
		* Mark position and patch `node.position`.
		*
		* @return {Function}
		*/
		function position() {
			var start = {
				line: lineno,
				column
			};
			return function(node) {
				node.position = new Position(start);
				whitespace();
				return node;
			};
		}
		/**
		* Store position information for a node.
		*
		* @constructor
		* @property {Object} start
		* @property {Object} end
		* @property {undefined|String} source
		*/
		function Position(start) {
			this.start = start;
			this.end = {
				line: lineno,
				column
			};
			this.source = options.source;
		}
		/**
		* Non-enumerable source string.
		*/
		Position.prototype.content = style;
		/**
		* Error `msg`.
		*
		* @param {String} msg
		* @throws {Error}
		*/
		function error(msg) {
			var err = /* @__PURE__ */ new Error(options.source + ":" + lineno + ":" + column + ": " + msg);
			err.reason = msg;
			err.filename = options.source;
			err.line = lineno;
			err.column = column;
			err.source = style;
			if (options.silent);
			else throw err;
		}
		/**
		* Match `re` and return captures.
		*
		* @param {RegExp} re
		* @return {undefined|Array}
		*/
		function match(re) {
			var m = re.exec(style);
			if (!m) return;
			var str = m[0];
			updatePosition(str);
			style = style.slice(str.length);
			return m;
		}
		/**
		* Parse whitespace.
		*/
		function whitespace() {
			match(WHITESPACE_REGEX);
		}
		/**
		* Parse comments.
		*
		* @param {Object[]} [rules]
		* @return {Object[]}
		*/
		function comments(rules) {
			var c;
			rules = rules || [];
			while (c = comment()) if (c !== false) rules.push(c);
			return rules;
		}
		/**
		* Parse comment.
		*
		* @return {Object}
		* @throws {Error}
		*/
		function comment() {
			var pos = position();
			if (FORWARD_SLASH != style.charAt(0) || ASTERISK != style.charAt(1)) return;
			var i = 2;
			while (EMPTY_STRING != style.charAt(i) && (ASTERISK != style.charAt(i) || FORWARD_SLASH != style.charAt(i + 1))) ++i;
			i += 2;
			if (EMPTY_STRING === style.charAt(i - 1)) return error("End of comment missing");
			var str = style.slice(2, i - 2);
			column += 2;
			updatePosition(str);
			style = style.slice(i);
			column += 2;
			return pos({
				type: TYPE_COMMENT,
				comment: str
			});
		}
		/**
		* Parse declaration.
		*
		* @return {Object}
		* @throws {Error}
		*/
		function declaration() {
			var pos = position();
			var prop = match(PROPERTY_REGEX);
			if (!prop) return;
			comment();
			if (!match(COLON_REGEX)) return error("property missing ':'");
			var val = match(VALUE_REGEX);
			var ret = pos({
				type: TYPE_DECLARATION,
				property: trim(prop[0].replace(COMMENT_REGEX, EMPTY_STRING)),
				value: val ? trim(val[0].replace(COMMENT_REGEX, EMPTY_STRING)) : EMPTY_STRING
			});
			match(SEMICOLON_REGEX);
			return ret;
		}
		/**
		* Parse declarations.
		*
		* @return {Object[]}
		*/
		function declarations() {
			var decls = [];
			comments(decls);
			var decl;
			while (decl = declaration()) if (decl !== false) {
				decls.push(decl);
				comments(decls);
			}
			return decls;
		}
		whitespace();
		return declarations();
	}
	/**
	* Trim `str`.
	*
	* @param {String} str
	* @return {String}
	*/
	function trim(str) {
		return str ? str.replace(TRIM_REGEX, EMPTY_STRING) : EMPTY_STRING;
	}
	/**
	* Parses inline style to object.
	*
	* @param style - Inline style.
	* @param iterator - Iterator.
	* @returns - Style object or null.
	*
	* @example Parsing inline style to object:
	*
	* ```js
	* import parse from 'style-to-object';
	* parse('line-height: 42;'); // { 'line-height': '42' }
	* ```
	*/
	function StyleToObject(style, iterator) {
		let styleObject = null;
		if (!style || typeof style !== "string") return styleObject;
		const declarations = index(style);
		const hasIterator = typeof iterator === "function";
		declarations.forEach((declaration) => {
			if (declaration.type !== "declaration") return;
			const { property, value } = declaration;
			if (hasIterator) iterator(property, value, declaration);
			else if (value) {
				styleObject = styleObject || {};
				styleObject[property] = value;
			}
		});
		return styleObject;
	}
	var NUMBER_CHAR_RE = /\d/;
	var STR_SPLITTERS = [
		"-",
		"_",
		"/",
		"."
	];
	function isUppercase(char = "") {
		if (NUMBER_CHAR_RE.test(char)) return void 0;
		return char !== char.toLowerCase();
	}
	function splitByCase(str) {
		const parts = [];
		let buff = "";
		let previousUpper;
		let previousSplitter;
		for (const char of str) {
			const isSplitter = STR_SPLITTERS.includes(char);
			if (isSplitter === true) {
				parts.push(buff);
				buff = "";
				previousUpper = void 0;
				continue;
			}
			const isUpper = isUppercase(char);
			if (previousSplitter === false) {
				if (previousUpper === false && isUpper === true) {
					parts.push(buff);
					buff = char;
					previousUpper = isUpper;
					continue;
				}
				if (previousUpper === true && isUpper === false && buff.length > 1) {
					const lastChar = buff.at(-1);
					parts.push(buff.slice(0, Math.max(0, buff.length - 1)));
					buff = lastChar + char;
					previousUpper = isUpper;
					continue;
				}
			}
			buff += char;
			previousUpper = isUpper;
			previousSplitter = isSplitter;
		}
		parts.push(buff);
		return parts;
	}
	function pascalCase(str) {
		if (!str) return "";
		return splitByCase(str).map((p) => upperFirst(p)).join("");
	}
	function camelCase(str) {
		return lowerFirst(pascalCase(str || ""));
	}
	function upperFirst(str) {
		return str ? str[0].toUpperCase() + str.slice(1) : "";
	}
	function lowerFirst(str) {
		return str ? str[0].toLowerCase() + str.slice(1) : "";
	}
	function cssToStyleObj(css) {
		if (!css) return {};
		const styleObj = {};
		function iterator(name, value) {
			if (name.startsWith("-moz-") || name.startsWith("-webkit-") || name.startsWith("-ms-") || name.startsWith("-o-")) {
				styleObj[pascalCase(name)] = value;
				return;
			}
			if (name.startsWith("--")) {
				styleObj[name] = value;
				return;
			}
			styleObj[camelCase(name)] = value;
		}
		StyleToObject(css, iterator);
		return styleObj;
	}
	/**
	* Executes an array of callback functions with the same arguments.
	* @template T The types of the arguments that the callback functions take.
	* @param callbacks array of callback functions to execute.
	* @returns A new function that executes all of the original callback functions with the same arguments.
	*/
	function executeCallbacks(...callbacks) {
		return (...args) => {
			for (const callback of callbacks) if (typeof callback === "function") callback(...args);
		};
	}
	function createParser(matcher, replacer) {
		const regex = RegExp(matcher, "g");
		return (str) => {
			if (typeof str !== "string") throw new TypeError(`expected an argument of type string, but got ${typeof str}`);
			if (!str.match(regex)) return str;
			return str.replace(regex, replacer);
		};
	}
	var camelToKebab = createParser(/[A-Z]/, (match) => `-${match.toLowerCase()}`);
	function styleToCSS(styleObj) {
		if (!styleObj || typeof styleObj !== "object" || Array.isArray(styleObj)) throw new TypeError(`expected an argument of type object, but got ${typeof styleObj}`);
		return Object.keys(styleObj).map((property) => `${camelToKebab(property)}: ${styleObj[property]};`).join("\n");
	}
	function styleToString(style = {}) {
		return styleToCSS(style).replace("\n", " ");
	}
	var EVENT_LIST_SET = /* @__PURE__ */ new Set([
		"onabort",
		"onanimationcancel",
		"onanimationend",
		"onanimationiteration",
		"onanimationstart",
		"onauxclick",
		"onbeforeinput",
		"onbeforetoggle",
		"onblur",
		"oncancel",
		"oncanplay",
		"oncanplaythrough",
		"onchange",
		"onclick",
		"onclose",
		"oncompositionend",
		"oncompositionstart",
		"oncompositionupdate",
		"oncontextlost",
		"oncontextmenu",
		"oncontextrestored",
		"oncopy",
		"oncuechange",
		"oncut",
		"ondblclick",
		"ondrag",
		"ondragend",
		"ondragenter",
		"ondragleave",
		"ondragover",
		"ondragstart",
		"ondrop",
		"ondurationchange",
		"onemptied",
		"onended",
		"onerror",
		"onfocus",
		"onfocusin",
		"onfocusout",
		"onformdata",
		"ongotpointercapture",
		"oninput",
		"oninvalid",
		"onkeydown",
		"onkeypress",
		"onkeyup",
		"onload",
		"onloadeddata",
		"onloadedmetadata",
		"onloadstart",
		"onlostpointercapture",
		"onmousedown",
		"onmouseenter",
		"onmouseleave",
		"onmousemove",
		"onmouseout",
		"onmouseover",
		"onmouseup",
		"onpaste",
		"onpause",
		"onplay",
		"onplaying",
		"onpointercancel",
		"onpointerdown",
		"onpointerenter",
		"onpointerleave",
		"onpointermove",
		"onpointerout",
		"onpointerover",
		"onpointerup",
		"onprogress",
		"onratechange",
		"onreset",
		"onresize",
		"onscroll",
		"onscrollend",
		"onsecuritypolicyviolation",
		"onseeked",
		"onseeking",
		"onselect",
		"onselectionchange",
		"onselectstart",
		"onslotchange",
		"onstalled",
		"onsubmit",
		"onsuspend",
		"ontimeupdate",
		"ontoggle",
		"ontouchcancel",
		"ontouchend",
		"ontouchmove",
		"ontouchstart",
		"ontransitioncancel",
		"ontransitionend",
		"ontransitionrun",
		"ontransitionstart",
		"onvolumechange",
		"onwaiting",
		"onwebkitanimationend",
		"onwebkitanimationiteration",
		"onwebkitanimationstart",
		"onwebkittransitionend",
		"onwheel"
	]);
	/**
	* Modified from https://github.com/adobe/react-spectrum/blob/main/packages/%40react-aria/utils/src/mergeProps.ts (see NOTICE.txt for source)
	*/
	function isEventHandler(key) {
		return EVENT_LIST_SET.has(key);
	}
	/**
	* Given a list of prop objects, merges them into a single object.
	* - Automatically composes event handlers (e.g. `onclick`, `oninput`, etc.)
	* - Chains regular functions with the same name so they are called in order
	* - Merges class strings with `clsx`
	* - Merges style objects and converts them to strings
	* - Handles a bug with Svelte where setting the `hidden` attribute to `false` doesn't remove it
	* - Overrides other values with the last one
	*/
	function mergeProps(...args) {
		const result = { ...args[0] };
		for (let i = 1; i < args.length; i++) {
			const props = args[i];
			if (!props) continue;
			for (const key of Object.keys(props)) {
				const a = result[key];
				const b = props[key];
				const aIsFunction = typeof a === "function";
				const bIsFunction = typeof b === "function";
				if (aIsFunction && typeof bIsFunction && isEventHandler(key)) result[key] = composeHandlers(a, b);
				else if (aIsFunction && bIsFunction) result[key] = executeCallbacks(a, b);
				else if (key === "class") {
					const aIsClassValue = isClassValue(a);
					const bIsClassValue = isClassValue(b);
					if (aIsClassValue && bIsClassValue) result[key] = clsx$1(a, b);
					else if (aIsClassValue) result[key] = clsx$1(a);
					else if (bIsClassValue) result[key] = clsx$1(b);
				} else if (key === "style") {
					const aIsObject = typeof a === "object";
					const bIsObject = typeof b === "object";
					const aIsString = typeof a === "string";
					const bIsString = typeof b === "string";
					if (aIsObject && bIsObject) result[key] = {
						...a,
						...b
					};
					else if (aIsObject && bIsString) {
						const parsedStyle = cssToStyleObj(b);
						result[key] = {
							...a,
							...parsedStyle
						};
					} else if (aIsString && bIsObject) result[key] = {
						...cssToStyleObj(a),
						...b
					};
					else if (aIsString && bIsString) {
						const parsedStyleA = cssToStyleObj(a);
						const parsedStyleB = cssToStyleObj(b);
						result[key] = {
							...parsedStyleA,
							...parsedStyleB
						};
					} else if (aIsObject) result[key] = a;
					else if (bIsObject) result[key] = b;
					else if (aIsString) result[key] = a;
					else if (bIsString) result[key] = b;
				} else result[key] = b !== void 0 ? b : a;
			}
			for (const key of Object.getOwnPropertySymbols(props)) {
				const a = result[key];
				const b = props[key];
				result[key] = b !== void 0 ? b : a;
			}
		}
		if (typeof result.style === "object") result.style = styleToString(result.style).replaceAll("\n", " ");
		if (result.hidden === false) {
			result.hidden = void 0;
			delete result.hidden;
		}
		if (result.disabled === false) {
			result.disabled = void 0;
			delete result.disabled;
		}
		return result;
	}
	var defaultWindow = void 0;
	/**
	* Handles getting the active element in a document or shadow root.
	* If the active element is within a shadow root, it will traverse the shadow root
	* to find the active element.
	* If not, it will return the active element in the document.
	*
	* @param document A document or shadow root to get the active element from.
	* @returns The active element in the document or shadow root.
	*/
	function getActiveElement$1(document) {
		let activeElement = document.activeElement;
		while (activeElement?.shadowRoot) {
			const node = activeElement.shadowRoot.activeElement;
			if (node === activeElement) break;
			else activeElement = node;
		}
		return activeElement;
	}
	/**
	* Used on elements, as a map of event type -> event handler,
	* and on events themselves to track which element handled an event
	*/
	var event_symbol = Symbol("events");
	/**
	* @param {string} event_name
	* @param {EventTarget} dom
	* @param {EventListener} [handler]
	* @param {AddEventListenerOptions} [options]
	*/
	function create_event(event_name, dom, handler, options = {}) {
		/**
		* @this {EventTarget}
		*/
		function target_handler(event) {
			if (!options.capture) handle_event_propagation.call(dom, event);
			if (!event.cancelBubble) return without_reactive_context(() => {
				return handler?.call(this, event);
			});
		}
		if (event_name.startsWith("pointer") || event_name.startsWith("touch") || event_name === "wheel") {
			target_handler.__removed = false;
			queue_micro_task(() => {
				if (!target_handler.__removed) dom.addEventListener(event_name, target_handler, options);
			});
		} else dom.addEventListener(event_name, target_handler, options);
		return target_handler;
	}
	/**
	* Attaches an event handler to an element and returns a function that removes the handler. Using this
	* rather than `addEventListener` will preserve the correct order relative to handlers added declaratively
	* (with attributes like `onclick`), which use event delegation for performance reasons
	*
	* @param {EventTarget} element
	* @param {string} type
	* @param {EventListener} handler
	* @param {AddEventListenerOptions} [options]
	*/
	function on(element, type, handler, options = {}) {
		var target_handler = create_event(type, element, handler, options);
		return () => {
			target_handler.__removed = true;
			element.removeEventListener(type, target_handler, options);
		};
	}
	var last_propagated_event = null;
	var last_propagated_event_clear_scheduled = false;
	/**
	* @this {EventTarget}
	* @param {Event} event
	* @returns {void}
	*/
	function handle_event_propagation(event) {
		var handler_element = this;
		var owner_document = handler_element.ownerDocument;
		var event_name = event.type;
		var path = event.composedPath?.() || [];
		var current_target = path[0] || event.target;
		last_propagated_event = event;
		if (!last_propagated_event_clear_scheduled) {
			last_propagated_event_clear_scheduled = true;
			setTimeout(() => {
				last_propagated_event_clear_scheduled = false;
				last_propagated_event = null;
			});
		}
		var path_idx = 0;
		var handled_at = last_propagated_event === event && event[event_symbol];
		if (handled_at) {
			var at_idx = path.indexOf(handled_at);
			if (at_idx !== -1 && (handler_element === document || handler_element === window)) {
				event[event_symbol] = handler_element;
				return;
			}
			var handler_idx = path.indexOf(handler_element);
			if (handler_idx === -1) return;
			if (at_idx <= handler_idx) path_idx = at_idx;
		}
		current_target = path[path_idx] || event.target;
		if (current_target === handler_element) return;
		define_property(event, "currentTarget", {
			configurable: true,
			get() {
				return current_target || owner_document;
			}
		});
		var previous_reaction = active_reaction;
		var previous_effect = active_effect;
		set_active_reaction(null);
		set_active_effect(null);
		try {
			/**
			* @type {unknown}
			*/
			var throw_error;
			/**
			* @type {unknown[]}
			*/
			var other_errors = [];
			while (current_target !== null) {
				if (current_target === handler_element) break;
				try {
					var delegated = current_target[event_symbol]?.[event_name];
					if (delegated != null && (!current_target.disabled || event.target === current_target)) delegated.call(current_target, event);
				} catch (error) {
					if (throw_error) other_errors.push(error);
					else throw_error = error;
				}
				if (event.cancelBubble) break;
				path_idx++;
				current_target = path_idx < path.length ? path[path_idx] : null;
			}
			if (throw_error) {
				for (let error of other_errors) queueMicrotask(() => {
					throw error;
				});
				throw throw_error;
			}
		} finally {
			event[event_symbol] = handler_element;
			delete event.currentTarget;
			set_active_reaction(previous_reaction);
			set_active_effect(previous_effect);
		}
	}
	globalThis.Date;
	globalThis.Set;
	var SvelteMap = globalThis.Map;
	globalThis.URL;
	globalThis.URLSearchParams;
	/**
	* @param {any} _
	*/
	function createSubscriber$1(_) {
		return () => {};
	}
	var ActiveElement = class {
		#document;
		#subscribe;
		constructor(options = {}) {
			const { window = defaultWindow, document = window?.document } = options;
			if (window === void 0) return;
			this.#document = document;
			this.#subscribe = createSubscriber$1((update) => {
				const cleanupFocusIn = on(window, "focusin", update);
				const cleanupFocusOut = on(window, "focusout", update);
				return () => {
					cleanupFocusIn();
					cleanupFocusOut();
				};
			});
		}
		get current() {
			this.#subscribe?.();
			if (!this.#document) return null;
			return getActiveElement$1(this.#document);
		}
	};
	new ActiveElement();
	function isFunction(value) {
		return typeof value === "function";
	}
	function extract(value, defaultValue) {
		if (isFunction(value)) {
			const gotten = value();
			if (gotten === void 0) return defaultValue;
			return gotten;
		}
		if (value === void 0) return defaultValue;
		return value;
	}
	var Context = class {
		#name;
		#key;
		/**
		* @param name The name of the context.
		* This is used for generating the context key and error messages.
		*/
		constructor(name) {
			this.#name = name;
			this.#key = Symbol(name);
		}
		/**
		* The key used to get and set the context.
		*
		* It is not recommended to use this value directly.
		* Instead, use the methods provided by this class.
		*/
		get key() {
			return this.#key;
		}
		/**
		* Checks whether this has been set in the context of a parent component.
		*
		* Must be called during component initialisation.
		*/
		exists() {
			return hasContext(this.#key);
		}
		/**
		* Retrieves the context that belongs to the closest parent component.
		*
		* Must be called during component initialisation.
		*
		* @throws An error if the context does not exist.
		*/
		get() {
			const context = getContext(this.#key);
			if (context === void 0) throw new Error(`Context "${this.#name}" not found`);
			return context;
		}
		/**
		* Retrieves the context that belongs to the closest parent component,
		* or the given fallback value if the context does not exist.
		*
		* Must be called during component initialisation.
		*/
		getOr(fallback) {
			const context = getContext(this.#key);
			if (context === void 0) return fallback;
			return context;
		}
		/**
		* Associates the given value with the current component and returns it.
		*
		* Must be called during component initialisation.
		*/
		set(context) {
			return setContext(this.#key, context);
		}
	};
	function useDebounce(callback, wait) {
		let context = null;
		const wait$ = derived(() => extract(wait, 250));
		function debounced(...args) {
			if (context) {
				if (context.timeout) clearTimeout(context.timeout);
			} else {
				let resolve;
				let reject;
				context = {
					timeout: null,
					runner: null,
					promise: new Promise((res, rej) => {
						resolve = res;
						reject = rej;
					}),
					resolve,
					reject
				};
			}
			context.runner = async () => {
				if (!context) return;
				const ctx = context;
				context = null;
				try {
					ctx.resolve(await callback.apply(this, args));
				} catch (error) {
					ctx.reject(error);
				}
			};
			context.timeout = setTimeout(context.runner, wait$());
			return context.promise;
		}
		debounced.cancel = async () => {
			if (!context || context.timeout === null) {
				await new Promise((resolve) => setTimeout(resolve, 0));
				if (!context || context.timeout === null) return;
			}
			clearTimeout(context.timeout);
			context.reject("Cancelled");
			context = null;
		};
		debounced.runScheduledNow = async () => {
			if (!context || !context.timeout) {
				await new Promise((resolve) => setTimeout(resolve, 0));
				if (!context || !context.timeout) return;
			}
			clearTimeout(context.timeout);
			context.timeout = null;
			await context.runner?.();
		};
		Object.defineProperty(debounced, "pending", {
			enumerable: true,
			get() {
				return !!context?.timeout;
			}
		});
		return debounced;
	}
	function runWatcher(sources, flush, effect, options = {}) {
		const { lazy = false } = options;
	}
	function watch(sources, effect, options) {
		runWatcher(sources, "post", effect, options);
	}
	function watchPre(sources, effect, options) {
		runWatcher(sources, "pre", effect, options);
	}
	watch.pre = watchPre;
	function watchOnce(source, effect) {}
	function watchOncePre(source, effect) {}
	watchOnce.pre = watchOncePre;
	var IsMounted = class {
		#isMounted = false;
		constructor() {}
		get current() {
			return this.#isMounted;
		}
	};
	function debounce$1(fn, delay) {
		let timeoutId;
		let lastResolve = null;
		return (...args) => {
			return new Promise((resolve) => {
				if (lastResolve) lastResolve(void 0);
				lastResolve = resolve;
				clearTimeout(timeoutId);
				timeoutId = setTimeout(async () => {
					const result = await fn(...args);
					if (lastResolve) {
						lastResolve(result);
						lastResolve = null;
					}
				}, delay);
			});
		};
	}
	function throttle(fn, delay) {
		let lastRun = 0;
		let lastPromise = null;
		return (...args) => {
			const now = Date.now();
			if (lastRun && now - lastRun < delay) return lastPromise ?? Promise.resolve(void 0);
			lastRun = now;
			lastPromise = fn(...args);
			return lastPromise;
		};
	}
	function runResource(source, fetcher, options = {}, effectFn) {
		const { lazy = false, once = false, initialValue, debounce: debounceTime, throttle: throttleTime } = options;
		let current = initialValue;
		let loading = false;
		let error = void 0;
		let cleanupFns = [];
		const runCleanup = () => {
			cleanupFns.forEach((fn) => fn());
			cleanupFns = [];
		};
		const onCleanup = (fn) => {
			cleanupFns = [...cleanupFns, fn];
		};
		const baseFetcher = async (value, previousValue, refetching = false) => {
			try {
				loading = true;
				error = void 0;
				runCleanup();
				const controller = new AbortController();
				onCleanup(() => controller.abort());
				const result = await fetcher(value, previousValue, {
					data: current,
					refetching,
					onCleanup,
					signal: controller.signal
				});
				current = result;
				return result;
			} catch (e) {
				if (!(e instanceof DOMException && e.name === "AbortError")) error = e;
				return;
			} finally {
				loading = false;
			}
		};
		const runFetcher = debounceTime ? debounce$1(baseFetcher, debounceTime) : throttleTime ? throttle(baseFetcher, throttleTime) : baseFetcher;
		const sources = Array.isArray(source) ? source : [source];
		let prevValues;
		effectFn((values, previousValues) => {
			if (once && prevValues) return;
			prevValues = values;
			runFetcher(Array.isArray(source) ? values : values[0], Array.isArray(source) ? previousValues : previousValues?.[0]);
		}, { lazy });
		return {
			get current() {
				return current;
			},
			get loading() {
				return loading;
			},
			get error() {
				return error;
			},
			mutate: (value) => {
				current = value;
			},
			refetch: (info) => {
				const values = sources.map((s) => s());
				return runFetcher(Array.isArray(source) ? values : values[0], Array.isArray(source) ? values : values[0], info ?? true);
			}
		};
	}
	function resource(source, fetcher, options) {
		return runResource(source, fetcher, options, (fn, options) => {
			const sources = Array.isArray(source) ? source : [source];
			const getters = () => sources.map((s) => s());
			watch(getters, (values, previousValues) => {
				fn(values, previousValues ?? []);
			}, options);
		});
	}
	function resourcePre(source, fetcher, options) {
		return runResource(source, fetcher, options, (fn, options) => {
			const sources = Array.isArray(source) ? source : [source];
			const getter = () => sources.map((s) => s());
			watch.pre(getter, (values, previousValues) => {
				fn(values, previousValues ?? []);
			}, options);
		});
	}
	resource.pre = resourcePre;
	/**
	* A utility function that executes a callback after a specified number of milliseconds.
	*/
	function afterSleep(ms, cb) {
		return setTimeout(cb, ms);
	}
	function afterTick(fn) {
		(/* @__PURE__ */ tick()).then(fn);
	}
	var ELEMENT_NODE = 1;
	var DOCUMENT_NODE = 9;
	var DOCUMENT_FRAGMENT_NODE = 11;
	function isHTMLElement$1(node) {
		return isObject(node) && node.nodeType === ELEMENT_NODE && typeof node.nodeName === "string";
	}
	function isDocument(node) {
		return isObject(node) && node.nodeType === DOCUMENT_NODE;
	}
	function isWindow(node) {
		return isObject(node) && node.constructor?.name === "VisualViewport";
	}
	function isNode(node) {
		return isObject(node) && node.nodeType !== void 0;
	}
	function isShadowRoot(node) {
		return isNode(node) && node.nodeType === DOCUMENT_FRAGMENT_NODE && "host" in node;
	}
	function contains(parent, child) {
		if (!parent || !child) return false;
		if (!isHTMLElement$1(parent) || !isHTMLElement$1(child)) return false;
		const rootNode = child.getRootNode?.();
		if (parent === child) return true;
		if (parent.contains(child)) return true;
		if (rootNode && isShadowRoot(rootNode)) {
			let next = child;
			while (next) {
				if (parent === next) return true;
				next = next.parentNode || next.host;
			}
		}
		return false;
	}
	function getDocument(node) {
		if (isDocument(node)) return node;
		if (isWindow(node)) return node.document;
		return node?.ownerDocument ?? document;
	}
	function getActiveElement(rootNode) {
		let activeElement = rootNode.activeElement;
		while (activeElement?.shadowRoot) {
			const el = activeElement.shadowRoot.activeElement;
			if (el === activeElement) break;
			else activeElement = el;
		}
		return activeElement;
	}
	var DOMContext = class {
		element;
		#root = derived(() => {
			if (!this.element.current) return document;
			return this.element.current.getRootNode() ?? document;
		});
		get root() {
			return this.#root();
		}
		set root($$value) {
			return this.#root($$value);
		}
		constructor(element) {
			if (typeof element === "function") this.element = boxWith(element);
			else this.element = element;
		}
		getDocument = () => {
			return getDocument(this.root);
		};
		getWindow = () => {
			return this.getDocument().defaultView ?? window;
		};
		getActiveElement = () => {
			return getActiveElement(this.root);
		};
		isActiveElement = (node) => {
			return node === this.getActiveElement();
		};
		getElementById(id) {
			return this.root.getElementById(id);
		}
		querySelector = (selector) => {
			if (!this.root) return null;
			return this.root.querySelector(selector);
		};
		querySelectorAll = (selector) => {
			if (!this.root) return [];
			return this.root.querySelectorAll(selector);
		};
		setTimeout = (callback, delay) => {
			return this.getWindow().setTimeout(callback, delay);
		};
		clearTimeout = (timeoutId) => {
			return this.getWindow().clearTimeout(timeoutId);
		};
	};
	globalThis?.window?.trustedTypes;
	EFFECT_TRANSPARENT | EFFECT_PRESERVED;
	/**
	* Creates an object key that will be recognised as an attachment when the object is spread onto an element,
	* as a programmatic alternative to using `{@attach ...}`. This can be useful for library authors, though
	* is generally not needed when building an app.
	*
	* ```svelte
	* <script>
	* 	import { createAttachmentKey } from 'svelte/attachments';
	*
	* 	const props = {
	* 		class: 'cool',
	* 		onclick: () => alert('clicked'),
	* 		[createAttachmentKey()]: (node) => {
	* 			node.textContent = 'attached!';
	* 		}
	* 	};
	* <\/script>
	*
	* <button {...props}>click me</button>
	* ```
	* @since 5.29
	*/
	function createAttachmentKey() {
		return Symbol(ATTACHMENT_KEY);
	}
	/**
	* Creates a Svelte Attachment that attaches a DOM element to a ref.
	* The ref can be either a WritableBox or a callback function.
	*
	* @param ref - Either a WritableBox to store the element in, or a callback function that receives the element
	* @param onChange - Optional callback that fires when the ref changes
	* @returns An object with a spreadable attachment key that should be spread onto the element
	*
	* @example
	* // Using with WritableBox
	* const ref = box<HTMLDivElement | null>(null);
	* <div {...attachRef(ref)}>Content</div>
	*
	* @example
	* // Using with callback
	* <div {...attachRef((node) => myNode = node)}>Content</div>
	*
	* @example
	* // Using with onChange
	* <div {...attachRef(ref, (node) => console.log(node))}>Content</div>
	*/
	function attachRef(ref, onChange) {
		return { [createAttachmentKey()]: (node) => {
			if (isBox(ref)) {
				ref.current = node;
				run(() => onChange?.(node));
				return () => {
					if ("isConnected" in node && node.isConnected) return;
					ref.current = null;
					onChange?.(null);
				};
			}
			ref(node);
			run(() => onChange?.(node));
			return () => {
				if ("isConnected" in node && node.isConnected) return;
				ref(null);
				onChange?.(null);
			};
		} };
	}
	function boolToStr(condition) {
		return condition ? "true" : "false";
	}
	function boolToEmptyStrOrUndef(condition) {
		return condition ? "" : void 0;
	}
	function getDataOpenClosed(condition) {
		return condition ? "open" : "closed";
	}
	function getDataTransitionAttrs(state) {
		if (state === "starting") return { "data-starting-style": "" };
		if (state === "ending") return { "data-ending-style": "" };
		return {};
	}
	var BitsAttrs = class {
		#variant;
		#prefix;
		attrs;
		constructor(config) {
			this.#variant = config.getVariant ? config.getVariant() : null;
			this.#prefix = this.#variant ? `data-${this.#variant}-` : `data-${config.component}-`;
			this.getAttr = this.getAttr.bind(this);
			this.selector = this.selector.bind(this);
			this.attrs = Object.fromEntries(config.parts.map((part) => [part, this.getAttr(part)]));
		}
		getAttr(part, variantOverride) {
			if (variantOverride) return `data-${variantOverride}-${part}`;
			return `${this.#prefix}${part}`;
		}
		selector(part, variantOverride) {
			return `[${this.getAttr(part, variantOverride)}]`;
		}
	};
	function createBitsAttrs(config) {
		const bitsAttrs = new BitsAttrs(config);
		return {
			...bitsAttrs.attrs,
			selector: bitsAttrs.selector,
			getAttr: bitsAttrs.getAttr
		};
	}
	var isBrowser = typeof document !== "undefined";
	var isIOS = getIsIOS();
	function getIsIOS() {
		return isBrowser && window?.navigator?.userAgent && (/iP(ad|hone|od)/.test(window.navigator.userAgent) || window?.navigator?.maxTouchPoints > 2 && /iPad|Macintosh/.test(window?.navigator.userAgent));
	}
	function isHTMLElement(element) {
		return element instanceof HTMLElement;
	}
	function isElementOrSVGElement(element) {
		return element instanceof Element || element instanceof SVGElement;
	}
	var AnimationsComplete = class {
		#opts;
		#currentFrame = null;
		#observer = null;
		#runId = 0;
		constructor(opts) {
			this.#opts = opts;
		}
		#cleanup() {
			if (this.#currentFrame !== null) {
				window.cancelAnimationFrame(this.#currentFrame);
				this.#currentFrame = null;
			}
			this.#observer?.disconnect();
			this.#observer = null;
			this.#runId++;
		}
		run(fn) {
			this.#cleanup();
			const node = this.#opts.ref.current;
			if (!node) return;
			if (typeof node.getAnimations !== "function") {
				this.#executeCallback(fn);
				return;
			}
			const runId = this.#runId;
			const executeIfCurrent = () => {
				if (runId !== this.#runId) return;
				this.#executeCallback(fn);
			};
			const waitForAnimations = () => {
				if (runId !== this.#runId) return;
				const animations = node.getAnimations();
				if (animations.length === 0) {
					executeIfCurrent();
					return;
				}
				Promise.all(animations.map((animation) => animation.finished)).then(() => {
					executeIfCurrent();
				}).catch(() => {
					if (runId !== this.#runId) return;
					if (node.getAnimations().some((animation) => animation.pending || animation.playState !== "finished")) {
						waitForAnimations();
						return;
					}
					executeIfCurrent();
				});
			};
			const requestWaitForAnimations = () => {
				this.#currentFrame = window.requestAnimationFrame(() => {
					this.#currentFrame = null;
					waitForAnimations();
				});
			};
			if (!this.#opts.afterTick.current) {
				requestWaitForAnimations();
				return;
			}
			this.#currentFrame = window.requestAnimationFrame(() => {
				this.#currentFrame = null;
				const startingStyleAttr = "data-starting-style";
				if (!node.hasAttribute(startingStyleAttr)) {
					requestWaitForAnimations();
					return;
				}
				this.#observer = new MutationObserver(() => {
					if (runId !== this.#runId) return;
					if (node.hasAttribute(startingStyleAttr)) return;
					this.#observer?.disconnect();
					this.#observer = null;
					requestWaitForAnimations();
				});
				this.#observer.observe(node, {
					attributes: true,
					attributeFilter: [startingStyleAttr]
				});
			});
		}
		#executeCallback(fn) {
			const execute = () => {
				fn();
			};
			if (this.#opts.afterTick) afterTick(execute);
			else execute();
		}
	};
	var PresenceManager = class {
		#opts;
		#enabled;
		#afterAnimations;
		#shouldRender = false;
		#transitionStatus = void 0;
		#hasMounted = false;
		#transitionFrame = null;
		constructor(opts) {
			this.#opts = opts;
			this.#shouldRender = opts.open.current;
			this.#enabled = opts.enabled ?? true;
			this.#afterAnimations = new AnimationsComplete({
				ref: this.#opts.ref,
				afterTick: this.#opts.open
			});
			watch(() => this.#opts.open.current, (isOpen) => {
				if (!this.#hasMounted) {
					this.#hasMounted = true;
					return;
				}
				this.#clearTransitionFrame();
				if (!isOpen && this.#opts.shouldSkipExitAnimation?.()) {
					this.#shouldRender = false;
					this.#transitionStatus = void 0;
					this.#opts.onComplete?.();
					return;
				}
				if (isOpen) this.#shouldRender = true;
				this.#transitionStatus = isOpen ? "starting" : "ending";
				if (isOpen) this.#transitionFrame = window.requestAnimationFrame(() => {
					this.#transitionFrame = null;
					if (this.#opts.open.current) this.#transitionStatus = void 0;
				});
				if (!this.#enabled) {
					if (!isOpen) this.#shouldRender = false;
					this.#transitionStatus = void 0;
					this.#opts.onComplete?.();
					return;
				}
				this.#afterAnimations.run(() => {
					if (isOpen === this.#opts.open.current) {
						if (!this.#opts.open.current) this.#shouldRender = false;
						this.#transitionStatus = void 0;
						this.#opts.onComplete?.();
					}
				});
			});
		}
		get shouldRender() {
			return this.#shouldRender;
		}
		get transitionStatus() {
			return this.#transitionStatus;
		}
		#clearTransitionFrame() {
			if (this.#transitionFrame === null) return;
			window.cancelAnimationFrame(this.#transitionFrame);
			this.#transitionFrame = null;
		}
	};
	/**
	* A no operation function (does nothing)
	*/
	function noop() {}
	function createId(prefixOrUid, uid) {
		if (uid === void 0) return `bits-${prefixOrUid}`;
		return `bits-${prefixOrUid}-${uid}`;
	}
	var dialogAttrs = createBitsAttrs({
		component: "dialog",
		parts: [
			"content",
			"trigger",
			"overlay",
			"title",
			"description",
			"close",
			"cancel",
			"action"
		]
	});
	var DialogRootContext = new Context("Dialog.Root | AlertDialog.Root");
	var DialogRootState = class DialogRootState {
		static create(opts) {
			const parent = DialogRootContext.getOr(null);
			return DialogRootContext.set(new DialogRootState(opts, parent));
		}
		opts;
		triggerNode = null;
		contentNode = null;
		overlayNode = null;
		descriptionNode = null;
		contentId = void 0;
		titleId = void 0;
		triggerId = void 0;
		descriptionId = void 0;
		cancelNode = null;
		nestedOpenCount = 0;
		depth;
		parent;
		contentPresence;
		overlayPresence;
		constructor(opts, parent) {
			this.opts = opts;
			this.parent = parent;
			this.depth = parent ? parent.depth + 1 : 0;
			this.handleOpen = this.handleOpen.bind(this);
			this.handleClose = this.handleClose.bind(this);
			this.contentPresence = new PresenceManager({
				ref: boxWith(() => this.contentNode),
				open: this.opts.open,
				enabled: true,
				onComplete: () => {
					this.opts.onOpenChangeComplete.current(this.opts.open.current);
				}
			});
			this.overlayPresence = new PresenceManager({
				ref: boxWith(() => this.overlayNode),
				open: this.opts.open,
				enabled: true
			});
			watch(() => this.opts.open.current, (isOpen) => {
				if (!this.parent) return;
				if (isOpen) this.parent.incrementNested();
				else this.parent.decrementNested();
			}, { lazy: true });
		}
		handleOpen() {
			if (this.opts.open.current) return;
			this.opts.open.current = true;
		}
		handleClose() {
			if (!this.opts.open.current) return;
			this.opts.open.current = false;
		}
		getBitsAttr = (part) => {
			return dialogAttrs.getAttr(part, this.opts.variant.current);
		};
		incrementNested() {
			this.nestedOpenCount++;
			this.parent?.incrementNested();
		}
		decrementNested() {
			if (this.nestedOpenCount === 0) return;
			this.nestedOpenCount--;
			this.parent?.decrementNested();
		}
		#sharedProps = derived(() => ({ "data-state": getDataOpenClosed(this.opts.open.current) }));
		get sharedProps() {
			return this.#sharedProps();
		}
		set sharedProps($$value) {
			return this.#sharedProps($$value);
		}
	};
	var DialogTriggerState = class DialogTriggerState {
		static create(opts) {
			return new DialogTriggerState(opts, DialogRootContext.get());
		}
		opts;
		root;
		attachment;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.attachment = attachRef(this.opts.ref, (v) => {
				this.root.triggerNode = v;
				this.root.triggerId = v?.id;
			});
			this.onclick = this.onclick.bind(this);
			this.onkeydown = this.onkeydown.bind(this);
		}
		onclick(e) {
			if (this.opts.disabled.current) return;
			if (e.button > 0) return;
			this.root.handleOpen();
		}
		onkeydown(e) {
			if (this.opts.disabled.current) return;
			if (e.key === " " || e.key === "Enter") {
				e.preventDefault();
				this.root.handleOpen();
			}
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			"aria-haspopup": "dialog",
			"aria-expanded": boolToStr(this.root.opts.open.current),
			"aria-controls": this.root.contentId,
			[this.root.getBitsAttr("trigger")]: "",
			onkeydown: this.onkeydown,
			onclick: this.onclick,
			disabled: this.opts.disabled.current ? true : void 0,
			...this.root.sharedProps,
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var DialogCloseState = class DialogCloseState {
		static create(opts) {
			return new DialogCloseState(opts, DialogRootContext.get());
		}
		opts;
		root;
		attachment;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.attachment = attachRef(this.opts.ref);
			this.onclick = this.onclick.bind(this);
			this.onkeydown = this.onkeydown.bind(this);
		}
		onclick(e) {
			if (this.opts.disabled.current) return;
			if (e.button > 0) return;
			this.root.handleClose();
		}
		onkeydown(e) {
			if (this.opts.disabled.current) return;
			if (e.key === " " || e.key === "Enter") {
				e.preventDefault();
				this.root.handleClose();
			}
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			[this.root.getBitsAttr(this.opts.variant.current)]: "",
			onclick: this.onclick,
			onkeydown: this.onkeydown,
			disabled: this.opts.disabled.current ? true : void 0,
			tabindex: 0,
			...this.root.sharedProps,
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var DialogTitleState = class DialogTitleState {
		static create(opts) {
			return new DialogTitleState(opts, DialogRootContext.get());
		}
		opts;
		root;
		attachment;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.root.titleId = this.opts.id.current;
			this.attachment = attachRef(this.opts.ref);
			watch.pre(() => this.opts.id.current, (id) => {
				this.root.titleId = id;
			});
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			role: "heading",
			"aria-level": this.opts.level.current,
			[this.root.getBitsAttr("title")]: "",
			...this.root.sharedProps,
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var DialogDescriptionState = class DialogDescriptionState {
		static create(opts) {
			return new DialogDescriptionState(opts, DialogRootContext.get());
		}
		opts;
		root;
		attachment;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.root.descriptionId = this.opts.id.current;
			this.attachment = attachRef(this.opts.ref, (v) => {
				this.root.descriptionNode = v;
			});
			watch.pre(() => this.opts.id.current, (id) => {
				this.root.descriptionId = id;
			});
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			[this.root.getBitsAttr("description")]: "",
			...this.root.sharedProps,
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var DialogContentState = class DialogContentState {
		static create(opts) {
			return new DialogContentState(opts, DialogRootContext.get());
		}
		opts;
		root;
		attachment;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.attachment = attachRef(this.opts.ref, (v) => {
				this.root.contentNode = v;
				this.root.contentId = v?.id;
			});
		}
		#snippetProps = derived(() => ({ open: this.root.opts.open.current }));
		get snippetProps() {
			return this.#snippetProps();
		}
		set snippetProps($$value) {
			return this.#snippetProps($$value);
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			role: this.root.opts.variant.current === "alert-dialog" ? "alertdialog" : "dialog",
			"aria-modal": "true",
			"aria-describedby": this.root.descriptionId,
			"aria-labelledby": this.root.titleId,
			[this.root.getBitsAttr("content")]: "",
			style: {
				pointerEvents: "auto",
				outline: this.root.opts.variant.current === "alert-dialog" ? "none" : void 0,
				"--bits-dialog-depth": this.root.depth,
				"--bits-dialog-nested-count": this.root.nestedOpenCount,
				contain: "layout style"
			},
			tabindex: this.root.opts.variant.current === "alert-dialog" ? -1 : void 0,
			"data-nested-open": boolToEmptyStrOrUndef(this.root.nestedOpenCount > 0),
			"data-nested": boolToEmptyStrOrUndef(this.root.parent !== null),
			...getDataTransitionAttrs(this.root.contentPresence.transitionStatus),
			...this.root.sharedProps,
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
		get shouldRender() {
			return this.root.contentPresence.shouldRender;
		}
	};
	function Dialog_title($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { id = createId(uid), ref = null, child, children, level = 2, $$slots, $$events, ...restProps } = $$props;
			const titleState = DialogTitleState.create({
				id: boxWith(() => id),
				level: boxWith(() => level),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const mergedProps = derived(() => mergeProps(restProps, titleState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><div${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></div>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	/**
	* Creates a typed event dispatcher and listener pair for custom events
	* @template T - The type of data that will be passed in the event detail
	* @param eventName - The name of the custom event
	* @param options - CustomEvent options (bubbles, cancelable, etc.)
	*/
	var CustomEventDispatcher = class {
		eventName;
		options;
		constructor(eventName, options = {
			bubbles: true,
			cancelable: true
		}) {
			this.eventName = eventName;
			this.options = options;
		}
		createEvent(detail) {
			return new CustomEvent(this.eventName, {
				...this.options,
				detail
			});
		}
		dispatch(element, detail) {
			const event = this.createEvent(detail);
			element.dispatchEvent(event);
			return event;
		}
		listen(element, callback, options) {
			const handler = (event) => {
				callback(event);
			};
			return on(element, this.eventName, handler, options);
		}
	};
	function debounce(fn, wait = 500) {
		let timeout = null;
		const debounced = (...args) => {
			if (timeout !== null) clearTimeout(timeout);
			timeout = setTimeout(() => {
				fn(...args);
			}, wait);
		};
		debounced.destroy = () => {
			if (timeout !== null) {
				clearTimeout(timeout);
				timeout = null;
			}
		};
		return debounced;
	}
	function isOrContainsTarget(node, target) {
		return node === target || node.contains(target);
	}
	function getOwnerDocument(el) {
		return el?.ownerDocument ?? document;
	}
	/**
	* Determines if the click event truly occurred outside the content node.
	* This was added to handle password managers and other elements that may be injected
	* into the DOM but visually appear inside the content.
	*/
	function isClickTrulyOutside(event, contentNode) {
		const { clientX, clientY } = event;
		const rect = contentNode.getBoundingClientRect();
		return clientX < rect.left || clientX > rect.right || clientY < rect.top || clientY > rect.bottom;
	}
	/*!
	* tabbable 6.5.0
	* @license MIT, https://github.com/focus-trap/tabbable/blob/master/LICENSE
	*/
	var candidateSelectors = [
		"input:not([inert]):not([inert] *)",
		"select:not([inert]):not([inert] *)",
		"textarea:not([inert]):not([inert] *)",
		"a[href]:not([inert]):not([inert] *)",
		"area[href]:not([inert]):not([inert] *)",
		"button:not([inert]):not([inert] *)",
		"[tabindex]:not(slot):not([inert]):not([inert] *)",
		"audio[controls]:not([inert]):not([inert] *)",
		"video[controls]:not([inert]):not([inert] *)",
		"[contenteditable]:not([contenteditable=\"false\"]):not([inert]):not([inert] *)",
		"details>summary:first-of-type:not([inert]):not([inert] *)",
		"details:not([inert]):not([inert] *)"
	];
	var candidateSelector = /* #__PURE__ */ candidateSelectors.join(",");
	var NoElement = typeof Element === "undefined";
	var matches = NoElement ? function() {} : Element.prototype.matches || Element.prototype.msMatchesSelector || Element.prototype.webkitMatchesSelector;
	var getRootNode = !NoElement && Element.prototype.getRootNode ? function(element) {
		var _element$getRootNode;
		return element === null || element === void 0 ? void 0 : (_element$getRootNode = element.getRootNode) === null || _element$getRootNode === void 0 ? void 0 : _element$getRootNode.call(element);
	} : function(element) {
		return element === null || element === void 0 ? void 0 : element.ownerDocument;
	};
	/**
	* Determines if a node is inert or in an inert ancestor.
	* @param {Node} [node]
	* @param {boolean} [lookUp] If true and `node` is not inert, looks up at ancestors to
	*  see if any of them are inert. If false, only `node` itself is considered.
	* @returns {boolean} True if inert itself or by way of being in an inert ancestor.
	*  False if `node` is falsy.
	*/
	var _isInert = function isInert(node, lookUp) {
		var _node$getAttribute;
		if (lookUp === void 0) lookUp = true;
		var inertAtt = node === null || node === void 0 ? void 0 : (_node$getAttribute = node.getAttribute) === null || _node$getAttribute === void 0 ? void 0 : _node$getAttribute.call(node, "inert");
		return inertAtt === "" || inertAtt === "true" || lookUp && node && (typeof node.closest === "function" ? node.closest("[inert]") : _isInert(node.parentNode));
	};
	/**
	* Determines if a node's content is editable.
	* @param {Element} [node]
	* @returns True if it's content-editable; false if it's not or `node` is falsy.
	*/
	var isContentEditable = function isContentEditable(node) {
		var _node$getAttribute2;
		var attValue = node === null || node === void 0 ? void 0 : (_node$getAttribute2 = node.getAttribute) === null || _node$getAttribute2 === void 0 ? void 0 : _node$getAttribute2.call(node, "contenteditable");
		return attValue === "" || attValue === "true";
	};
	/**
	* @param {Element} el container to check in
	* @param {boolean} includeContainer add container to check
	* @param {(node: Element) => boolean} filter filter candidates
	* @returns {Element[]}
	*/
	var getCandidates = function getCandidates(el, includeContainer, filter) {
		if (_isInert(el)) return [];
		var candidates = Array.prototype.slice.apply(el.querySelectorAll(candidateSelector));
		if (includeContainer && matches.call(el, candidateSelector)) candidates.unshift(el);
		candidates = candidates.filter(filter);
		return candidates;
	};
	/**
	* @callback GetShadowRoot
	* @param {Element} element to check for shadow root
	* @returns {ShadowRoot|boolean} ShadowRoot if available or boolean indicating if a shadowRoot is attached but not available.
	*/
	/**
	* @callback ShadowRootFilter
	* @param {Element} shadowHostNode the element which contains shadow content
	* @returns {boolean} true if a shadow root could potentially contain valid candidates.
	*/
	/**
	* @typedef {Object} CandidateScope
	* @property {Element} scopeParent contains inner candidates
	* @property {Element[]} candidates list of candidates found in the scope parent
	*/
	/**
	* @typedef {Object} IterativeOptions
	* @property {GetShadowRoot|boolean} getShadowRoot true if shadow support is enabled; falsy if not;
	*  if a function, implies shadow support is enabled and either returns the shadow root of an element
	*  or a boolean stating if it has an undisclosed shadow root
	* @property {(node: Element) => boolean} filter filter candidates
	* @property {boolean} flatten if true then result will flatten any CandidateScope into the returned list
	* @property {ShadowRootFilter} shadowRootFilter filter shadow roots;
	*/
	/**
	* @param {Element[]} elements list of element containers to match candidates from
	* @param {boolean} includeContainer add container list to check
	* @param {IterativeOptions} options
	* @returns {Array.<Element|CandidateScope>}
	*/
	var _getCandidatesIteratively = function getCandidatesIteratively(elements, includeContainer, options) {
		var candidates = [];
		var elementsToCheck = Array.from(elements);
		while (elementsToCheck.length) {
			var element = elementsToCheck.shift();
			if (_isInert(element, false)) continue;
			if (element.tagName === "SLOT") {
				var assigned = element.assignedElements();
				var nestedCandidates = _getCandidatesIteratively(assigned.length ? assigned : element.children, true, options);
				if (options.flatten) candidates.push.apply(candidates, nestedCandidates);
				else candidates.push({
					scopeParent: element,
					candidates: nestedCandidates
				});
			} else {
				if (matches.call(element, candidateSelector) && options.filter(element) && (includeContainer || !elements.includes(element))) candidates.push(element);
				var shadowRoot = element.shadowRoot || typeof options.getShadowRoot === "function" && options.getShadowRoot(element);
				var validShadowRoot = !_isInert(shadowRoot, false) && (!options.shadowRootFilter || options.shadowRootFilter(element));
				if (shadowRoot && validShadowRoot) {
					var _nestedCandidates = _getCandidatesIteratively(shadowRoot === true ? element.children : shadowRoot.children, true, options);
					if (options.flatten) candidates.push.apply(candidates, _nestedCandidates);
					else candidates.push({
						scopeParent: element,
						candidates: _nestedCandidates
					});
				} else elementsToCheck.unshift.apply(elementsToCheck, element.children);
			}
		}
		return candidates;
	};
	/**
	* @private
	* Determines if the node has an explicitly specified `tabindex` attribute.
	* @param {HTMLElement} node
	* @returns {boolean} True if so; false if not.
	*/
	var hasTabIndex = function hasTabIndex(node) {
		return !isNaN(parseInt(node.getAttribute("tabindex"), 10));
	};
	/**
	* Determine the tab index of a given node.
	* @param {HTMLElement} node
	* @returns {number} Tab order (negative, 0, or positive number).
	* @throws {Error} If `node` is falsy.
	*/
	var getTabIndex = function getTabIndex(node) {
		if (!node) throw new Error("No node provided");
		if (node.tabIndex < 0) {
			if ((/^(AUDIO|VIDEO|DETAILS)$/.test(node.tagName) || isContentEditable(node)) && !hasTabIndex(node)) return 0;
		}
		return node.tabIndex;
	};
	/**
	* Determine the tab index of a given node __for sort order purposes__.
	* @param {HTMLElement} node
	* @param {boolean} [isScope] True for a custom element with shadow root or slot that, by default,
	*  has tabIndex -1, but needs to be sorted by document order in order for its content to be
	*  inserted into the correct sort position.
	* @returns {number} Tab order (negative, 0, or positive number).
	*/
	var getSortOrderTabIndex = function getSortOrderTabIndex(node, isScope) {
		var tabIndex = getTabIndex(node);
		if (tabIndex < 0 && isScope && !hasTabIndex(node)) return 0;
		return tabIndex;
	};
	var sortOrderedTabbables = function sortOrderedTabbables(a, b) {
		return a.tabIndex === b.tabIndex ? a.documentOrder - b.documentOrder : a.tabIndex - b.tabIndex;
	};
	var isInput = function isInput(node) {
		return node.tagName === "INPUT";
	};
	var isHiddenInput = function isHiddenInput(node) {
		return isInput(node) && node.type === "hidden";
	};
	var isDetailsWithSummary = function isDetailsWithSummary(node) {
		return node.tagName === "DETAILS" && Array.prototype.slice.apply(node.children).some(function(child) {
			return child.tagName === "SUMMARY";
		});
	};
	var getCheckedRadio = function getCheckedRadio(nodes, form) {
		for (var i = 0; i < nodes.length; i++) if (nodes[i].checked && nodes[i].form === form) return nodes[i];
	};
	var isTabbableRadio = function isTabbableRadio(node) {
		if (!node.name) return true;
		var radioScope = node.form || getRootNode(node);
		var queryRadios = function queryRadios(name) {
			return radioScope.querySelectorAll("input[type=\"radio\"][name=\"" + name + "\"]");
		};
		var radioSet;
		if (typeof window !== "undefined" && typeof window.CSS !== "undefined" && typeof window.CSS.escape === "function") radioSet = queryRadios(window.CSS.escape(node.name));
		else try {
			radioSet = queryRadios(node.name);
		} catch (err) {
			console.error("Looks like you have a radio button with a name attribute containing invalid CSS selector characters and need the CSS.escape polyfill: %s", err.message);
			return false;
		}
		var checked = getCheckedRadio(radioSet, node.form);
		return !checked || checked === node;
	};
	var isRadio = function isRadio(node) {
		return isInput(node) && node.type === "radio";
	};
	var isNonTabbableRadio = function isNonTabbableRadio(node) {
		return isRadio(node) && !isTabbableRadio(node);
	};
	var isNodeAttached = function isNodeAttached(node) {
		var _nodeRoot;
		var nodeRoot = node && getRootNode(node);
		var nodeRootHost = (_nodeRoot = nodeRoot) === null || _nodeRoot === void 0 ? void 0 : _nodeRoot.host;
		var attached = false;
		if (nodeRoot && nodeRoot !== node) {
			var _nodeRootHost, _nodeRootHost$ownerDo, _node$ownerDocument;
			attached = !!((_nodeRootHost = nodeRootHost) !== null && _nodeRootHost !== void 0 && (_nodeRootHost$ownerDo = _nodeRootHost.ownerDocument) !== null && _nodeRootHost$ownerDo !== void 0 && _nodeRootHost$ownerDo.contains(nodeRootHost) || node !== null && node !== void 0 && (_node$ownerDocument = node.ownerDocument) !== null && _node$ownerDocument !== void 0 && _node$ownerDocument.contains(node));
			while (!attached && nodeRootHost) {
				var _nodeRoot2, _nodeRootHost2, _nodeRootHost2$ownerD;
				nodeRoot = getRootNode(nodeRootHost);
				nodeRootHost = (_nodeRoot2 = nodeRoot) === null || _nodeRoot2 === void 0 ? void 0 : _nodeRoot2.host;
				attached = !!((_nodeRootHost2 = nodeRootHost) !== null && _nodeRootHost2 !== void 0 && (_nodeRootHost2$ownerD = _nodeRootHost2.ownerDocument) !== null && _nodeRootHost2$ownerD !== void 0 && _nodeRootHost2$ownerD.contains(nodeRootHost));
			}
		}
		return attached;
	};
	var isZeroArea = function isZeroArea(node) {
		var _node$getBoundingClie = node.getBoundingClientRect(), width = _node$getBoundingClie.width, height = _node$getBoundingClie.height;
		return width === 0 && height === 0;
	};
	var isHidden = function isHidden(node, _ref) {
		var displayCheck = _ref.displayCheck, getShadowRoot = _ref.getShadowRoot;
		if (displayCheck === "full-native") {
			if ("checkVisibility" in node) return !node.checkVisibility({
				checkOpacity: false,
				opacityProperty: false,
				contentVisibilityAuto: true,
				visibilityProperty: true,
				checkVisibilityCSS: true
			});
		}
		var visibility = getComputedStyle(node).visibility;
		if (visibility === "hidden" || visibility === "collapse") return true;
		var nodeUnderDetails = matches.call(node, "details>summary:first-of-type") ? node.parentElement : node;
		if (matches.call(nodeUnderDetails, "details:not([open]) *")) return true;
		if (!displayCheck || displayCheck === "full" || displayCheck === "full-native" || displayCheck === "legacy-full") {
			if (typeof getShadowRoot === "function") {
				var originalNode = node;
				while (node) {
					var parentElement = node.parentElement;
					var rootNode = getRootNode(node);
					if (parentElement && !parentElement.shadowRoot && getShadowRoot(parentElement) === true) return isZeroArea(node);
					else if (node.assignedSlot) node = node.assignedSlot;
					else if (!parentElement && rootNode !== node.ownerDocument) node = rootNode.host;
					else node = parentElement;
				}
				node = originalNode;
			}
			if (isNodeAttached(node)) return !node.getClientRects().length;
			if (displayCheck !== "legacy-full") return true;
		} else if (displayCheck === "non-zero-area") return isZeroArea(node);
		return false;
	};
	var isDisabledFromFieldset = function isDisabledFromFieldset(node) {
		if (/^(INPUT|BUTTON|SELECT|TEXTAREA)$/.test(node.tagName)) {
			var parentNode = node.parentElement;
			while (parentNode) {
				if (parentNode.tagName === "FIELDSET" && parentNode.disabled) {
					for (var i = 0; i < parentNode.children.length; i++) {
						var child = parentNode.children.item(i);
						if (child.tagName === "LEGEND") return matches.call(parentNode, "fieldset[disabled] *") ? true : !child.contains(node);
					}
					return true;
				}
				parentNode = parentNode.parentElement;
			}
		}
		return false;
	};
	var isNodeMatchingSelectorFocusable = function isNodeMatchingSelectorFocusable(options, node) {
		if (node.disabled || isHiddenInput(node) || isHidden(node, options) || isDetailsWithSummary(node) || isDisabledFromFieldset(node)) return false;
		return true;
	};
	var isNodeMatchingSelectorTabbable = function isNodeMatchingSelectorTabbable(options, node) {
		if (isNonTabbableRadio(node) || getTabIndex(node) < 0 || !isNodeMatchingSelectorFocusable(options, node)) return false;
		return true;
	};
	var isShadowRootTabbable = function isShadowRootTabbable(shadowHostNode) {
		var tabIndex = parseInt(shadowHostNode.getAttribute("tabindex"), 10);
		if (isNaN(tabIndex) || tabIndex >= 0) return true;
		return false;
	};
	/**
	* @param {Array.<Element|CandidateScope>} candidates
	* @returns Element[]
	*/
	var _sortByOrder = function sortByOrder(candidates) {
		var regularTabbables = [];
		var orderedTabbables = [];
		candidates.forEach(function(item, i) {
			var isScope = !!item.scopeParent;
			var element = isScope ? item.scopeParent : item;
			var candidateTabindex = getSortOrderTabIndex(element, isScope);
			var elements = isScope ? _sortByOrder(item.candidates) : element;
			if (candidateTabindex === 0) isScope ? regularTabbables.push.apply(regularTabbables, elements) : regularTabbables.push(element);
			else orderedTabbables.push({
				documentOrder: i,
				tabIndex: candidateTabindex,
				item,
				isScope,
				content: elements
			});
		});
		return orderedTabbables.sort(sortOrderedTabbables).reduce(function(acc, sortable) {
			sortable.isScope ? acc.push.apply(acc, sortable.content) : acc.push(sortable.content);
			return acc;
		}, []).concat(regularTabbables);
	};
	var tabbable = function tabbable(container, options) {
		options = options || {};
		var candidates;
		if (options.getShadowRoot) candidates = _getCandidatesIteratively([container], options.includeContainer, {
			filter: isNodeMatchingSelectorTabbable.bind(null, options),
			flatten: false,
			getShadowRoot: options.getShadowRoot,
			shadowRootFilter: isShadowRootTabbable
		});
		else candidates = getCandidates(container, options.includeContainer, isNodeMatchingSelectorTabbable.bind(null, options));
		return _sortByOrder(candidates);
	};
	var focusable = function focusable(container, options) {
		options = options || {};
		var candidates;
		if (options.getShadowRoot) candidates = _getCandidatesIteratively([container], options.includeContainer, {
			filter: isNodeMatchingSelectorFocusable.bind(null, options),
			flatten: true,
			getShadowRoot: options.getShadowRoot
		});
		else candidates = getCandidates(container, options.includeContainer, isNodeMatchingSelectorFocusable.bind(null, options));
		return candidates;
	};
	var focusableCandidateSelector = /* #__PURE__ */ candidateSelectors.concat("iframe:not([inert]):not([inert] *)").join(",");
	var isFocusable = function isFocusable(node, options) {
		options = options || {};
		if (!node) throw new Error("No node provided");
		if (matches.call(node, focusableCandidateSelector) === false) return false;
		return isNodeMatchingSelectorFocusable(options, node);
	};
	var CONTEXT_MENU_TRIGGER_ATTR = "data-context-menu-trigger";
	var CONTEXT_MENU_CONTENT_ATTR = "data-context-menu-content";
	new Context("Menu.Root");
	new Context("Menu.Root | Menu.Sub");
	new Context("Menu.Content");
	new Context("Menu.Group | Menu.RadioGroup");
	new Context("Menu.RadioGroup");
	new Context("Menu.CheckboxGroup");
	new CustomEventDispatcher("bitsmenuopen", {
		bubbles: false,
		cancelable: true
	});
	createBitsAttrs({
		component: "menu",
		parts: [
			"trigger",
			"content",
			"sub-trigger",
			"item",
			"group",
			"group-heading",
			"checkbox-group",
			"checkbox-item",
			"radio-group",
			"radio-item",
			"separator",
			"sub-content",
			"arrow"
		]
	});
	globalThis.bitsDismissableLayers ??= /* @__PURE__ */ new Map();
	var DismissibleLayerState = class DismissibleLayerState {
		static create(opts) {
			return new DismissibleLayerState(opts);
		}
		opts;
		#interactOutsideProp;
		#behaviorType;
		#interceptedEvents = { pointerdown: false };
		#isResponsibleLayer = false;
		#isFocusInsideDOMTree = false;
		#documentObj = void 0;
		#onFocusOutside;
		#unsubClickListener = noop;
		/**
		* Set once the layer is torn down. Deferred work scheduled before teardown can
		* still run afterwards, so every such callback must short-circuit on this
		* before reading `this.opts.ref.current` — reading a destroyed `$derived`
		* triggers Svelte's `derived_inert` warning. A class field rather than a
		* constructor local so the class-field handlers below can see it too.
		*/
		#destroyed = false;
		constructor(opts) {
			this.opts = opts;
			this.#behaviorType = opts.interactOutsideBehavior;
			this.#interactOutsideProp = opts.onInteractOutside;
			this.#onFocusOutside = opts.onFocusOutside;
			let unsubEvents = noop;
			let pendingTimer = null;
			const clearPendingTimer = () => {
				if (pendingTimer != null) {
					clearTimeout(pendingTimer);
					pendingTimer = null;
				}
			};
			const cleanup = () => {
				clearPendingTimer();
				this.#resetState();
				globalThis.bitsDismissableLayers.delete(this);
				this.#handleInteractOutside.destroy();
				unsubEvents();
			};
			watch([() => this.opts.enabled.current, () => this.opts.ref.current], () => {
				if (!this.opts.enabled.current || !this.opts.ref.current) return;
				clearPendingTimer();
				pendingTimer = afterSleep(1, () => {
					pendingTimer = null;
					if (this.#destroyed || !this.opts.ref.current) return;
					globalThis.bitsDismissableLayers.set(this, this.#behaviorType);
					unsubEvents();
					unsubEvents = this.#addEventListeners();
				});
				return cleanup;
			});
		}
		#handleFocus = (event) => {
			if (event.defaultPrevented) return;
			if (this.#destroyed || !this.opts.ref.current) return;
			afterTick(() => {
				if (this.#destroyed) return;
				if (!this.opts.ref.current || this.#isTargetWithinLayer(event.target)) return;
				if (event.target && !this.#isFocusInsideDOMTree) this.#onFocusOutside.current?.(event);
			});
		};
		#addEventListeners() {
			return executeCallbacks(
				/**
				* CAPTURE INTERACTION START
				* mark interaction-start event as intercepted.
				* mark responsible layer during interaction start
				* to avoid checking if is responsible layer during interaction end
				* when a new floating element may have been opened.
				*/
				on(this.#documentObj, "pointerdown", executeCallbacks(this.#markInterceptedEvent, this.#markResponsibleLayer), { capture: true }),
				/**
				* BUBBLE INTERACTION START
				* Mark interaction-start event as non-intercepted. Debounce `onInteractOutsideStart`
				* to avoid prematurely checking if other events were intercepted.
				*/
				on(this.#documentObj, "pointerdown", executeCallbacks(this.#markNonInterceptedEvent, this.#handleInteractOutside)),
				/**
				* HANDLE FOCUS OUTSIDE
				*/
				on(this.#documentObj, "focusin", this.#handleFocus)
			);
		}
		#handleDismiss = (e) => {
			let event = e;
			if (event.defaultPrevented) event = createWrappedEvent(e);
			this.#interactOutsideProp.current(e);
		};
		#handleInteractOutside = debounce((e) => {
			if (!this.opts.ref.current) {
				this.#unsubClickListener();
				return;
			}
			const isEventValid = this.opts.isValidEvent.current(e, this.opts.ref.current) || isValidEvent(e, this.opts.ref.current);
			if (!this.#isResponsibleLayer || this.#isAnyEventIntercepted() || !isEventValid) {
				this.#unsubClickListener();
				return;
			}
			let event = e;
			if (event.defaultPrevented) event = createWrappedEvent(event);
			if (this.#behaviorType.current !== "close" && this.#behaviorType.current !== "defer-otherwise-close") {
				this.#unsubClickListener();
				return;
			}
			if (e.pointerType === "touch") {
				this.#unsubClickListener();
				this.#unsubClickListener = on(this.#documentObj, "click", this.#handleDismiss, { once: true });
			} else this.#interactOutsideProp.current(event);
		}, 10);
		#markInterceptedEvent = (e) => {
			this.#interceptedEvents[e.type] = true;
		};
		#markNonInterceptedEvent = (e) => {
			this.#interceptedEvents[e.type] = false;
		};
		#markResponsibleLayer = () => {
			if (!this.opts.ref.current) return;
			this.#isResponsibleLayer = isResponsibleLayer(this.opts.ref.current);
		};
		#isTargetWithinLayer = (target) => {
			if (!this.opts.ref.current) return false;
			return isOrContainsTarget(this.opts.ref.current, target);
		};
		/**
		* Resets the per-interaction state. Must stay synchronous.
		*
		* This was a `debounce(..., 20)` from when it was also wired to a capture-phase
		* interaction-end listener and had to land after the 10ms `#handleInteractOutside`
		* debounce. That listener is gone, but the debounce stayed on the `cleanup()` path —
		* and because `watch` runs `cleanup()` once per open (`ref` goes null -> node), every
		* layer scheduled a reset 20ms into its own lifetime. An outside `pointerdown` landing
		* 10-20ms after that cleanup would have its `#isResponsibleLayer` flag cleared by the
		* stale reset in the gap before the debounced `#handleInteractOutside` ran, which then
		* bailed and left the layer open. `cleanup()` destroys `#handleInteractOutside` anyway,
		* so nothing is left in flight that needs to observe the pre-reset state.
		*/
		#resetState = () => {
			for (const eventType in this.#interceptedEvents) this.#interceptedEvents[eventType] = false;
			this.#isResponsibleLayer = false;
		};
		#isAnyEventIntercepted() {
			return Object.values(this.#interceptedEvents).some(Boolean);
		}
		#onfocuscapture = () => {
			this.#isFocusInsideDOMTree = true;
		};
		#onblurcapture = () => {
			this.#isFocusInsideDOMTree = false;
		};
		props = {
			onfocuscapture: this.#onfocuscapture,
			onblurcapture: this.#onblurcapture
		};
	};
	function getTopMostDismissableLayer(layersArr = [...globalThis.bitsDismissableLayers]) {
		return layersArr.findLast(([_, { current: behaviorType }]) => behaviorType === "close" || behaviorType === "ignore");
	}
	function isResponsibleLayer(node) {
		const layersArr = [...globalThis.bitsDismissableLayers];
		/**
		* We first check if we can find a top layer with `close` or `ignore`.
		* If that top layer was found and matches the provided node, then the node is
		* responsible for the outside interaction. Otherwise, we know that all layers defer so
		* the first layer is the responsible one.
		*/
		const topMostLayer = getTopMostDismissableLayer(layersArr);
		if (topMostLayer) return topMostLayer[0].opts.ref.current === node;
		const [firstLayerNode] = layersArr[0];
		return firstLayerNode.opts.ref.current === node;
	}
	function isValidEvent(e, node) {
		const target = e.target;
		if (!isElementOrSVGElement(target)) return false;
		const targetIsContextMenuTrigger = Boolean(target.closest(`[${CONTEXT_MENU_TRIGGER_ATTR}]`));
		const nodeIsContextMenu = Boolean(node.closest(`[${CONTEXT_MENU_CONTENT_ATTR}]`));
		if ("button" in e && e.button > 0 && !targetIsContextMenuTrigger) return false;
		if ("button" in e && e.button === 0 && targetIsContextMenuTrigger && nodeIsContextMenu) return true;
		if (targetIsContextMenuTrigger && nodeIsContextMenu) return false;
		return getOwnerDocument(target).documentElement.contains(target) && !isOrContainsTarget(node, target) && isClickTrulyOutside(e, node);
	}
	function createWrappedEvent(e) {
		const capturedCurrentTarget = e.currentTarget;
		const capturedTarget = e.target;
		let newEvent;
		if (e instanceof PointerEvent) newEvent = new PointerEvent(e.type, e);
		else newEvent = new PointerEvent("pointerdown", e);
		let isPrevented = false;
		return new Proxy(newEvent, { get: (target, prop) => {
			if (prop === "currentTarget") return capturedCurrentTarget;
			if (prop === "target") return capturedTarget;
			if (prop === "preventDefault") return () => {
				isPrevented = true;
				if (typeof target.preventDefault === "function") target.preventDefault();
			};
			if (prop === "defaultPrevented") return isPrevented;
			if (prop in target) return target[prop];
			return e[prop];
		} });
	}
	function Dismissible_layer($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { interactOutsideBehavior = "close", onInteractOutside = noop, onFocusOutside = noop, id, children, enabled, isValidEvent = () => false, ref } = $$props;
			const dismissibleLayerState = DismissibleLayerState.create({
				id: boxWith(() => id),
				interactOutsideBehavior: boxWith(() => interactOutsideBehavior),
				onInteractOutside: boxWith(() => onInteractOutside),
				enabled: boxWith(() => enabled),
				onFocusOutside: boxWith(() => onFocusOutside),
				isValidEvent: boxWith(() => isValidEvent),
				ref
			});
			children?.($$renderer, { props: dismissibleLayerState.props });
			$$renderer.push(`<!---->`);
		});
	}
	globalThis.bitsEscapeLayers ??= /* @__PURE__ */ new Map();
	var EscapeLayerState = class EscapeLayerState {
		static create(opts) {
			return new EscapeLayerState(opts);
		}
		opts;
		domContext;
		constructor(opts) {
			this.opts = opts;
			this.domContext = new DOMContext(this.opts.ref);
			let unsubEvents = noop;
			watch(() => opts.enabled.current, (enabled) => {
				if (enabled) {
					globalThis.bitsEscapeLayers.set(this, opts.escapeKeydownBehavior);
					unsubEvents = this.#addEventListener();
				}
				return () => {
					unsubEvents();
					globalThis.bitsEscapeLayers.delete(this);
				};
			});
		}
		#addEventListener = () => {
			return on(this.domContext.getDocument(), "keydown", this.#onkeydown, { passive: false });
		};
		#onkeydown = (e) => {
			if (e.key !== "Escape" || !isResponsibleEscapeLayer(this)) return;
			const clonedEvent = new KeyboardEvent(e.type, e);
			e.preventDefault();
			const behaviorType = this.opts.escapeKeydownBehavior.current;
			if (behaviorType !== "close" && behaviorType !== "defer-otherwise-close") return;
			this.opts.onEscapeKeydown.current(clonedEvent);
		};
	};
	function isResponsibleEscapeLayer(instance) {
		const layersArr = [...globalThis.bitsEscapeLayers];
		/**
		* We first check if we can find a top layer with `close` or `ignore`.
		* If that top layer was found and matches the provided node, then the node is
		* responsible for the escape. Otherwise, we know that all layers defer so
		* the first layer is the responsible one.
		*/
		const topMostLayer = layersArr.findLast(([_, { current: behaviorType }]) => behaviorType === "close" || behaviorType === "ignore");
		if (topMostLayer) return topMostLayer[0] === instance;
		const [firstLayerNode] = layersArr[0];
		return firstLayerNode === instance;
	}
	function Escape_layer($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { escapeKeydownBehavior = "close", onEscapeKeydown = noop, children, enabled, ref } = $$props;
			EscapeLayerState.create({
				escapeKeydownBehavior: boxWith(() => escapeKeydownBehavior),
				onEscapeKeydown: boxWith(() => onEscapeKeydown),
				enabled: boxWith(() => enabled),
				ref
			});
			children?.($$renderer);
			$$renderer.push(`<!---->`);
		});
	}
	var FocusScopeManager = class FocusScopeManager {
		static instance;
		#scopeStack = simpleBox([]);
		#focusHistory = /* @__PURE__ */ new WeakMap();
		#preFocusHistory = /* @__PURE__ */ new WeakMap();
		static getInstance() {
			if (!this.instance) this.instance = new FocusScopeManager();
			return this.instance;
		}
		register(scope) {
			const current = this.getActive();
			if (current && current !== scope) current.pause();
			const activeElement = document.activeElement;
			if (activeElement && activeElement !== document.body) this.#preFocusHistory.set(scope, activeElement);
			this.#scopeStack.current = this.#scopeStack.current.filter((s) => s !== scope);
			this.#scopeStack.current.unshift(scope);
		}
		unregister(scope) {
			this.#scopeStack.current = this.#scopeStack.current.filter((s) => s !== scope);
			const next = this.getActive();
			if (next) next.resume();
		}
		getActive() {
			return this.#scopeStack.current[0];
		}
		setFocusMemory(scope, element) {
			this.#focusHistory.set(scope, element);
		}
		getFocusMemory(scope) {
			return this.#focusHistory.get(scope);
		}
		isActiveScope(scope) {
			return this.getActive() === scope;
		}
		setPreFocusMemory(scope, element) {
			this.#preFocusHistory.set(scope, element);
		}
		getPreFocusMemory(scope) {
			return this.#preFocusHistory.get(scope);
		}
		clearPreFocusMemory(scope) {
			this.#preFocusHistory.delete(scope);
		}
	};
	var FocusScope = class FocusScope {
		#paused = false;
		#container = null;
		#manager = FocusScopeManager.getInstance();
		#cleanupFns = [];
		#opts;
		constructor(opts) {
			this.#opts = opts;
		}
		get paused() {
			return this.#paused;
		}
		pause() {
			this.#paused = true;
		}
		resume() {
			this.#paused = false;
		}
		#cleanup() {
			for (const fn of this.#cleanupFns) fn();
			this.#cleanupFns = [];
		}
		mount(container) {
			if (this.#container) this.unmount();
			this.#container = container;
			this.#manager.register(this);
			this.#setupEventListeners();
			this.#handleOpenAutoFocus();
		}
		unmount() {
			if (!this.#container) return;
			this.#cleanup();
			this.#handleCloseAutoFocus();
			this.#manager.unregister(this);
			this.#manager.clearPreFocusMemory(this);
			this.#container = null;
		}
		#handleOpenAutoFocus() {
			if (!this.#container) return;
			const event = new CustomEvent("focusScope.onOpenAutoFocus", {
				bubbles: false,
				cancelable: true
			});
			this.#opts.onOpenAutoFocus.current(event);
			if (!event.defaultPrevented) requestAnimationFrame(() => {
				if (!this.#container) return;
				const firstTabbable = this.#getFirstTabbable();
				if (firstTabbable) {
					firstTabbable.focus();
					this.#manager.setFocusMemory(this, firstTabbable);
				} else this.#container.focus();
			});
		}
		#handleCloseAutoFocus() {
			const event = new CustomEvent("focusScope.onCloseAutoFocus", {
				bubbles: false,
				cancelable: true
			});
			this.#opts.onCloseAutoFocus.current?.(event);
			if (!event.defaultPrevented) {
				const preFocusedElement = this.#manager.getPreFocusMemory(this);
				if (preFocusedElement && document.contains(preFocusedElement)) try {
					preFocusedElement.focus();
				} catch {
					document.body.focus();
				}
			}
		}
		#setupEventListeners() {
			if (!this.#container || !this.#opts.trap.current) return;
			const container = this.#container;
			const doc = container.ownerDocument;
			const handleFocus = (e) => {
				if (this.#paused || !this.#manager.isActiveScope(this)) return;
				const target = e.target;
				if (!target) return;
				if (container.contains(target)) this.#manager.setFocusMemory(this, target);
				else {
					const lastFocused = this.#manager.getFocusMemory(this);
					if (lastFocused && container.contains(lastFocused) && isFocusable(lastFocused)) {
						e.preventDefault();
						lastFocused.focus();
					} else {
						const firstTabbable = this.#getFirstTabbable();
						const firstFocusable = this.#getAllFocusables()[0];
						(firstTabbable || firstFocusable || container).focus();
					}
				}
			};
			const handleKeydown = (e) => {
				if (!this.#opts.loop || this.#paused || e.key !== "Tab") return;
				if (!this.#manager.isActiveScope(this)) return;
				const tabbables = this.#getTabbables();
				if (tabbables.length === 0) return;
				const first = tabbables[0];
				const last = tabbables[tabbables.length - 1];
				if (!e.shiftKey && doc.activeElement === last) {
					e.preventDefault();
					first.focus();
				} else if (e.shiftKey && doc.activeElement === first) {
					e.preventDefault();
					last.focus();
				}
			};
			this.#cleanupFns.push(on(doc, "focusin", handleFocus, { capture: true }), on(container, "keydown", handleKeydown));
			const observer = new MutationObserver(() => {
				const lastFocused = this.#manager.getFocusMemory(this);
				if (lastFocused && !container.contains(lastFocused)) {
					const firstTabbable = this.#getFirstTabbable();
					const firstFocusable = this.#getAllFocusables()[0];
					const elementToFocus = firstTabbable || firstFocusable;
					if (elementToFocus) {
						elementToFocus.focus();
						this.#manager.setFocusMemory(this, elementToFocus);
					} else container.focus();
				}
			});
			observer.observe(container, {
				childList: true,
				subtree: true
			});
			this.#cleanupFns.push(() => observer.disconnect());
		}
		#getTabbables() {
			if (!this.#container) return [];
			return tabbable(this.#container, {
				includeContainer: false,
				getShadowRoot: true
			});
		}
		#getFirstTabbable() {
			return this.#getTabbables()[0] || null;
		}
		#getAllFocusables() {
			if (!this.#container) return [];
			return focusable(this.#container, {
				includeContainer: false,
				getShadowRoot: true
			});
		}
		static use(opts) {
			let scope = null;
			watch([() => opts.ref.current, () => opts.enabled.current], ([ref, enabled]) => {
				if (ref && enabled) {
					if (!scope) scope = new FocusScope(opts);
					scope.mount(ref);
				} else if (scope) {
					scope.unmount();
					scope = null;
				}
			});
			return { get props() {
				return { tabindex: -1 };
			} };
		}
	};
	function Focus_scope($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { enabled = false, trapFocus = false, loop = false, onCloseAutoFocus = noop, onOpenAutoFocus = noop, focusScope, ref } = $$props;
			const focusScopeState = FocusScope.use({
				enabled: boxWith(() => enabled),
				trap: boxWith(() => trapFocus),
				loop,
				onCloseAutoFocus: boxWith(() => onCloseAutoFocus),
				onOpenAutoFocus: boxWith(() => onOpenAutoFocus),
				ref
			});
			focusScope?.($$renderer, { props: focusScopeState.props });
			$$renderer.push(`<!---->`);
		});
	}
	var noopPointer = () => {};
	globalThis.bitsTextSelectionLayers ??= /* @__PURE__ */ new Map();
	var TextSelectionLayerState = class TextSelectionLayerState {
		static create(opts) {
			return new TextSelectionLayerState(opts);
		}
		opts;
		domContext;
		#unsubSelectionLock = noop;
		#enabledSnapshot = false;
		#onPointerDownSnapshot = noopPointer;
		#onPointerUpSnapshot = noopPointer;
		constructor(opts) {
			this.opts = opts;
			this.domContext = new DOMContext(opts.ref);
			let unsubEvents = noop;
			watch(() => [
				this.opts.enabled.current,
				this.opts.onPointerDown.current,
				this.opts.onPointerUp.current
			], ([enabled, onPointerDown, onPointerUp]) => {
				this.#enabledSnapshot = enabled;
				this.#onPointerDownSnapshot = onPointerDown;
				this.#onPointerUpSnapshot = onPointerUp;
				if (enabled) {
					globalThis.bitsTextSelectionLayers.set(this, this.opts.enabled);
					unsubEvents();
					unsubEvents = this.#addEventListeners();
				}
				return () => {
					this.#enabledSnapshot = false;
					unsubEvents();
					this.#resetSelectionLock();
					globalThis.bitsTextSelectionLayers.delete(this);
				};
			});
		}
		#addEventListeners() {
			return executeCallbacks(on(this.domContext.getDocument(), "pointerdown", this.#pointerdown), on(this.domContext.getDocument(), "pointerup", composeHandlers(this.#resetSelectionLock, this.#pointerupUserHandler)));
		}
		#pointerupUserHandler = (e) => {
			this.#onPointerUpSnapshot(e);
		};
		#pointerdown = (e) => {
			if (!this.#enabledSnapshot) return;
			const node = this.opts.ref.current;
			const target = e.target;
			if (!isHTMLElement(node) || !isHTMLElement(target)) return;
			/**
			* We only lock user-selection overflow if layer is the top most layer and
			* pointerdown occurred inside the node. You are still allowed to select text
			* outside the node provided pointerdown occurs outside the node.
			*/
			if (!isHighestLayer(this) || !contains(node, target)) return;
			this.#onPointerDownSnapshot(e);
			if (e.defaultPrevented) return;
			this.#unsubSelectionLock = preventTextSelectionOverflow(node, this.domContext.getDocument().body);
		};
		#resetSelectionLock = () => {
			this.#unsubSelectionLock();
			this.#unsubSelectionLock = noop;
		};
	};
	var getUserSelect = (node) => node.style.userSelect || node.style.webkitUserSelect;
	function preventTextSelectionOverflow(node, body) {
		const originalBodyUserSelect = getUserSelect(body);
		const originalNodeUserSelect = getUserSelect(node);
		setUserSelect(body, "none");
		setUserSelect(node, "text");
		return () => {
			setUserSelect(body, originalBodyUserSelect);
			setUserSelect(node, originalNodeUserSelect);
		};
	}
	function setUserSelect(node, value) {
		node.style.userSelect = value;
		node.style.webkitUserSelect = value;
	}
	function isHighestLayer(instance) {
		const layersArr = [...globalThis.bitsTextSelectionLayers];
		if (!layersArr.length) return false;
		const highestLayer = layersArr.at(-1);
		if (!highestLayer) return false;
		return highestLayer[0] === instance;
	}
	function Text_selection_layer($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { preventOverflowTextSelection = true, onPointerDown = noop, onPointerUp = noop, id, children, enabled, ref } = $$props;
			TextSelectionLayerState.create({
				id: boxWith(() => id),
				onPointerDown: boxWith(() => onPointerDown),
				onPointerUp: boxWith(() => onPointerUp),
				enabled: boxWith(() => enabled && preventOverflowTextSelection),
				ref
			});
			children?.($$renderer);
			$$renderer.push(`<!---->`);
		});
	}
	globalThis.bitsIdCounter ??= { current: 0 };
	/**
	* Generates a unique ID based on a global counter.
	*/
	function useId(prefix = "bits") {
		globalThis.bitsIdCounter.current++;
		return `${prefix}-${globalThis.bitsIdCounter.current}`;
	}
	var SharedState = class {
		#factory;
		#subscribers = 0;
		#state;
		#scope;
		constructor(factory) {
			this.#factory = factory;
		}
		#dispose() {
			this.#subscribers -= 1;
			if (this.#scope && this.#subscribers <= 0) {
				this.#scope();
				this.#state = void 0;
				this.#scope = void 0;
			}
		}
		get(...args) {
			this.#subscribers += 1;
			if (this.#state === void 0) this.#scope = () => {};
			return this.#state;
		}
	};
	var lockMap = new SvelteMap();
	var initialBodyStyle = null;
	var cleanupTimeoutId = null;
	var isInCleanupTransition = false;
	var anyLocked = boxWith(() => {
		for (const value of lockMap.values()) if (value) return true;
		return false;
	});
	/**
	* We track the time we scheduled the cleanup to prevent race conditions
	* when multiple locks are created/destroyed in the same tick, ensuring
	* only the last one to schedule the cleanup will run.
	*
	* reference: https://github.com/huntabyte/bits-ui/issues/1639
	*/
	var cleanupScheduledAt = null;
	var bodyLockStackCount = new SharedState(() => {
		function resetBodyStyle(documentObj) {}
		function cancelPendingCleanup() {
			if (cleanupTimeoutId === null) return;
			window.clearTimeout(cleanupTimeoutId);
			cleanupTimeoutId = null;
		}
		function scheduleCleanupIfNoNewLocks(delay, callback) {
			cancelPendingCleanup();
			isInCleanupTransition = true;
			cleanupScheduledAt = Date.now();
			const currentCleanupId = cleanupScheduledAt;
			/**
			* We schedule the cleanup to run after a delay to allow new locks to register
			* that might have been added in the same tick as the current cleanup.
			*
			* If a new lock is added in the same tick, the cleanup will be cancelled and
			* a new cleanup will be scheduled.
			*
			* This is to prevent the cleanup from running too early and resetting the body
			* style before the new lock has had a chance to apply its styles.
			*/
			const cleanupFn = () => {
				cleanupTimeoutId = null;
				if (cleanupScheduledAt !== currentCleanupId) return;
				if (!isAnyLocked(lockMap)) {
					isInCleanupTransition = false;
					callback();
				} else isInCleanupTransition = false;
			};
			const actualDelay = delay === null ? 24 : delay;
			cleanupTimeoutId = window.setTimeout(cleanupFn, actualDelay);
		}
		function ensureInitialStyleCaptured() {
			if (initialBodyStyle === null && lockMap.size === 0 && !isInCleanupTransition) initialBodyStyle = document.body.getAttribute("style");
		}
		watch(() => anyLocked.current, () => {
			if (!anyLocked.current) return;
			ensureInitialStyleCaptured();
			isInCleanupTransition = false;
			const htmlStyle = getComputedStyle(document.documentElement);
			const bodyStyle = getComputedStyle(document.body);
			const hasStableGutter = htmlStyle.scrollbarGutter?.includes("stable") || bodyStyle.scrollbarGutter?.includes("stable");
			const verticalScrollbarWidth = window.innerWidth - document.documentElement.clientWidth;
			const config = {
				padding: Number.parseInt(bodyStyle.paddingRight ?? "0", 10) + verticalScrollbarWidth,
				margin: Number.parseInt(bodyStyle.marginRight ?? "0", 10)
			};
			if (verticalScrollbarWidth > 0 && !hasStableGutter) {
				document.body.style.paddingRight = `${config.padding}px`;
				document.body.style.marginRight = `${config.margin}px`;
				document.body.style.setProperty("--scrollbar-width", `${verticalScrollbarWidth}px`);
			}
			document.body.style.overflow = "hidden";
			if (isIOS) on(document, "touchmove", (e) => {
				if (e.target !== document.documentElement) return;
				if (e.touches.length > 1) return;
				e.preventDefault();
			}, { passive: false });
			/**
			* We ensure pointer-events: none is applied _after_ DOM updates, so that any focus/
			* interaction changes from opening overlays/menus complete _before_ we block pointer
			* events.
			*
			* this avoids race conditions where pointer-events could be set too early and break
			* focus/interaction.
			*/
			afterTick(() => {
				document.body.style.pointerEvents = "none";
				document.body.style.overflow = "hidden";
			});
		});
		return {
			get lockMap() {
				return lockMap;
			},
			resetBodyStyle,
			scheduleCleanupIfNoNewLocks,
			cancelPendingCleanup,
			ensureInitialStyleCaptured
		};
	});
	var BodyScrollLock = class {
		#id = useId();
		#initialState;
		#restoreScrollDelay = () => null;
		#countState;
		locked;
		constructor(initialState, restoreScrollDelay = () => null) {
			this.#initialState = initialState;
			this.#restoreScrollDelay = restoreScrollDelay;
			this.#countState = bodyLockStackCount.get();
			if (!this.#countState) return;
			/**
			* Since a new lock is being created, we cancel any pending cleanup to
			* prevent the cleanup from running too early and resetting the body style
			* before the new lock has had a chance to apply its styles.
			*
			* reference: https://github.com/huntabyte/bits-ui/issues/1639
			*/
			this.#countState.cancelPendingCleanup();
			this.#countState.ensureInitialStyleCaptured();
			this.#countState.lockMap.set(this.#id, this.#initialState ?? false);
			this.locked = boxWith(() => this.#countState.lockMap.get(this.#id) ?? false, (v) => this.#countState.lockMap.set(this.#id, v));
		}
	};
	function isAnyLocked(map) {
		for (const [_, value] of map) if (value) return true;
		return false;
	}
	function Scroll_lock($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { preventScroll = true, restoreScrollDelay = null } = $$props;
			if (preventScroll) new BodyScrollLock(preventScroll, () => restoreScrollDelay);
		});
	}
	function Dialog_trigger($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { id = createId(uid), ref = null, children, child, disabled = false, $$slots, $$events, ...restProps } = $$props;
			const triggerState = DialogTriggerState.create({
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v),
				disabled: boxWith(() => Boolean(disabled))
			});
			const mergedProps = derived(() => mergeProps(restProps, triggerState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><button${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></button>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Dialog_description($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { id = createId(uid), children, child, ref = null, $$slots, $$events, ...restProps } = $$props;
			const descriptionState = DialogDescriptionState.create({
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const mergedProps = derived(() => mergeProps(restProps, descriptionState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><div${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></div>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Dialog($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { open = false, onOpenChange = noop, onOpenChangeComplete = noop, children } = $$props;
			DialogRootState.create({
				variant: boxWith(() => "dialog"),
				open: boxWith(() => open, (v) => {
					open = v;
					onOpenChange(v);
				}),
				onOpenChangeComplete: boxWith(() => onOpenChangeComplete)
			});
			children?.($$renderer);
			$$renderer.push(`<!---->`);
			bind_props($$props, { open });
		});
	}
	function Dialog_close($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { children, child, id = createId(uid), ref = null, disabled = false, $$slots, $$events, ...restProps } = $$props;
			const closeState = DialogCloseState.create({
				variant: boxWith(() => "close"),
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v),
				disabled: boxWith(() => Boolean(disabled))
			});
			const mergedProps = derived(() => mergeProps(restProps, closeState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><button${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></button>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Dialog_content($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { id = createId(uid), children, child, ref = null, forceMount = false, onCloseAutoFocus = noop, onOpenAutoFocus = noop, onEscapeKeydown = noop, onInteractOutside = noop, trapFocus = true, preventScroll = true, restoreScrollDelay = null, $$slots, $$events, ...restProps } = $$props;
			const contentState = DialogContentState.create({
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const mergedProps = derived(() => mergeProps(restProps, contentState.props));
			if (contentState.shouldRender || forceMount) {
				$$renderer.push("<!--[0-->");
				{
					function focusScope($$renderer, { props: focusScopeProps }) {
						Escape_layer($$renderer, spread_props([mergedProps(), {
							enabled: contentState.root.opts.open.current,
							ref: contentState.opts.ref,
							onEscapeKeydown: (e) => {
								onEscapeKeydown(e);
								if (e.defaultPrevented) return;
								contentState.root.handleClose();
							},
							children: ($$renderer) => {
								Dismissible_layer($$renderer, spread_props([mergedProps(), {
									ref: contentState.opts.ref,
									enabled: contentState.root.opts.open.current,
									onInteractOutside: (e) => {
										onInteractOutside(e);
										if (e.defaultPrevented) return;
										contentState.root.handleClose();
									},
									children: ($$renderer) => {
										Text_selection_layer($$renderer, spread_props([mergedProps(), {
											ref: contentState.opts.ref,
											enabled: contentState.root.opts.open.current,
											children: ($$renderer) => {
												if (child) {
													$$renderer.push("<!--[0-->");
													if (contentState.root.opts.open.current) {
														$$renderer.push("<!--[0-->");
														Scroll_lock($$renderer, {
															preventScroll,
															restoreScrollDelay
														});
													} else $$renderer.push("<!--[-1-->");
													$$renderer.push(`<!--]--> `);
													child($$renderer, {
														props: mergeProps(mergedProps(), focusScopeProps),
														...contentState.snippetProps
													});
													$$renderer.push(`<!---->`);
												} else {
													$$renderer.push("<!--[-1-->");
													Scroll_lock($$renderer, { preventScroll });
													$$renderer.push(`<!----> <div${attributes({ ...mergeProps(mergedProps(), focusScopeProps) })}>`);
													children?.($$renderer);
													$$renderer.push(`<!----></div>`);
												}
												$$renderer.push(`<!--]-->`);
											},
											$$slots: { default: true }
										}]));
									},
									$$slots: { default: true }
								}]));
							},
							$$slots: { default: true }
						}]));
					}
					Focus_scope($$renderer, {
						ref: contentState.opts.ref,
						loop: true,
						trapFocus,
						enabled: contentState.root.opts.open.current,
						onOpenAutoFocus,
						onCloseAutoFocus,
						focusScope,
						$$slots: { focusScope: true }
					});
				}
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	var SvelteResizeObserver = class {
		#node;
		#onResize;
		constructor(node, onResize) {
			this.#node = node;
			this.#onResize = onResize;
			this.handler = this.handler.bind(this);
		}
		handler() {
			let rAF = 0;
			const _node = this.#node();
			if (!_node) return;
			const resizeObserver = new ResizeObserver(() => {
				cancelAnimationFrame(rAF);
				rAF = window.requestAnimationFrame(this.#onResize);
			});
			resizeObserver.observe(_node);
			return () => {
				window.cancelAnimationFrame(rAF);
				resizeObserver.unobserve(_node);
			};
		}
	};
	var Presence = class {
		opts;
		present;
		#afterAnimations;
		#isPresent = false;
		#hasMounted = false;
		#transitionStatus = void 0;
		#transitionFrame = null;
		constructor(opts) {
			this.opts = opts;
			this.present = this.opts.open;
			this.#isPresent = opts.open.current;
			this.#afterAnimations = new AnimationsComplete({
				ref: this.opts.ref,
				afterTick: this.opts.open
			});
			watch(() => this.present.current, (isOpen) => {
				if (!this.#hasMounted) {
					this.#hasMounted = true;
					return;
				}
				this.#clearTransitionFrame();
				if (isOpen) this.#isPresent = true;
				this.#transitionStatus = isOpen ? "starting" : "ending";
				if (isOpen) this.#transitionFrame = window.requestAnimationFrame(() => {
					this.#transitionFrame = null;
					if (this.present.current) this.#transitionStatus = void 0;
				});
				this.#afterAnimations.run(() => {
					if (isOpen !== this.present.current) return;
					if (!isOpen) this.#isPresent = false;
					this.#transitionStatus = void 0;
				});
			});
		}
		#_isPresent = derived(() => {
			return this.#isPresent;
		});
		get isPresent() {
			return this.#_isPresent();
		}
		set isPresent($$value) {
			return this.#_isPresent($$value);
		}
		get transitionStatus() {
			return this.#transitionStatus;
		}
		#clearTransitionFrame() {
			if (this.#transitionFrame === null) return;
			window.cancelAnimationFrame(this.#transitionFrame);
			this.#transitionFrame = null;
		}
	};
	function Presence_layer($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { open, forceMount, presence, ref } = $$props;
			const presenceState = new Presence({
				open: boxWith(() => open),
				ref
			});
			if (forceMount || open || presenceState.isPresent) {
				$$renderer.push("<!--[0-->");
				presence?.($$renderer, {
					present: presenceState.isPresent,
					transitionStatus: presenceState.transitionStatus
				});
				$$renderer.push(`<!---->`);
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	/**
	* Clamps a number between a minimum and maximum value.
	*/
	function clamp(n, min, max) {
		return Math.min(max, Math.max(min, n));
	}
	var StateMachine = class {
		state;
		#machine;
		constructor(initialState, machine) {
			this.state = simpleBox(initialState);
			this.#machine = machine;
			this.dispatch = this.dispatch.bind(this);
		}
		#reducer(event) {
			return this.#machine[this.state.current][event] ?? this.state.current;
		}
		dispatch(event) {
			this.state.current = this.#reducer(event);
		}
	};
	var scrollAreaAttrs = createBitsAttrs({
		component: "scroll-area",
		parts: [
			"root",
			"viewport",
			"corner",
			"thumb",
			"scrollbar"
		]
	});
	var ScrollAreaRootContext = new Context("ScrollArea.Root");
	var ScrollAreaScrollbarContext = new Context("ScrollArea.Scrollbar");
	var ScrollAreaScrollbarVisibleContext = new Context("ScrollArea.ScrollbarVisible");
	var ScrollAreaScrollbarAxisContext = new Context("ScrollArea.ScrollbarAxis");
	var ScrollAreaScrollbarSharedContext = new Context("ScrollArea.ScrollbarShared");
	var ScrollAreaRootState = class ScrollAreaRootState {
		static create(opts) {
			return ScrollAreaRootContext.set(new ScrollAreaRootState(opts));
		}
		opts;
		attachment;
		scrollAreaNode = null;
		viewportNode = null;
		contentNode = null;
		scrollbarXNode = null;
		scrollbarYNode = null;
		cornerWidth = 0;
		cornerHeight = 0;
		scrollbarXEnabled = false;
		scrollbarYEnabled = false;
		domContext;
		constructor(opts) {
			this.opts = opts;
			this.attachment = attachRef(opts.ref, (v) => this.scrollAreaNode = v);
			this.domContext = new DOMContext(opts.ref);
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			dir: this.opts.dir.current,
			style: {
				position: "relative",
				"--bits-scroll-area-corner-height": `${this.cornerHeight}px`,
				"--bits-scroll-area-corner-width": `${this.cornerWidth}px`
			},
			[scrollAreaAttrs.root]: "",
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaViewportState = class ScrollAreaViewportState {
		static create(opts) {
			return new ScrollAreaViewportState(opts, ScrollAreaRootContext.get());
		}
		opts;
		root;
		attachment;
		#contentId = simpleBox(useId());
		#contentRef = simpleBox(null);
		contentAttachment = attachRef(this.#contentRef, (v) => this.root.contentNode = v);
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			this.attachment = attachRef(opts.ref, (v) => this.root.viewportNode = v);
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			style: {
				overflowX: this.root.scrollbarXEnabled ? "scroll" : "hidden",
				overflowY: this.root.scrollbarYEnabled ? "scroll" : "hidden"
			},
			[scrollAreaAttrs.viewport]: "",
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
		#contentProps = derived(() => ({
			id: this.#contentId.current,
			"data-scroll-area-content": "",
			style: { minWidth: this.root.scrollbarXEnabled ? "fit-content" : void 0 },
			...this.contentAttachment
		}));
		get contentProps() {
			return this.#contentProps();
		}
		set contentProps($$value) {
			return this.#contentProps($$value);
		}
	};
	var ScrollAreaScrollbarState = class ScrollAreaScrollbarState {
		static create(opts) {
			return ScrollAreaScrollbarContext.set(new ScrollAreaScrollbarState(opts, ScrollAreaRootContext.get()));
		}
		opts;
		root;
		#isHorizontal = derived(() => this.opts.orientation.current === "horizontal");
		get isHorizontal() {
			return this.#isHorizontal();
		}
		set isHorizontal($$value) {
			return this.#isHorizontal($$value);
		}
		hasThumb = false;
		constructor(opts, root) {
			this.opts = opts;
			this.root = root;
			watch(() => this.isHorizontal, (isHorizontal) => {
				if (isHorizontal) {
					this.root.scrollbarXEnabled = true;
					return () => {
						this.root.scrollbarXEnabled = false;
					};
				} else {
					this.root.scrollbarYEnabled = true;
					return () => {
						this.root.scrollbarYEnabled = false;
					};
				}
			});
		}
	};
	var ScrollAreaScrollbarHoverState = class ScrollAreaScrollbarHoverState {
		static create() {
			return new ScrollAreaScrollbarHoverState(ScrollAreaScrollbarContext.get());
		}
		scrollbar;
		root;
		isVisible = false;
		constructor(scrollbar) {
			this.scrollbar = scrollbar;
			this.root = scrollbar.root;
		}
		#props = derived(() => ({ "data-state": this.isVisible ? "visible" : "hidden" }));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaScrollbarScrollState = class ScrollAreaScrollbarScrollState {
		static create() {
			return new ScrollAreaScrollbarScrollState(ScrollAreaScrollbarContext.get());
		}
		scrollbar;
		root;
		machine = new StateMachine("hidden", {
			hidden: { SCROLL: "scrolling" },
			scrolling: {
				SCROLL_END: "idle",
				POINTER_ENTER: "interacting"
			},
			interacting: {
				SCROLL: "interacting",
				POINTER_LEAVE: "idle"
			},
			idle: {
				HIDE: "hidden",
				SCROLL: "scrolling",
				POINTER_ENTER: "interacting"
			}
		});
		#isHidden = derived(() => this.machine.state.current === "hidden");
		get isHidden() {
			return this.#isHidden();
		}
		set isHidden($$value) {
			return this.#isHidden($$value);
		}
		constructor(scrollbar) {
			this.scrollbar = scrollbar;
			this.root = scrollbar.root;
			useDebounce(() => this.machine.dispatch("SCROLL_END"), 100);
			this.onpointerenter = this.onpointerenter.bind(this);
			this.onpointerleave = this.onpointerleave.bind(this);
		}
		onpointerenter(_) {
			this.machine.dispatch("POINTER_ENTER");
		}
		onpointerleave(_) {
			this.machine.dispatch("POINTER_LEAVE");
		}
		#props = derived(() => ({
			"data-state": this.machine.state.current === "hidden" ? "hidden" : "visible",
			onpointerenter: this.onpointerenter,
			onpointerleave: this.onpointerleave
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaScrollbarAutoState = class ScrollAreaScrollbarAutoState {
		static create() {
			return new ScrollAreaScrollbarAutoState(ScrollAreaScrollbarContext.get());
		}
		scrollbar;
		root;
		isVisible = false;
		constructor(scrollbar) {
			this.scrollbar = scrollbar;
			this.root = scrollbar.root;
			const handleResize = () => {
				const viewportNode = this.root.viewportNode;
				if (!viewportNode) return;
				const isOverflowX = viewportNode.offsetWidth < viewportNode.scrollWidth;
				const isOverflowY = viewportNode.offsetHeight < viewportNode.scrollHeight;
				this.isVisible = this.scrollbar.isHorizontal ? isOverflowX : isOverflowY;
			};
			new SvelteResizeObserver(() => this.root.viewportNode, handleResize);
			new SvelteResizeObserver(() => this.root.contentNode, handleResize);
		}
		#props = derived(() => ({ "data-state": this.isVisible ? "visible" : "hidden" }));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaScrollbarVisibleState = class ScrollAreaScrollbarVisibleState {
		static create() {
			return ScrollAreaScrollbarVisibleContext.set(new ScrollAreaScrollbarVisibleState(ScrollAreaScrollbarContext.get()));
		}
		scrollbar;
		root;
		thumbNode = null;
		pointerOffset = 0;
		sizes = {
			content: 0,
			viewport: 0,
			scrollbar: {
				size: 0,
				paddingStart: 0,
				paddingEnd: 0
			}
		};
		#thumbRatio = derived(() => getThumbRatio(this.sizes.viewport, this.sizes.content));
		get thumbRatio() {
			return this.#thumbRatio();
		}
		set thumbRatio($$value) {
			return this.#thumbRatio($$value);
		}
		#hasThumb = derived(() => Boolean(this.thumbRatio > 0 && this.thumbRatio < 1));
		get hasThumb() {
			return this.#hasThumb();
		}
		set hasThumb($$value) {
			return this.#hasThumb($$value);
		}
		prevTransformStyle = "";
		constructor(scrollbar) {
			this.scrollbar = scrollbar;
			this.root = scrollbar.root;
		}
		setSizes(sizes) {
			this.sizes = sizes;
		}
		getScrollPosition(pointerPos, dir) {
			return getScrollPositionFromPointer({
				pointerPos,
				pointerOffset: this.pointerOffset,
				sizes: this.sizes,
				dir
			});
		}
		onThumbPointerUp() {
			this.pointerOffset = 0;
		}
		onThumbPointerDown(pointerPos) {
			this.pointerOffset = pointerPos;
		}
		xOnThumbPositionChange() {
			if (!(this.root.viewportNode && this.thumbNode)) return;
			const scrollPos = this.root.viewportNode.scrollLeft;
			const transformStyle = `translate3d(${getThumbOffsetFromScroll({
				scrollPos,
				sizes: this.sizes,
				dir: this.root.opts.dir.current
			})}px, 0, 0)`;
			this.thumbNode.style.transform = transformStyle;
			this.prevTransformStyle = transformStyle;
		}
		xOnWheelScroll(scrollPos) {
			if (!this.root.viewportNode) return;
			this.root.viewportNode.scrollLeft = scrollPos;
		}
		xOnDragScroll(pointerPos) {
			if (!this.root.viewportNode) return;
			this.root.viewportNode.scrollLeft = this.getScrollPosition(pointerPos, this.root.opts.dir.current);
		}
		yOnThumbPositionChange() {
			if (!(this.root.viewportNode && this.thumbNode)) return;
			const scrollPos = this.root.viewportNode.scrollTop;
			const transformStyle = `translate3d(0, ${getThumbOffsetFromScroll({
				scrollPos,
				sizes: this.sizes
			})}px, 0)`;
			this.thumbNode.style.transform = transformStyle;
			this.prevTransformStyle = transformStyle;
		}
		yOnWheelScroll(scrollPos) {
			if (!this.root.viewportNode) return;
			this.root.viewportNode.scrollTop = scrollPos;
		}
		yOnDragScroll(pointerPos) {
			if (!this.root.viewportNode) return;
			this.root.viewportNode.scrollTop = this.getScrollPosition(pointerPos, this.root.opts.dir.current);
		}
	};
	var ScrollAreaScrollbarXState = class ScrollAreaScrollbarXState {
		static create(opts) {
			return ScrollAreaScrollbarAxisContext.set(new ScrollAreaScrollbarXState(opts, ScrollAreaScrollbarVisibleContext.get()));
		}
		opts;
		scrollbarVis;
		root;
		scrollbar;
		attachment;
		computedStyle;
		constructor(opts, scrollbarVis) {
			this.opts = opts;
			this.scrollbarVis = scrollbarVis;
			this.root = scrollbarVis.root;
			this.scrollbar = scrollbarVis.scrollbar;
			this.attachment = attachRef(this.scrollbar.opts.ref, (v) => this.root.scrollbarXNode = v);
		}
		onThumbPointerDown = (pointerPos) => {
			this.scrollbarVis.onThumbPointerDown(pointerPos.x);
		};
		onDragScroll = (pointerPos) => {
			this.scrollbarVis.xOnDragScroll(pointerPos.x);
		};
		onThumbPointerUp = () => {
			this.scrollbarVis.onThumbPointerUp();
		};
		onThumbPositionChange = () => {
			this.scrollbarVis.xOnThumbPositionChange();
		};
		onWheelScroll = (e, maxScrollPos) => {
			if (!this.root.viewportNode) return;
			const scrollPos = this.root.viewportNode.scrollLeft + e.deltaX;
			this.scrollbarVis.xOnWheelScroll(scrollPos);
			if (isScrollingWithinScrollbarBounds(scrollPos, maxScrollPos)) e.preventDefault();
		};
		onResize = () => {
			if (!(this.scrollbar.opts.ref.current && this.root.viewportNode && this.computedStyle)) return;
			this.scrollbarVis.setSizes({
				content: this.root.viewportNode.scrollWidth,
				viewport: this.root.viewportNode.offsetWidth,
				scrollbar: {
					size: this.scrollbar.opts.ref.current.clientWidth,
					paddingStart: toInt(this.computedStyle.paddingLeft),
					paddingEnd: toInt(this.computedStyle.paddingRight)
				}
			});
		};
		#thumbSize = derived(() => {
			return getThumbSize(this.scrollbarVis.sizes);
		});
		get thumbSize() {
			return this.#thumbSize();
		}
		set thumbSize($$value) {
			return this.#thumbSize($$value);
		}
		#props = derived(() => ({
			id: this.scrollbar.opts.id.current,
			"data-orientation": "horizontal",
			style: {
				bottom: 0,
				left: this.root.opts.dir.current === "rtl" ? "var(--bits-scroll-area-corner-width)" : 0,
				right: this.root.opts.dir.current === "ltr" ? "var(--bits-scroll-area-corner-width)" : 0,
				"--bits-scroll-area-thumb-width": `${this.thumbSize}px`
			},
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaScrollbarYState = class ScrollAreaScrollbarYState {
		static create(opts) {
			return ScrollAreaScrollbarAxisContext.set(new ScrollAreaScrollbarYState(opts, ScrollAreaScrollbarVisibleContext.get()));
		}
		opts;
		scrollbarVis;
		root;
		scrollbar;
		attachment;
		computedStyle;
		constructor(opts, scrollbarVis) {
			this.opts = opts;
			this.scrollbarVis = scrollbarVis;
			this.root = scrollbarVis.root;
			this.scrollbar = scrollbarVis.scrollbar;
			this.attachment = attachRef(this.scrollbar.opts.ref, (v) => this.root.scrollbarYNode = v);
			this.onThumbPointerDown = this.onThumbPointerDown.bind(this);
			this.onDragScroll = this.onDragScroll.bind(this);
			this.onThumbPointerUp = this.onThumbPointerUp.bind(this);
			this.onThumbPositionChange = this.onThumbPositionChange.bind(this);
			this.onWheelScroll = this.onWheelScroll.bind(this);
			this.onResize = this.onResize.bind(this);
		}
		onThumbPointerDown(pointerPos) {
			this.scrollbarVis.onThumbPointerDown(pointerPos.y);
		}
		onDragScroll(pointerPos) {
			this.scrollbarVis.yOnDragScroll(pointerPos.y);
		}
		onThumbPointerUp() {
			this.scrollbarVis.onThumbPointerUp();
		}
		onThumbPositionChange() {
			this.scrollbarVis.yOnThumbPositionChange();
		}
		onWheelScroll(e, maxScrollPos) {
			if (!this.root.viewportNode) return;
			const scrollPos = this.root.viewportNode.scrollTop + e.deltaY;
			this.scrollbarVis.yOnWheelScroll(scrollPos);
			if (isScrollingWithinScrollbarBounds(scrollPos, maxScrollPos)) e.preventDefault();
		}
		onResize() {
			if (!(this.scrollbar.opts.ref.current && this.root.viewportNode && this.computedStyle)) return;
			this.scrollbarVis.setSizes({
				content: this.root.viewportNode.scrollHeight,
				viewport: this.root.viewportNode.offsetHeight,
				scrollbar: {
					size: this.scrollbar.opts.ref.current.clientHeight,
					paddingStart: toInt(this.computedStyle.paddingTop),
					paddingEnd: toInt(this.computedStyle.paddingBottom)
				}
			});
		}
		#thumbSize = derived(() => {
			return getThumbSize(this.scrollbarVis.sizes);
		});
		get thumbSize() {
			return this.#thumbSize();
		}
		set thumbSize($$value) {
			return this.#thumbSize($$value);
		}
		#props = derived(() => ({
			id: this.scrollbar.opts.id.current,
			"data-orientation": "vertical",
			style: {
				top: 0,
				right: this.root.opts.dir.current === "ltr" ? 0 : void 0,
				left: this.root.opts.dir.current === "rtl" ? 0 : void 0,
				bottom: "var(--bits-scroll-area-corner-height)",
				"--bits-scroll-area-thumb-height": `${this.thumbSize}px`
			},
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaScrollbarSharedState = class ScrollAreaScrollbarSharedState {
		static create() {
			return ScrollAreaScrollbarSharedContext.set(new ScrollAreaScrollbarSharedState(ScrollAreaScrollbarAxisContext.get()));
		}
		scrollbarState;
		root;
		scrollbarVis;
		scrollbar;
		rect = null;
		prevWebkitUserSelect = "";
		handleResize;
		handleThumbPositionChange;
		handleWheelScroll;
		handleThumbPointerDown;
		handleThumbPointerUp;
		#maxScrollPos = derived(() => this.scrollbarVis.sizes.content - this.scrollbarVis.sizes.viewport);
		get maxScrollPos() {
			return this.#maxScrollPos();
		}
		set maxScrollPos($$value) {
			return this.#maxScrollPos($$value);
		}
		constructor(scrollbarState) {
			this.scrollbarState = scrollbarState;
			this.root = scrollbarState.root;
			this.scrollbarVis = scrollbarState.scrollbarVis;
			this.scrollbar = scrollbarState.scrollbarVis.scrollbar;
			this.handleResize = () => this.scrollbarState.onResize();
			this.handleThumbPositionChange = this.scrollbarState.onThumbPositionChange;
			this.handleWheelScroll = this.scrollbarState.onWheelScroll;
			this.handleThumbPointerDown = this.scrollbarState.onThumbPointerDown;
			this.handleThumbPointerUp = this.scrollbarState.onThumbPointerUp;
			new SvelteResizeObserver(() => this.scrollbar.opts.ref.current, this.handleResize);
			new SvelteResizeObserver(() => this.root.contentNode, this.handleResize);
			this.onpointerdown = this.onpointerdown.bind(this);
			this.onpointermove = this.onpointermove.bind(this);
			this.onpointerup = this.onpointerup.bind(this);
			this.onlostpointercapture = this.onlostpointercapture.bind(this);
		}
		handleDragScroll(e) {
			if (!this.rect) return;
			const x = e.clientX - this.rect.left;
			const y = e.clientY - this.rect.top;
			this.scrollbarState.onDragScroll({
				x,
				y
			});
		}
		#cleanupPointerState() {
			if (this.rect === null) return;
			this.root.domContext.getDocument().body.style.webkitUserSelect = this.prevWebkitUserSelect;
			if (this.root.viewportNode) this.root.viewportNode.style.scrollBehavior = "";
			this.rect = null;
		}
		onpointerdown(e) {
			if (e.button !== 0) return;
			e.target.setPointerCapture(e.pointerId);
			this.rect = this.scrollbar.opts.ref.current?.getBoundingClientRect() ?? null;
			this.prevWebkitUserSelect = this.root.domContext.getDocument().body.style.webkitUserSelect;
			this.root.domContext.getDocument().body.style.webkitUserSelect = "none";
			if (this.root.viewportNode) this.root.viewportNode.style.scrollBehavior = "auto";
			this.handleDragScroll(e);
		}
		onpointermove(e) {
			this.handleDragScroll(e);
		}
		onpointerup(e) {
			const target = e.target;
			if (target.hasPointerCapture(e.pointerId)) target.releasePointerCapture(e.pointerId);
			this.#cleanupPointerState();
		}
		onlostpointercapture(_) {
			this.#cleanupPointerState();
		}
		#props = derived(() => mergeProps({
			...this.scrollbarState.props,
			style: {
				position: "absolute",
				...this.scrollbarState.props.style
			},
			[scrollAreaAttrs.scrollbar]: "",
			onpointerdown: this.onpointerdown,
			onpointermove: this.onpointermove,
			onpointerup: this.onpointerup,
			onlostpointercapture: this.onlostpointercapture
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	var ScrollAreaThumbImplState = class ScrollAreaThumbImplState {
		static create(opts) {
			return new ScrollAreaThumbImplState(opts, ScrollAreaScrollbarSharedContext.get());
		}
		opts;
		scrollbarState;
		attachment;
		#root;
		#removeUnlinkedScrollListener;
		#debounceScrollEnd = useDebounce(() => {
			if (this.#removeUnlinkedScrollListener) {
				this.#removeUnlinkedScrollListener();
				this.#removeUnlinkedScrollListener = void 0;
			}
		}, 100);
		constructor(opts, scrollbarState) {
			this.opts = opts;
			this.scrollbarState = scrollbarState;
			this.#root = scrollbarState.root;
			this.attachment = attachRef(this.opts.ref, (v) => this.scrollbarState.scrollbarVis.thumbNode = v);
			this.onpointerdowncapture = this.onpointerdowncapture.bind(this);
			this.onpointerup = this.onpointerup.bind(this);
		}
		onpointerdowncapture(e) {
			const thumb = e.target;
			if (!thumb) return;
			const thumbRect = thumb.getBoundingClientRect();
			const x = e.clientX - thumbRect.left;
			const y = e.clientY - thumbRect.top;
			this.scrollbarState.handleThumbPointerDown({
				x,
				y
			});
		}
		onpointerup(_) {
			this.scrollbarState.handleThumbPointerUp();
		}
		#props = derived(() => ({
			id: this.opts.id.current,
			"data-state": this.scrollbarState.scrollbarVis.hasThumb ? "visible" : "hidden",
			style: {
				width: "var(--bits-scroll-area-thumb-width)",
				height: "var(--bits-scroll-area-thumb-height)",
				transform: this.scrollbarState.scrollbarVis.prevTransformStyle
			},
			onpointerdowncapture: this.onpointerdowncapture,
			onpointerup: this.onpointerup,
			[scrollAreaAttrs.thumb]: "",
			...this.attachment
		}));
		get props() {
			return this.#props();
		}
		set props($$value) {
			return this.#props($$value);
		}
	};
	function toInt(value) {
		return value ? Number.parseInt(value, 10) : 0;
	}
	function getThumbRatio(viewportSize, contentSize) {
		const ratio = viewportSize / contentSize;
		return Number.isNaN(ratio) ? 0 : ratio;
	}
	function getThumbSize(sizes) {
		const ratio = getThumbRatio(sizes.viewport, sizes.content);
		const scrollbarPadding = sizes.scrollbar.paddingStart + sizes.scrollbar.paddingEnd;
		const thumbSize = (sizes.scrollbar.size - scrollbarPadding) * ratio;
		return Math.max(thumbSize, 18);
	}
	function getScrollPositionFromPointer({ pointerPos, pointerOffset, sizes, dir = "ltr" }) {
		const thumbSizePx = getThumbSize(sizes);
		const thumbCenter = thumbSizePx / 2;
		const offset = pointerOffset || thumbCenter;
		const thumbOffsetFromEnd = thumbSizePx - offset;
		const minPointerPos = sizes.scrollbar.paddingStart + offset;
		const maxPointerPos = sizes.scrollbar.size - sizes.scrollbar.paddingEnd - thumbOffsetFromEnd;
		const maxScrollPos = sizes.content - sizes.viewport;
		const scrollRange = dir === "ltr" ? [0, maxScrollPos] : [maxScrollPos * -1, 0];
		return linearScale([minPointerPos, maxPointerPos], scrollRange)(pointerPos);
	}
	function getThumbOffsetFromScroll({ scrollPos, sizes, dir = "ltr" }) {
		const thumbSizePx = getThumbSize(sizes);
		const scrollbarPadding = sizes.scrollbar.paddingStart + sizes.scrollbar.paddingEnd;
		const scrollbar = sizes.scrollbar.size - scrollbarPadding;
		const maxScrollPos = sizes.content - sizes.viewport;
		const maxThumbPos = scrollbar - thumbSizePx;
		const scrollClampRange = dir === "ltr" ? [0, maxScrollPos] : [maxScrollPos * -1, 0];
		const scrollWithoutMomentum = clamp(scrollPos, scrollClampRange[0], scrollClampRange[1]);
		return linearScale([0, maxScrollPos], [0, maxThumbPos])(scrollWithoutMomentum);
	}
	function linearScale(input, output) {
		return (value) => {
			if (input[0] === input[1] || output[0] === output[1]) return output[0];
			const ratio = (output[1] - output[0]) / (input[1] - input[0]);
			return output[0] + ratio * (value - input[0]);
		};
	}
	function isScrollingWithinScrollbarBounds(scrollPos, maxScrollPos) {
		return scrollPos > 0 && scrollPos < maxScrollPos;
	}
	function Scroll_area($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { ref = null, id = createId(uid), type = "hover", dir = "ltr", scrollHideDelay = 600, children, child, $$slots, $$events, ...restProps } = $$props;
			const rootState = ScrollAreaRootState.create({
				type: boxWith(() => type),
				dir: boxWith(() => dir),
				scrollHideDelay: boxWith(() => scrollHideDelay),
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const mergedProps = derived(() => mergeProps(restProps, rootState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><div${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></div>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Scroll_area_viewport($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { ref = null, id = createId(uid), children, $$slots, $$events, ...restProps } = $$props;
			const viewportState = ScrollAreaViewportState.create({
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const mergedProps = derived(() => mergeProps(restProps, viewportState.props));
			const mergedContentProps = derived(() => mergeProps({}, viewportState.contentProps));
			$$renderer.push(`<div${attributes({ ...mergedProps() })}><div${attributes({ ...mergedContentProps() })}>`);
			children?.($$renderer);
			$$renderer.push(`<!----></div></div>`);
			bind_props($$props, { ref });
		});
	}
	function Scroll_area_scrollbar_shared($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { child, children, $$slots, $$events, ...restProps } = $$props;
			const scrollbarSharedState = ScrollAreaScrollbarSharedState.create();
			const mergedProps = derived(() => mergeProps(restProps, scrollbarSharedState.props));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><div${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></div>`);
			}
			$$renderer.push(`<!--]-->`);
		});
	}
	function Scroll_area_scrollbar_x($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { $$slots, $$events, ...restProps } = $$props;
			const isMounted = new IsMounted();
			const scrollbarXState = ScrollAreaScrollbarXState.create({ mounted: boxWith(() => isMounted.current) });
			Scroll_area_scrollbar_shared($$renderer, spread_props([derived(() => mergeProps(restProps, scrollbarXState.props))()]));
		});
	}
	function Scroll_area_scrollbar_y($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { $$slots, $$events, ...restProps } = $$props;
			const isMounted = new IsMounted();
			const scrollbarYState = ScrollAreaScrollbarYState.create({ mounted: boxWith(() => isMounted.current) });
			Scroll_area_scrollbar_shared($$renderer, spread_props([derived(() => mergeProps(restProps, scrollbarYState.props))()]));
		});
	}
	function Scroll_area_scrollbar_visible($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { $$slots, $$events, ...restProps } = $$props;
			if (ScrollAreaScrollbarVisibleState.create().scrollbar.opts.orientation.current === "horizontal") {
				$$renderer.push("<!--[0-->");
				Scroll_area_scrollbar_x($$renderer, spread_props([restProps]));
			} else {
				$$renderer.push("<!--[-1-->");
				Scroll_area_scrollbar_y($$renderer, spread_props([restProps]));
			}
			$$renderer.push(`<!--]-->`);
		});
	}
	function Scroll_area_scrollbar_auto($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { forceMount = false, $$slots, $$events, ...restProps } = $$props;
			const scrollbarAutoState = ScrollAreaScrollbarAutoState.create();
			const mergedProps = derived(() => mergeProps(restProps, scrollbarAutoState.props));
			{
				function presence($$renderer) {
					Scroll_area_scrollbar_visible($$renderer, spread_props([mergedProps()]));
				}
				Presence_layer($$renderer, {
					open: forceMount || scrollbarAutoState.isVisible,
					ref: scrollbarAutoState.scrollbar.opts.ref,
					presence,
					$$slots: { presence: true }
				});
			}
		});
	}
	function Scroll_area_scrollbar_scroll($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { forceMount = false, $$slots, $$events, ...restProps } = $$props;
			const scrollbarScrollState = ScrollAreaScrollbarScrollState.create();
			const mergedProps = derived(() => mergeProps(restProps, scrollbarScrollState.props));
			{
				function presence($$renderer) {
					Scroll_area_scrollbar_visible($$renderer, spread_props([mergedProps()]));
				}
				Presence_layer($$renderer, spread_props([mergedProps(), {
					open: forceMount || !scrollbarScrollState.isHidden,
					ref: scrollbarScrollState.scrollbar.opts.ref,
					presence,
					$$slots: { presence: true }
				}]));
			}
		});
	}
	function Scroll_area_scrollbar_hover($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { forceMount = false, $$slots, $$events, ...restProps } = $$props;
			const scrollbarHoverState = ScrollAreaScrollbarHoverState.create();
			const scrollbarAutoState = ScrollAreaScrollbarAutoState.create();
			const mergedProps = derived(() => mergeProps(restProps, scrollbarHoverState.props, scrollbarAutoState.props, { "data-state": scrollbarHoverState.isVisible ? "visible" : "hidden" }));
			const open = derived(() => forceMount || scrollbarHoverState.isVisible && scrollbarAutoState.isVisible);
			{
				function presence($$renderer) {
					Scroll_area_scrollbar_visible($$renderer, spread_props([mergedProps()]));
				}
				Presence_layer($$renderer, {
					open: open(),
					ref: scrollbarAutoState.scrollbar.opts.ref,
					presence,
					$$slots: { presence: true }
				});
			}
		});
	}
	function Scroll_area_scrollbar($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { ref = null, id = createId(uid), orientation, $$slots, $$events, ...restProps } = $$props;
			const scrollbarState = ScrollAreaScrollbarState.create({
				orientation: boxWith(() => orientation),
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v)
			});
			const type = derived(() => scrollbarState.root.opts.type.current);
			if (type() === "hover") {
				$$renderer.push("<!--[0-->");
				Scroll_area_scrollbar_hover($$renderer, spread_props([restProps, { id }]));
			} else if (type() === "scroll") {
				$$renderer.push("<!--[1-->");
				Scroll_area_scrollbar_scroll($$renderer, spread_props([restProps, { id }]));
			} else if (type() === "auto") {
				$$renderer.push("<!--[2-->");
				Scroll_area_scrollbar_auto($$renderer, spread_props([restProps, { id }]));
			} else if (type() === "always") {
				$$renderer.push("<!--[3-->");
				Scroll_area_scrollbar_visible($$renderer, spread_props([restProps, { id }]));
			} else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Scroll_area_thumb_impl($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { ref = null, id, child, children, present, $$slots, $$events, ...restProps } = $$props;
			const isMounted = new IsMounted();
			const thumbState = ScrollAreaThumbImplState.create({
				id: boxWith(() => id),
				ref: boxWith(() => ref, (v) => ref = v),
				mounted: boxWith(() => isMounted.current)
			});
			const mergedProps = derived(() => mergeProps(restProps, thumbState.props, { style: { hidden: !present } }));
			if (child) {
				$$renderer.push("<!--[0-->");
				child($$renderer, { props: mergedProps() });
				$$renderer.push(`<!---->`);
			} else {
				$$renderer.push(`<!--[-1--><div${attributes({ ...mergedProps() })}>`);
				children?.($$renderer);
				$$renderer.push(`<!----></div>`);
			}
			$$renderer.push(`<!--]-->`);
			bind_props($$props, { ref });
		});
	}
	function Scroll_area_thumb($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const uid = props_id($$renderer);
			let { id = createId(uid), ref = null, forceMount = false, $$slots, $$events, ...restProps } = $$props;
			const scrollbarState = ScrollAreaScrollbarVisibleContext.get();
			let $$settled = true;
			let $$inner_renderer;
			function $$render_inner($$renderer) {
				{
					function presence($$renderer, { present }) {
						Scroll_area_thumb_impl($$renderer, spread_props([restProps, {
							id,
							present,
							get ref() {
								return ref;
							},
							set ref($$value) {
								ref = $$value;
								$$settled = false;
							}
						}]));
					}
					Presence_layer($$renderer, {
						open: forceMount || scrollbarState.hasThumb,
						ref: scrollbarState.scrollbar.opts.ref,
						presence,
						$$slots: { presence: true }
					});
				}
			}
			do {
				$$settled = true;
				$$inner_renderer = $$renderer.copy();
				$$render_inner($$inner_renderer);
			} while (!$$settled);
			$$renderer.subsume($$inner_renderer);
			bind_props($$props, { ref });
		});
	}
	function _page$39($$renderer) {
		let open = false;
		let $$settled = true;
		let $$inner_renderer;
		function $$render_inner($$renderer) {
			$$renderer.push(`<h1>Bits UI server rendering</h1> `);
			if (Dialog) {
				$$renderer.push("<!--[-->");
				Dialog($$renderer, {
					get open() {
						return open;
					},
					set open($$value) {
						open = $$value;
						$$settled = false;
					},
					children: ($$renderer) => {
						if (Dialog_trigger) {
							$$renderer.push("<!--[-->");
							Dialog_trigger($$renderer, {
								children: ($$renderer) => {
									$$renderer.push(`<!---->Open library dialog`);
								},
								$$slots: { default: true }
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
						$$renderer.push(` `);
						if (Dialog_content) {
							$$renderer.push("<!--[-->");
							Dialog_content($$renderer, {
								children: ($$renderer) => {
									if (Dialog_title) {
										$$renderer.push("<!--[-->");
										Dialog_title($$renderer, {
											children: ($$renderer) => {
												$$renderer.push(`<!---->Library dialog`);
											},
											$$slots: { default: true }
										});
										$$renderer.push("<!--]-->");
									} else {
										$$renderer.push("<!--[!-->");
										$$renderer.push("<!--]-->");
									}
									$$renderer.push(` `);
									if (Dialog_description) {
										$$renderer.push("<!--[-->");
										Dialog_description($$renderer, {
											children: ($$renderer) => {
												$$renderer.push(`<!---->Hydrated Bits UI dialog content.`);
											},
											$$slots: { default: true }
										});
										$$renderer.push("<!--]-->");
									} else {
										$$renderer.push("<!--[!-->");
										$$renderer.push("<!--]-->");
									}
									$$renderer.push(` `);
									if (Dialog_close) {
										$$renderer.push("<!--[-->");
										Dialog_close($$renderer, {
											children: ($$renderer) => {
												$$renderer.push(`<!---->Close library dialog`);
											},
											$$slots: { default: true }
										});
										$$renderer.push("<!--]-->");
									} else {
										$$renderer.push("<!--[!-->");
										$$renderer.push("<!--]-->");
									}
								},
								$$slots: { default: true }
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
					},
					$$slots: { default: true }
				});
				$$renderer.push("<!--]-->");
			} else {
				$$renderer.push("<!--[!-->");
				$$renderer.push("<!--]-->");
			}
			$$renderer.push(` <p data-testid="dialog-state">${escape_html$1(open ? "open" : "closed")}</p> `);
			if (Scroll_area) {
				$$renderer.push("<!--[-->");
				Scroll_area($$renderer, {
					type: "always",
					style: "height: 180px; width: 320px; overflow: hidden;",
					children: ($$renderer) => {
						if (Scroll_area_viewport) {
							$$renderer.push("<!--[-->");
							Scroll_area_viewport($$renderer, {
								"data-testid": "library-scroll-viewport",
								style: "height: 100%; width: 100%;",
								children: ($$renderer) => {
									$$renderer.push(`<!--[-->`);
									const each_array = ensure_array_like(Array.from({ length: 20 }, (_, i) => i + 1));
									for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
										let number = each_array[$$index];
										$$renderer.push(`<p>Library row ${escape_html$1(number)}</p>`);
									}
									$$renderer.push(`<!--]-->`);
								},
								$$slots: { default: true }
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
						$$renderer.push(` `);
						if (Scroll_area_scrollbar) {
							$$renderer.push("<!--[-->");
							Scroll_area_scrollbar($$renderer, {
								orientation: "vertical",
								style: "width: 10px; background: #ddd;",
								children: ($$renderer) => {
									if (Scroll_area_thumb) {
										$$renderer.push("<!--[-->");
										Scroll_area_thumb($$renderer, { style: "background: #555; border-radius: 5px;" });
										$$renderer.push("<!--]-->");
									} else {
										$$renderer.push("<!--[!-->");
										$$renderer.push("<!--]-->");
									}
								},
								$$slots: { default: true }
							});
							$$renderer.push("<!--]-->");
						} else {
							$$renderer.push("<!--[!-->");
							$$renderer.push("<!--]-->");
						}
					},
					$$slots: { default: true }
				});
				$$renderer.push("<!--]-->");
			} else {
				$$renderer.push("<!--[!-->");
				$$renderer.push("<!--]-->");
			}
		}
		do {
			$$settled = true;
			$$inner_renderer = $$renderer.copy();
			$$render_inner($$inner_renderer);
		} while (!$$settled);
		$$renderer.subsume($$inner_renderer);
	}
	function _page$38($$renderer) {
		/**
		* A page that reports a failure the way kit and Svelte report one: through
		* the console. Kit's `log_handle_error_hook_failure` and Svelte's
		* `unresolved_hydratable` both do exactly this, in a production build, on
		* paths a render reaches — so before the engine had a console at all, every
		* such report was a ReferenceError thrown in the middle of the render and
		* the reason went nowhere.
		*
		* The line is written while the page renders, which in production is inside
		* the Go process. It has to come out of Go's log.
		*/
		console.error("skgo-console-probe: this page reported a failure while rendering");
		$$renderer.push(`<h1 data-testid="title">Console</h1> <p data-testid="console-note">This page reported a failure to the console while it rendered.</p>`);
	}
	function _page$37($$renderer) {
		$$renderer.push(`<h1 data-testid="title">Contact</h1>`);
	}
	function _page$36($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Destinations</h1> <p>One Go load, one fetch of each kind of destination the app has.</p> <ul><!--[-->`);
			const each_array = ensure_array_like(data.answers);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let answer = each_array[$$index];
				$$renderer.push(`<li${attr("data-path", answer.path)}>${escape_html$1(answer.status)} ${escape_html$1(answer.contentType)}</li>`);
			}
			$$renderer.push(`<!--]--></ul>`);
		});
	}
	function _page$35($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { params } = $$props;
			$$renderer.push(`<h1 data-testid="title">Docs</h1> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="doc-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="doc-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function _page$34($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Empty</h1> <section><h2>The load's notes</h2> <p data-testid="notes-count">${escape_html$1(data.notes.length)} notes</p> <ul data-testid="notes">`);
			const each_array = ensure_array_like(data.notes);
			if (each_array.length !== 0) {
				$$renderer.push("<!--[-->");
				for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
					let note = each_array[$$index];
					$$renderer.push(`<li data-testid="note">${escape_html$1(note)}</li>`);
				}
			} else $$renderer.push(`<!--[!--><li data-testid="notes-empty">No notes.</li>`);
			$$renderer.push(`<!--]--></ul></section> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="report-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let report;
						var promises = $$renderer.run([async () => report = (await save(getReport()))()]);
						$$renderer.push(`<section><h2 data-testid="report-title">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(report.title)));
						$$renderer.push(`</h2> <p data-testid="report-count">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(report.diagnostics.length)));
						$$renderer.push(` diagnostics</p> <ul data-testid="report-diagnostics">`);
						$$renderer.async_block([promises[0]], ($$renderer) => {
							const each_array_1 = ensure_array_like(report.diagnostics);
							if (each_array_1.length !== 0) {
								$$renderer.push("<!--[-->");
								for (let $$index_1 = 0, $$length = each_array_1.length; $$index_1 < $$length; $$index_1++) {
									let diagnostic = each_array_1[$$index_1];
									$$renderer.push(`<li data-testid="report-diagnostic">${escape_html$1(diagnostic.message)}</li>`);
								}
							} else $$renderer.push(`<!--[!--><li data-testid="report-empty">No diagnostics.</li>`);
						});
						$$renderer.push(`<!--]--></ul> <p data-testid="report-models">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(report.models.length)));
						$$renderer.push(` models</p></section>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="models-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let models;
						var promises_1 = $$renderer.run([async () => models = (await save(getModels()))()]);
						$$renderer.push(`<section><p data-testid="models-count">`);
						$$renderer.async([promises_1[0]], ($$renderer) => $$renderer.push(() => escape_html$1(models.length)));
						$$renderer.push(` models</p> <ul data-testid="models">`);
						$$renderer.async_block([promises_1[0]], ($$renderer) => {
							const each_array_2 = ensure_array_like(models);
							if (each_array_2.length !== 0) {
								$$renderer.push("<!--[-->");
								for (let $$index_2 = 0, $$length = each_array_2.length; $$index_2 < $$length; $$index_2++) {
									let model = each_array_2[$$index_2];
									$$renderer.push(`<li data-testid="model">${escape_html$1(model)}</li>`);
								}
							} else $$renderer.push(`<!--[!--><li data-testid="models-empty">No models.</li>`);
						});
						$$renderer.push(`<!--]--></ul></section>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="known-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let known;
						var promises_2 = $$renderer.run([async () => known = (await save(getKnownReport()))()]);
						$$renderer.push(`<section><h2 data-testid="known-title">`);
						$$renderer.async([promises_2[0]], ($$renderer) => $$renderer.push(() => escape_html$1(known.title)));
						$$renderer.push(`</h2> <p data-testid="known-count">`);
						$$renderer.async([promises_2[0]], ($$renderer) => $$renderer.push(() => escape_html$1(known.diagnostics.length)));
						$$renderer.push(` diagnostics</p> <ul data-testid="known-diagnostics">`);
						$$renderer.async_block([promises_2[0]], ($$renderer) => {
							const each_array_3 = ensure_array_like(known.diagnostics);
							if (each_array_3.length !== 0) {
								$$renderer.push("<!--[-->");
								for (let $$index_3 = 0, $$length = each_array_3.length; $$index_3 < $$length; $$index_3++) {
									let diagnostic = each_array_3[$$index_3];
									$$renderer.push(`<li data-testid="known-diagnostic">${escape_html$1(diagnostic.message)}</li>`);
								}
							} else $$renderer.push(`<!--[!--><li data-testid="known-empty">No diagnostics.</li>`);
						});
						$$renderer.push(`<!--]--></ul></section>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` <section><button data-testid="reparse">Reparse</button> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--></section>`);
		});
	}
	function _page$33($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Sensor</h1> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="sensor-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					$$renderer.push(`<p data-testid="reading">`);
					$$renderer.push(async () => escape_html$1((await save(readSensor()))().celsius));
					$$renderer.push(`</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function _page$32($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Tally</h1> <p data-testid="tally">`);
			$$renderer.push(async () => escape_html$1((await save(bumpTally()))().runs));
			$$renderer.push(`</p>`);
		});
	}
	function _page$31($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Brew</h1> <p data-testid="brew">${escape_html$1(data.brew)}</p>`);
		});
	}
	function _page$30($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Where to</h1> <p data-testid="where">`);
			$$renderer.push(async () => escape_html$1((await save(whereTo()))().where));
			$$renderer.push(`</p>`);
		});
	}
	function _page$29($$renderer) {
		$$renderer.push(`<h1 data-testid="title">Never seen</h1>`);
	}
	function _page$28($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			/**
			* A component that can only fail on the server.
			*
			* `document` exists in a browser and does not exist in the rendering engine,
			* so this throw happens while Go builds the document and never once the page
			* is live. It is the failure dev could not have before Go rendered in dev:
			* kit's own dev server never ran this component on the server, so a
			* component that breaks only there broke for the first time in production.
			*
			* What a visitor gets is the app's error page, at the status handleError
			* chose — not a blank document, and not a page that quietly leaves this out.
			*/
			function failOnTheServer() {
				throw new Error("skgo: /error/server-only cannot render on the server");
			}
			if (typeof document === "undefined") $$renderer.push(`<!--[0-->${escape_html$1(failOnTheServer())}`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <p data-testid="server-only">this page only renders in the browser</p>`);
		});
	}
	function _page$27($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Secret</h1> <p data-testid="secret">${escape_html$1(data.secret)}</p>`);
		});
	}
	function _page$26($$renderer) {
		$$renderer.push(`<h2 data-testid="title">First sibling page</h2>`);
	}
	function _page$25($$renderer) {
		$$renderer.push(`<h2 data-testid="title">Second sibling page</h2>`);
	}
	var load$5 = async ({ fetch, url }) => {
		const response = await fetch(`/api/todos?via=universal&refresh=${url.searchParams.get("refresh") ?? "0"}`, { headers: {
			"x-demo": "universal",
			accept: "application/json"
		} });
		const publicResponse = await fetch("/api/todos", { credentials: "omit" });
		if (!response.ok || !publicResponse.ok) throw new Error("The todos API did not answer");
		return {
			todos: await response.json(),
			publicTodos: await publicResponse.json(),
			hook: response.headers.get("x-fetch-hook") ?? "browser"
		};
	};
	function _page$24($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Server fetch</h1> <p>This universal load calls the Go todos API. Its first response is included in
  the document and reused during hydration. Sign in above and reload to see
  private todos; the request with credentials omitted stays public.</p> <p data-testid="fetch-hook">Fetch hook: ${escape_html$1(data.hook)}</p> <h2>Your todos</h2> <ul data-testid="fetch-todos"><!--[-->`);
			const each_array = ensure_array_like(data.todos);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let todo = each_array[$$index];
				$$renderer.push(`<li${attr("data-id", todo.id)}>${escape_html$1(todo.text)}</li>`);
			}
			$$renderer.push(`<!--]--></ul> <h2>With credentials omitted</h2> <ul data-testid="fetch-public-todos"><!--[-->`);
			const each_array_1 = ensure_array_like(data.publicTodos);
			for (let $$index_1 = 0, $$length = each_array_1.length; $$index_1 < $$length; $$index_1++) {
				let todo = each_array_1[$$index_1];
				$$renderer.push(`<li${attr("data-id", todo.id)}>${escape_html$1(todo.text)}</li>`);
			}
			$$renderer.push(`<!--]--></ul> <a${attr("href", `/fetch?refresh=${Number(page.url.searchParams.get("refresh") ?? 0) + 1}`)} data-testid="fetch-navigate">Refresh through client navigation</a>`);
		});
	}
	var load$4 = ({ data, url }) => {
		const overdueOnly = url.searchParams.get("overdue") === "1";
		const invoices = data.invoices.map((invoice) => ({
			...invoice,
			overdue: invoice.outstandingCents > 0 && invoice.dueDate < data.asOf
		})).filter((invoice) => !overdueOnly || invoice.overdue).sort((a, b) => a.dueDate.localeCompare(b.dueDate));
		return {
			invoices,
			totalCents: invoices.reduce((total, invoice) => total + invoice.outstandingCents, 0),
			overdueOnly
		};
	};
	function _page$23($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1>Invoices</h1> <nav><a href="/invoices?overdue=1">Overdue</a> <a href="/invoices">All invoices</a></nav> <p data-testid="invoice-filter">${escape_html$1(data.overdueOnly ? "Overdue" : "All invoices")}</p> <ul data-testid="invoices"><!--[-->`);
			const each_array = ensure_array_like(data.invoices);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let invoice = each_array[$$index];
				$$renderer.push(`<li data-testid="invoice">${escape_html$1(invoice.number)}: ${escape_html$1(invoice.dueDate)} — ${escape_html$1(invoice.outstandingCents)} cents${escape_html$1(invoice.overdue ? " (overdue)" : "")}</li>`);
			}
			$$renderer.push(`<!--]--></ul> <p data-testid="invoice-total">Total: ${escape_html$1(data.totalCents)} cents</p>`);
		});
	}
	var load$3 = async ({ params, url }) => {
		const item = getItem(params.id);
		const loadedItem = await item;
		return url.searchParams.has("retain") ? {
			loadedItem,
			item
		} : { loadedItem };
	};
	function _page$22($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, params } = $$props;
			$$renderer.push(`<h1 data-testid="title">Item ${escape_html$1(params.id)}</h1> <p data-testid="load-item-name">${escape_html$1(data.loadedItem.name)}</p> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="item-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					if (params.id) {
						$$renderer.push("<!--[0-->");
						let item;
						var promises = $$renderer.run([async () => item = (await save(getItem(params.id)))()]);
						$$renderer.push(`<p data-testid="item-name">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(item.name)));
						$$renderer.push(`</p> <p data-testid="colocated">`);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(item.colocated)));
						$$renderer.push(`</p>`);
					} else $$renderer.push("<!--[-1-->");
					$$renderer.push(`<!--]-->`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function Board($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			var board;
			var $$promises = $$renderer.run([async () => board = await async_derived(watchBoard)]);
			$$renderer.push(`<section class="board svelte-m5babl"><p>Todos on the board: <strong data-testid="board-count">`);
			$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(board().count)));
			$$renderer.push(`</strong></p> <p>Newest: <strong data-testid="board-newest">`);
			$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(board().newest)));
			$$renderer.push(`</strong></p> <p>Stream frame: <strong data-testid="board-push">`);
			$$renderer.async([$$promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(board().push)));
			$$renderer.push(`</strong></p></section>`);
		});
	}
	function _page$21($$renderer) {
		$$renderer.push(`<h1 data-testid="title">Live</h1> <p>The board below is a <code>query.live</code>. Go answered it while this page
  was being rendered, so the number was in the HTML before any script ran, and
  Go pushes every later value down the same stream.</p> `);
		{
			function failed($$renderer, error) {
				$$renderer.push(`<p data-testid="board-failed">${escape_html$1(error.message)}</p>`);
			}
			$$renderer.boundary({ failed }, ($$renderer) => {
				$$renderer.push(`<!--[-->`);
				Board($$renderer, {});
				$$renderer.push(`<!--]-->`);
			});
		}
	}
	function _page$20($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data, form } = $$props;
			head("4ega0l", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Middleware | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-4ega0l"><h1 data-testid="title">Middleware</h1> <p>Go middleware authenticated this visit before the load below ran.</p> <p data-testid="mw-token">${escape_html$1(data.token)}</p> <p data-testid="mw-route">${escape_html$1(data.route)}</p> <p data-testid="mw-cookie">${escape_html$1(data.cookie)}</p> <p><a data-testid="mw-alpha" href="/middleware/alpha">Alpha</a></p> <noscript><p data-testid="mw-noscript">JavaScript is disabled. The form still works.</p></noscript> `);
			if (form && "receipt" in form) $$renderer.push(`<!--[0--><h2 data-testid="mw-receipt">${escape_html$1(form.receipt)}</h2>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <!--[-->`);
			const each_array = ensure_array_like(["native", "enhanced"]);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let mode = each_array[$$index];
				$$renderer.push(`<form method="POST"${attr("data-testid", mode + "-note-form")} class="svelte-4ega0l"><h2>${escape_html$1(mode === "native" ? "Native submission" : "Enhanced submission")}</h2> <label class="svelte-4ega0l">Note <input name="note" value="remember-this" class="svelte-4ega0l"/></label> <button type="submit" class="svelte-4ega0l">Save note</button></form>`);
			}
			$$renderer.push(`<!--]--></main>`);
		});
	}
	function _page$19($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			head("7sa0pj", $$renderer, ($$renderer) => {
				$$renderer.title(($$renderer) => {
					$$renderer.push(`<title>Middleware ${escape_html$1(data.slug)} | skgo example</title>`);
				});
			});
			$$renderer.push(`<main class="card svelte-7sa0pj"><p><a href="/middleware">Middleware</a></p> <h1 data-testid="title">Middleware ${escape_html$1(data.slug)}</h1> <p data-testid="mw-slug-token">${escape_html$1(data.token)}</p> <p data-testid="mw-slug-route">${escape_html$1(data.route)}</p> <p data-testid="mw-slug-param">${escape_html$1(data.slug)}</p> <p data-testid="mw-slug-data">${escape_html$1(data.data ? "data request" : "document request")}</p></main>`);
		});
	}
	var load$2 = async ({ fetch }) => {
		const response = await fetch("/request-fetch");
		if (!response.ok) throw new Error("The request-fetch page did not answer");
		const html = await response.text();
		return {
			fact: /data-testid="request-fetch-fact">([^<]+)</.exec(html)?.[1] ?? "not found",
			visitor: /data-testid="request-fetch-visitor">Fetched as ([^<]+)</.exec(html)?.[1] ?? "not found",
			status: response.status
		};
	};
	function _page$18($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Nested universal fetch</h1> <p>This page's universal load fetched another rendered page of the app.</p> <p data-testid="nested-fact">${escape_html$1(data.fact)}</p> <p data-testid="nested-visitor">${escape_html$1(data.visitor)}</p> <p data-testid="nested-status">${escape_html$1(data.status)}</p>`);
		});
	}
	function _page$17($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let withCount = false;
			let withEnabled = false;
			let withLabel = false;
			const fields = submit.fields;
			$$renderer.push(`<h1>Optional form values</h1> <form${attributes({
				"data-testid": "optional-form",
				...submit.enhance(async (form) => {
					await form.submit();
				})
			})}><label>Name <input${attributes({
				"data-testid": "optional-name",
				...fields.name.as("text")
			}, void 0, void 0, void 0, 4)}/></label> <!--[-->`);
			const each_array = ensure_array_like(fields.name.issues() ?? []);
			for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
				let issue = each_array[$$index];
				$$renderer.push(`<p data-testid="optional-name-issue">${escape_html$1(issue.message)}</p>`);
			}
			$$renderer.push(`<!--]--> <label><input data-testid="include-count" type="checkbox"${attr("checked", withCount, true)}/> Include count</label> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <label><input data-testid="include-enabled" type="checkbox"${attr("checked", withEnabled, true)}/> Include false</label> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <label><input data-testid="include-label" type="checkbox"${attr("checked", withLabel, true)}/> Include label</label> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> <button data-testid="optional-submit" type="submit">Submit</button> <button data-testid="optional-validate" type="button">Validate without submitting</button></form> `);
			$$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if ((fields.allIssues() ?? []).length > 0) $$renderer.push(`<!--[0--><p data-testid="optional-rejected">The form was rejected.</p>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]--> `);
			if (submit.result) $$renderer.push(`<!--[0--><section data-testid="optional-result"><p data-testid="optional-result-name">Name: ${escape_html$1(submit.result.name)}</p> <p data-testid="optional-result-count">Count: ${escape_html$1(submit.result.count)}</p> <p data-testid="optional-result-enabled">Enabled: ${escape_html$1(submit.result.enabled)}</p> <p data-testid="optional-result-label">Label: ${escape_html$1(submit.result.label)}</p> <p data-testid="optional-result-operations">Operations for ${escape_html$1(submit.result.name)}: ${escape_html$1(submit.result.operations)}</p></section>`);
			else $$renderer.push("<!--[-1-->");
			$$renderer.push(`<!--]-->`);
		});
	}
	function _page$16($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Plain</h1> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="plain-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					$$renderer.push(`<p data-testid="site-name">`);
					$$renderer.push(async () => escape_html$1((await save(getSite$1()))().name));
					$$renderer.push(`</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function _page$15($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Prerendered ${escape_html$1(data.slug)}</h1> <p data-testid="entry-receipt">${escape_html$1(data.receipt)}</p>`);
		});
	}
	async function load$1() {
		return { receipt: await buildReceipt("atlas") };
	}
	function _page$14($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Prerender consumer</h1> <p data-testid="prerender-consumer-receipt">${escape_html$1(data.receipt)}</p>`);
		});
	}
	async function match(url) {
		let pathname = decode_pathname((typeof url === "string" ? new URL(url, "https://skgo.internal/") : url).pathname);
		const raw = globalThis.__skgo_match(pathname);
		return JSON.parse(raw);
	}
	function _page$13($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const toApiTodos = resolve("/api/todos");
			const toItem42 = resolve("/items/[id]", { id: "42" });
			const robots = asset("robots.txt");
			$$renderer.push(`<h1 data-testid="title">Render-time paths</h1> <p>Every href below was computed by kit's own <code>$app/paths</code> functions while
  this page was rendered, not written into the markup by hand.</p> <p data-testid="resolve-static">resolve('/api/todos') = ${escape_html$1(toApiTodos)}</p> <p data-testid="resolve-dynamic">resolve('/items/[id]', id: '42') = ${escape_html$1(toItem42)}</p> <p data-testid="asset-href">asset('robots.txt') = ${escape_html$1(robots)}</p> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="match-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[-->`);
					{
						let matched;
						var promises = $$renderer.run([async () => matched = (await save(match("/items/77")))()]);
						$$renderer.push(`<p data-testid="match-result">match('/items/77') = `);
						$$renderer.async([promises[0]], ($$renderer) => $$renderer.push(() => escape_html$1(matched ? `${matched.id} ${JSON.stringify(matched.params)}` : "null")));
						$$renderer.push(`</p>`);
					}
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function _page$12($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Request fetch</h1> <p>A Go load fetched this page's data from a Go endpoint of the same app,
  in-process, with the visitor's own credentials.</p> <p data-testid="request-fetch-fact">${escape_html$1(data.fact)}</p> <p data-testid="request-fetch-visitor">Fetched as ${escape_html$1(data.visitor)}</p>`);
		});
	}
	var Editor = class {
		words;
		rev = 0;
		#count = derived(() => {
			this.rev;
			return this.words.length;
		});
		get count() {
			return this.#count();
		}
		set count($$value) {
			return this.#count($$value);
		}
		constructor(text) {
			this.words = text.split(" ");
		}
	};
	function _page$11($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const editor = new Editor("one two three four five");
			$$renderer.push(`<h1 data-testid="title">Runes class</h1> <p data-testid="word-count">Words: ${escape_html$1(editor.count)}</p>`);
		});
	}
	function _page$10($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">Prerendered shadow</h1> <p data-testid="shadow-slug">prerendered:${escape_html$1(page.params.slug)}</p>`);
		});
	}
	function _page$9($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<h1 data-testid="title">SPA</h1> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="spa-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="spa-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
		});
	}
	function _page$8($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Stream</h1> <p data-testid="headline">${escape_html$1(data.headline)}</p> <section><h2>Ticker</h2> `);
			await_block($$renderer, data.ticker, () => {
				$$renderer.push(`<p data-testid="ticker-pending">waiting for the ticker…</p>`);
			}, (ticker) => {
				$$renderer.push(`<p data-testid="ticker">${escape_html$1(ticker)}</p>`);
			});
			$$renderer.push(`<!--]--></section> <section><h2>Digest</h2> `);
			await_block($$renderer, data.digest, () => {
				$$renderer.push(`<p data-testid="digest-pending">waiting for the digest…</p>`);
			}, (digest) => {
				$$renderer.push(`<ul data-testid="digest"><!--[-->`);
				const each_array = ensure_array_like(digest);
				for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
					let line = each_array[$$index];
					$$renderer.push(`<li data-testid="digest-line">${escape_html$1(line)}</li>`);
				}
				$$renderer.push(`<!--]--></ul>`);
			});
			$$renderer.push(`<!--]--></section> <section><h2>${escape_html$1(data.weather.heading)}</h2> `);
			await_block($$renderer, data.weather.forecast, () => {
				$$renderer.push(`<p data-testid="forecast-pending">waiting for the forecast…</p>`);
			}, (forecast) => {
				$$renderer.push(`<p data-testid="forecast">${escape_html$1(forecast)}</p>`);
			});
			$$renderer.push(`<!--]--></section>`);
		});
	}
	function AddTodo($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<form><input data-testid="new-todo" name="text"${attr("value", "")} placeholder="new todo"/> <button data-testid="add-todo" type="submit"${attr("disabled", false, true)}>Add</button></form>`);
		});
	}
	function TodoCount($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<p>Todos: <strong data-testid="count">`);
			$$renderer.push(async () => escape_html$1((await save(watchCount()))()));
			$$renderer.push(`</strong></p>`);
		});
	}
	function TodoList($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			$$renderer.push(`<ul data-testid="todos"><!--[-->`);
			$$renderer.child_block(async ($$renderer) => {
				const each_array = ensure_array_like((await save(getTodos()))());
				for (let $$index = 0, $$length = each_array.length; $$index < $$length; $$index++) {
					let todo = each_array[$$index];
					$$renderer.push(`<li data-testid="todo"${attr_class("svelte-worp00", void 0, { "private": todo.private })}><a${attr("href", `/todos/${stringify$1(todo.id)}`)}>${escape_html$1(todo.text)}</a></li>`);
				}
			});
			$$renderer.push(`<!--]--></ul>`);
		});
	}
	function _page$7($$renderer) {
		$$renderer.push(`<h1 data-testid="title">Todos</h1> `);
		{
			function failed($$renderer, error) {
				$$renderer.push(`<p data-testid="count-failed">${escape_html$1(error.message)}</p>`);
			}
			$$renderer.boundary({ failed }, ($$renderer) => {
				$$renderer.push(`<!--[-->`);
				TodoCount($$renderer, {});
				$$renderer.push(`<!--]-->`);
			});
		}
		$$renderer.push(` `);
		{
			function failed($$renderer, error) {
				$$renderer.push(`<p data-testid="todos-failed">${escape_html$1(error.message)}</p>`);
			}
			$$renderer.boundary({ failed }, ($$renderer) => {
				$$renderer.push(`<!--[-->`);
				TodoList($$renderer, {});
				$$renderer.push(`<!--]-->`);
			});
		}
		$$renderer.push(` `);
		AddTodo($$renderer, {});
		$$renderer.push(`<!---->`);
	}
	function _page$6($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { params } = $$props;
			$$renderer.push(`<h1 data-testid="title">Todo</h1> `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="todo-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="todo-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` <p><a href="/todos">Back to todos</a></p>`);
		});
	}
	function _page$5($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const left = getNote("left");
			const right = getNote("right");
			const banner = getBanner();
			let leftText = "";
			let rightText = "";
			let bannerText = "";
			let busy = false;
			let wrote = 0;
			$$renderer.push(`<h1 data-testid="title">The refresh gate</h1> <p>Writing changes both notes and the banner, and asks for all three back. The
  handler accepts one instance of <code>getNote</code> and explicitly ignores <code>getBanner</code>, leaving its current value alone.</p> <div class="panels svelte-irlffu"><article data-testid="gate-panel" data-id="left" class="svelte-irlffu"><h2>left note</h2> `);
			if (left.error) $$renderer.push(`<!--[0--><p class="refused svelte-irlffu" data-testid="note-refused-left">${escape_html$1(left.error.message)}</p>`);
			else if (left.current) $$renderer.push(`<!--[1--><p data-testid="note-left">${escape_html$1(left.current.text)}</p>`);
			else $$renderer.push(`<!--[-1--><p data-testid="gate-pending">loading…</p>`);
			$$renderer.push(`<!--]--></article> <article data-testid="gate-panel" data-id="right" class="svelte-irlffu"><h2>right note</h2> `);
			if (right.error) $$renderer.push(`<!--[0--><p class="refused svelte-irlffu" data-testid="note-refused-right">${escape_html$1(right.error.message)}</p>`);
			else if (right.current) $$renderer.push(`<!--[1--><p data-testid="note-right">${escape_html$1(right.current.text)}</p>`);
			else $$renderer.push(`<!--[-1--><p data-testid="gate-pending">loading…</p>`);
			$$renderer.push(`<!--]--></article> <article data-testid="gate-panel" data-id="banner" class="svelte-irlffu"><h2>banner</h2> `);
			if (banner.error) $$renderer.push(`<!--[0--><p class="refused svelte-irlffu" data-testid="note-refused-banner">${escape_html$1(banner.error.message)}</p>`);
			else if (banner.current) $$renderer.push(`<!--[1--><p data-testid="note-banner">${escape_html$1(banner.current.text)}</p>`);
			else $$renderer.push(`<!--[-1--><p data-testid="gate-pending">loading…</p>`);
			$$renderer.push(`<!--]--></article></div> <form class="svelte-irlffu"><label class="svelte-irlffu">left <input data-testid="gate-left" name="left"${attr("value", leftText)}/></label> <label class="svelte-irlffu">right <input data-testid="gate-right" name="right"${attr("value", rightText)}/></label> <label class="svelte-irlffu">banner <input data-testid="gate-banner" name="banner"${attr("value", bannerText)}/></label> <button data-testid="gate-write" type="submit"${attr("disabled", busy, true)}>Write all three</button></form> <p>The command reported writing <strong data-testid="gate-wrote">${escape_html$1(wrote)}</strong> values.</p> <button data-testid="gate-reload"${attr("disabled", busy, true)}>Reload all three</button> <p><a href="/todos">Back to todos</a></p>`);
		});
	}
	function _page$4($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			const left = "p1";
			let id = left;
			let text = "";
			let refreshId = left;
			let busy = false;
			$$renderer.push(`<h1 data-testid="title">A pair of todos</h1> <div class="pair svelte-wt4csr">`);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="pair-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="pair-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(` `);
			{
				function failed($$renderer, error) {
					$$renderer.push(`<p data-testid="pair-failed">${escape_html$1(error.message)}</p>`);
				}
				$$renderer.boundary({ failed }, ($$renderer) => {
					$$renderer.push(`<!--[!-->`);
					$$renderer.push(`<p data-testid="pair-pending">loading…</p>`);
					$$renderer.push(`<!--]-->`);
				});
			}
			$$renderer.push(`</div> <form class="svelte-wt4csr"><label class="svelte-wt4csr">retitle <input data-testid="retitle-id" name="id"${attr("value", id)}/></label> <label class="svelte-wt4csr">to <input data-testid="retitle-text" name="text"${attr("value", text)} placeholder="new text"/></label> <label class="svelte-wt4csr">then refresh <input data-testid="retitle-refresh" name="refreshId"${attr("value", refreshId)}/></label> <button data-testid="retitle-save" type="submit"${attr("disabled", busy, true)}>Retitle</button></form> <p><a href="/todos">Back to todos</a></p>`);
		});
	}
	function _page$3($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1>Typed load dependencies</h1> <p data-testid="typed-dependency-label">${escape_html$1(data.label)}</p> <p>Load serial: <span data-testid="typed-dependency-serial">${escape_html$1(data.serial)}</span></p> <p>Untracked value: <span data-testid="typed-dependency-ignored">${escape_html$1(data.ignored)}</span></p> <nav><a href="/typed-dependencies/0/7/first">Unread B</a> <a href="/typed-dependencies/42/7/first">Read A</a> <a href="/typed-dependencies/42/7/second">Untracked ignored</a> <a href="/typed-dependencies/42/7/second?read=b">Read B instead</a> <a href="/typed-dependencies/0/7/second?read=b">Now unread A</a> <a href="/typed-dependencies/0/42/second?read=b">Now read B</a></nav>`);
		});
	}
	function _page$2($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1>Typed Go load</h1> <p data-testid="typed-load-label">${escape_html$1(data.label)}</p> <nav><a href="/typed-load/42">Order 42</a> <a href="/typed-load/0">Order zero</a></nav>`);
		});
	}
	var hex = (bytes) => Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");
	async function stream(response) {
		const reader = response.body.getReader();
		const decoder = new TextDecoder();
		let text = "";
		for (;;) {
			const { value, done } = await reader.read();
			if (done) return text;
			text += decoder.decode(value);
		}
	}
	async function external(fetch, url) {
		const query = url.searchParams;
		const target = query.get("target") ?? "";
		const out = {
			status: 0,
			body: "",
			header: "",
			cookies: "",
			error: ""
		};
		if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(target)) {
			out.error = "target must be a loopback http origin";
			return out;
		}
		try {
			const response = await fetch(`${target}${query.get("path") ?? "/"}`, { mode: query.get("mode") ?? "cors" });
			out.status = response.status;
			const header = query.get("header");
			if (header) try {
				out.header = String(response.headers.get(header));
			} catch (e) {
				out.header = "denied";
			}
			if (query.get("cookies")) try {
				out.cookies = response.headers.getSetCookie().join(" | ");
			} catch (e) {
				out.cookies = "denied";
			}
			switch (query.get("read") ?? "text") {
				case "text":
					out.body = await response.text() ?? "";
					break;
				case "json":
					out.body = JSON.stringify(await response.json());
					break;
				case "bytes":
					out.body = hex(new Uint8Array(await response.arrayBuffer()));
					break;
				case "stream": out.body = await stream(response);
			}
		} catch (e) {
			out.error = e.message;
		}
		return out;
	}
	async function request(fetch, url, q) {
		const kind = url.searchParams.get("kind");
		const echo = `/api/replay/echo?${q}`;
		let response;
		if (kind === "object") response = await fetch(new Request(echo, {
			method: "POST",
			headers: { "x-replay": "r" },
			body: "object-body"
		}));
		else if (kind === "bytes") response = await fetch(echo, {
			method: "POST",
			body: new TextEncoder().encode("bytes-body")
		});
		else response = await fetch(echo, {
			method: "POST",
			headers: { "x-replay": "h" },
			body: "both"
		});
		return await response.json();
	}
	async function secret(fetch, q) {
		const response = await fetch(`/api/replay/binary?${q}`);
		try {
			return `read:${response.headers.get("x-replay-secret")}`;
		} catch (e) {
			return "denied";
		}
	}
	async function cookies(fetch, q) {
		const first = await fetch(`/api/replay/cookies?${q}&set=1`);
		const second = await fetch(`/api/replay/cookies?${q}`);
		return {
			first: await first.text(),
			second: await second.text()
		};
	}
	var load = async ({ fetch, url }) => {
		const run = url.searchParams.get("run") ?? "none";
		const step = url.searchParams.get("step") ?? "1";
		const q = `run=${encodeURIComponent(run)}&step=${encodeURIComponent(step)}`;
		const probe = url.searchParams.get("probe");
		if (probe === "external") return {
			run,
			step,
			main: null,
			probe: await external(fetch, url)
		};
		if (probe === "request") return {
			run,
			step,
			main: null,
			probe: await request(fetch, url, q)
		};
		if (probe === "cookies") return {
			run,
			step,
			main: null,
			probe: await cookies(fetch, q)
		};
		if (probe === "secret") return {
			run,
			step,
			main: null,
			probe: await secret(fetch, q)
		};
		const text = await (await fetch(`/api/replay/text?${q}`)).text();
		const json = await (await fetch(`/api/replay/json?${q}`)).json();
		const aliased = await (await fetch(`/api/replay/json?run=${encodeURIComponent(run)}&alias=1`)).json();
		const binaryResponse = await fetch(`/api/replay/binary?${q}`);
		const binary = hex(new Uint8Array(await binaryResponse.arrayBuffer()));
		const shown = binaryResponse.headers.get("x-replay-allowed");
		const streamed = await stream(await fetch(`/api/replay/stream?${q}`));
		const breaking = await (await fetch(`/api/replay/breaking?${q}`)).text();
		const emptyResponse = await fetch(`/api/replay/empty?${q}`);
		const empty = `${emptyResponse.status}:${await emptyResponse.text() ? "body" : "none"}`;
		const missingResponse = await fetch(`/api/replay/missing?${q}`);
		const missing = `${missingResponse.status}:${missingResponse.ok}:${await missingResponse.text()}`;
		const echo = `/api/replay/echo?${q}`;
		const headerA = await (await fetch(echo, { headers: { "x-replay": "a" } })).json();
		const headerB = await (await fetch(echo, { headers: { "x-replay": "b" } })).json();
		const bodyAlpha = await (await fetch(echo, {
			method: "POST",
			body: "alpha"
		})).json();
		const bodyBeta = await (await fetch(echo, {
			method: "POST",
			body: "beta"
		})).json();
		const form = await (await fetch(`/api/replay/form?${q}`, {
			method: "POST",
			body: new URLSearchParams({ lamp: "one" })
		})).json();
		const cached = await (await fetch(`/api/replay/cached?run=${encodeURIComponent(run)}`)).json();
		const page = await (await fetch(`/request-fetch?run=${encodeURIComponent(run)}`)).text();
		const nested = /data-testid="request-fetch-fact">([^<]+)</.exec(page)?.[1] ?? "not found";
		return {
			run,
			step,
			probe: null,
			main: {
				text,
				json: `${json.lamp}@${json.step}`,
				aliased: `${aliased.lamp}@${aliased.step}`,
				binary,
				shown,
				streamed,
				breaking,
				empty,
				missing,
				headerA: `${headerA.header}@${headerA.step}`,
				headerB: `${headerB.header}@${headerB.step}`,
				bodyAlpha: `${bodyAlpha.method}:${bodyAlpha.body}@${bodyAlpha.step}`,
				bodyBeta: `${bodyBeta.method}:${bodyBeta.body}@${bodyBeta.step}`,
				form: `${form.method}:${form.body}:${form.contentType}`,
				cached: cached.lamp,
				nested
			}
		};
	};
	function _page$1($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			const m = derived(() => data.main);
			$$renderer.push(`<h1 data-testid="title">Universal fetch</h1> <p>This page's universal load fetches the app's replay endpoints. Each answer is
  in the document, and the browser reuses it instead of asking again.</p> <a data-testid="next-step"${attr("href", `/universal-fetch?run=${data.run}&step=${Number(data.step) + 1}`)}>Next step</a> <p data-testid="step">${escape_html$1(data.step)}</p> `);
			if (m()) $$renderer.push(`<!--[0--><dl><dt>text</dt> <dd data-testid="uf-text">${escape_html$1(m().text)}</dd> <dt>json</dt> <dd data-testid="uf-json">${escape_html$1(m().json)}</dd> <dt>rewritten by the hook</dt> <dd data-testid="uf-aliased">${escape_html$1(m().aliased)}</dd> <dt>binary</dt> <dd data-testid="uf-binary">${escape_html$1(m().binary)}</dd> <dt>allowed header</dt> <dd data-testid="uf-shown">${escape_html$1(m().shown)}</dd> <dt>stream</dt> <dd data-testid="uf-streamed">${escape_html$1(m().streamed)}</dd> <dt>script-breaking text</dt> <dd data-testid="uf-breaking">${escape_html$1(m().breaking)}</dd> <dt>no body</dt> <dd data-testid="uf-empty">${escape_html$1(m().empty)}</dd> <dt>not found</dt> <dd data-testid="uf-missing">${escape_html$1(m().missing)}</dd> <dt>header a</dt> <dd data-testid="uf-header-a">${escape_html$1(m().headerA)}</dd> <dt>header b</dt> <dd data-testid="uf-header-b">${escape_html$1(m().headerB)}</dd> <dt>body alpha</dt> <dd data-testid="uf-body-alpha">${escape_html$1(m().bodyAlpha)}</dd> <dt>body beta</dt> <dd data-testid="uf-body-beta">${escape_html$1(m().bodyBeta)}</dd> <dt>form body</dt> <dd data-testid="uf-form">${escape_html$1(m().form)}</dd> <dt>cached</dt> <dd data-testid="uf-cached">${escape_html$1(m().cached)}</dd> <dt>rendered page</dt> <dd data-testid="uf-nested">${escape_html$1(m().nested)}</dd></dl>`);
			else $$renderer.push(`<!--[-1--><pre data-testid="uf-probe">${escape_html$1(JSON.stringify(data.probe))}</pre>`);
			$$renderer.push(`<!--]-->`);
		});
	}
	function _page($$renderer, $$props) {
		$$renderer.component(($$renderer) => {
			let { data } = $$props;
			$$renderer.push(`<h1 data-testid="title">Universal fetch without a script</h1> <pre data-testid="uf-probe">${escape_html$1(JSON.stringify(data.probe))}</pre>`);
		});
	}
	var componentLoaders = [
		() => _layout$2,
		() => Error$1,
		() => _layout$1,
		() => _error$3,
		() => _error$2,
		() => _error$1,
		() => _error,
		() => _layout,
		() => _page$58,
		() => _page$57,
		() => _page$56,
		() => _page$55,
		() => _page$54,
		() => _page$53,
		() => _page$52,
		() => _page$51,
		() => _page$50,
		() => _page$49,
		() => _page$48,
		() => _page$47,
		() => _page$46,
		() => _page$45,
		() => _page$44,
		() => _page$43,
		() => _page$42,
		() => _page$41,
		() => _page$40,
		() => _page$39,
		() => _page$38,
		() => _page$37,
		() => _page$36,
		() => _page$35,
		() => _page$34,
		() => _page$33,
		() => _page$32,
		() => _page$31,
		() => _page$30,
		() => _page$29,
		() => _page$28,
		() => _page$27,
		() => _page$26,
		() => _page$25,
		() => _page$24,
		() => _page$23,
		() => _page$22,
		() => _page$21,
		() => _page$20,
		() => _page$19,
		() => _page$18,
		() => _page$17,
		() => _page$16,
		() => _page$15,
		() => _page$14,
		() => _page$13,
		() => _page$12,
		() => _page$11,
		() => _page$10,
		() => _page$9,
		() => _page$8,
		() => _page$7,
		() => _page$6,
		() => _page$5,
		() => _page$4,
		() => _page$3,
		() => _page$2,
		() => _page$1,
		() => _page
	];
	var universalLoaders = [
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		() => load$6,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		() => void 0,
		() => void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		() => load$5,
		() => load$4,
		() => load$3,
		void 0,
		void 0,
		void 0,
		() => load$2,
		void 0,
		() => void 0,
		() => void 0,
		() => load$1,
		void 0,
		void 0,
		() => void 0,
		() => void 0,
		() => void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		void 0,
		() => load,
		() => load
	];
	init_hooks();
	var transport = transport$1 ?? {};
	/**
	* When inside a double-quoted attribute value, only `&` and `"` hold special meaning.
	* @see https://html.spec.whatwg.org/multipage/parsing.html#attribute-value-(double-quoted)-state
	* @type {Record<string, string>}
	*/
	var escape_html_attr_dict = {
		"&": "&amp;",
		"\"": "&quot;"
	};
	/**
	* @type {Record<string, string>}
	*/
	var escape_html_dict = {
		"&": "&amp;",
		"<": "&lt;"
	};
	/** @param {Record<string, string>} dict */
	var escape_regex = (dict) => new RegExp(`[${Object.keys(dict).join("")}]|\\p{Surrogate}`, "gu");
	var escape_html_attr_regex = escape_regex(escape_html_attr_dict);
	var escape_html_regex = escape_regex(escape_html_dict);
	/**
	* Escapes unpaired surrogates (which are allowed in js strings but invalid in HTML) and
	* escapes characters that are special.
	*
	* @param {string} str
	* @param {boolean} [is_attr]
	* @returns {string} escaped string
	* @example const html = `<tag data-value="${escape_html('value', true)}">...</tag>`;
	*/
	function escape_html(str, is_attr) {
		const dict = is_attr ? escape_html_attr_dict : escape_html_dict;
		return str.replace(is_attr ? escape_html_attr_regex : escape_html_regex, (match) => dict[match] ?? `&#${match.charCodeAt(0)};`);
	}
	/**
	* Hash using djb2
	* @param {import('types').StrictBody[]} values
	*/
	function hash(...values) {
		let hash = 5381;
		for (const value of values) if (typeof value === "string") {
			let i = value.length;
			while (i) hash = hash * 33 ^ value.charCodeAt(--i);
		} else if (ArrayBuffer.isView(value)) {
			const buffer = new Uint8Array(value.buffer, value.byteOffset, value.byteLength);
			let i = buffer.length;
			while (i) hash = hash * 33 ^ buffer[--i];
		} else throw new TypeError("value must be a string or TypedArray");
		return (hash >>> 0).toString(36);
	}
	/**
	* Hash of the headers and body a `fetch` was called with. The server-side serializer and the
	* client-side cache lookup must produce identical values for cached responses to be found.
	* @param {HeadersInit | undefined} headers
	* @param {import('types').StrictBody | null | undefined} body
	*/
	function hash_request(headers, body) {
		/** @type {import('types').StrictBody[]} */
		const values = [];
		if (headers) values.push([...new Headers(headers)].join(","));
		if (body) values.push(body);
		return hash(...values);
	}
	/**
	* Inside a script element, only `<\/script` and `<!--` hold special meaning to the HTML parser.
	*
	* The first closes the script element, so everything after is treated as raw HTML.
	* The second disables further parsing until `-->`, so the script element might be unexpectedly
	* kept open up until an unrelated HTML comment in the page.
	*
	* U+2028 LINE SEPARATOR and U+2029 PARAGRAPH SEPARATOR are escaped for the sake of pre-2018
	* browsers.
	*
	* @see tests for unsafe parsing examples.
	* @see https://html.spec.whatwg.org/multipage/scripting.html#restrictions-for-contents-of-script-elements
	* @see https://html.spec.whatwg.org/multipage/syntax.html#cdata-rcdata-restrictions
	* @see https://html.spec.whatwg.org/multipage/parsing.html#script-data-state
	* @see https://html.spec.whatwg.org/multipage/parsing.html#script-data-double-escaped-state
	* @see https://github.com/tc39/proposal-json-superset
	* @type {Record<string, string>}
	*/
	var replacements = {
		"<": "\\u003C",
		"\u2028": "\\u2028",
		"\u2029": "\\u2029"
	};
	var pattern = new RegExp(`[${Object.keys(replacements).join("")}]`, "g");
	/**
	* Generates a raw HTML string containing a safe script element carrying data and associated attributes.
	*
	* It escapes all the special characters needed to guarantee the element is unbroken, but care must
	* be taken to ensure it is inserted in the document at an acceptable position for a script element,
	* and that the resulting string isn't further modified.
	*
	* @param {import('./types.js').Fetched} fetched
	* @param {(name: string, value: string) => boolean} filter
	* @param {boolean} [prerendering]
	* @returns {string} The raw HTML of a script element carrying the JSON payload.
	* @example const html = serialize_data('/data.json', null, { foo: 'bar' });
	*/
	function serialize_data(fetched, filter, prerendering = false) {
		/** @type {Record<string, string>} */
		const headers = {};
		let cache_control = null;
		let age = null;
		let varyAny = false;
		for (const [key, value] of fetched.response.headers) {
			if (filter(key, value)) headers[key] = value;
			if (key === "cache-control") cache_control = value;
			else if (key === "age") age = value;
			else if (key === "vary" && value.trim() === "*") varyAny = true;
		}
		const payload = {
			status: fetched.response.status,
			statusText: fetched.response.statusText,
			headers,
			body: fetched.response_body
		};
		const safe_payload = JSON.stringify(payload).replace(pattern, (match) => replacements[match]);
		const attrs = [
			"type=\"application/json\"",
			"data-sveltekit-fetched",
			`data-url="${escape_html(fetched.url, true)}"`
		];
		if (fetched.is_b64) attrs.push("data-b64");
		if (fetched.request_headers || fetched.request_body) attrs.push(`data-hash="${hash_request(fetched.request_headers, fetched.request_body)}"`);
		if (!prerendering && fetched.method === "GET" && cache_control && !varyAny) {
			const match = /s-maxage=(\d+)/g.exec(cache_control) ?? /max-age=(\d+)/g.exec(cache_control);
			if (match) {
				const ttl = +match[1] - +(age ?? "0");
				attrs.push(`data-ttl="${ttl}"`);
			}
		}
		return `<script ${attrs.join(" ")}>${safe_payload}<\/script>`;
	}
	var NULL_BODY_STATUS = [
		101,
		103,
		204,
		205,
		304
	];
	/**
	* @param {Pick<import('@sveltejs/kit').RequestEvent, 'fetch' | 'url' | 'request' | 'route'>} event
	* @param {import('types').PrerenderOptions | undefined} prerendering
	* @param {import('./types.js').Fetched[]} fetched
	* @param {boolean} csr
	* @param {Pick<Required<import('@sveltejs/kit/hooks').ResolveOptions>, 'filterSerializedResponseHeaders'>} resolve_opts
	* @returns {typeof fetch}
	*/
	function create_universal_fetch(event, prerendering, fetched, csr, resolve_opts) {
		/**
		* @param {URL | RequestInfo} input
		* @param {RequestInit} [init]
		*/
		const universal_fetch = async (input, init) => {
			const cloned_body = input instanceof Request && input.body ? input.clone().body : null;
			const cloned_headers = input instanceof Request && [...input.headers].length ? new Headers(input.headers) : init?.headers;
			let response = await event.fetch(input, init);
			const url = new URL(input instanceof Request ? input.url : input, event.url);
			const same_origin = url.origin === event.url.origin;
			/** @type {import('types').PrerenderDependency} */
			let dependency;
			if (same_origin) {
				if (prerendering) {
					dependency = {
						response,
						body: null
					};
					prerendering.dependencies.set(url.pathname, dependency);
				}
			} else if (url.protocol === "https:" || url.protocol === "http:") {
				if ((input instanceof Request ? input.mode : init?.mode ?? "cors") === "no-cors") response = new Response("", {
					status: response.status,
					statusText: response.statusText,
					headers: response.headers
				});
				else {
					const acao = response.headers.get("access-control-allow-origin");
					if (!acao || acao !== event.url.origin && acao !== "*") load_fetch_cors({ reason: acao ? "Incorrect" : "No" });
				}
			}
			/** @type {ReadableStream<Uint8Array>} */
			let teed_body;
			const proxy = new Proxy(response, { get(response, key, receiver) {
				/**
				* @param {string | undefined} body
				* @param {boolean} is_b64
				*/
				async function push_fetched(body, is_b64) {
					const status_number = Number(response.status);
					if (isNaN(status_number)) throw new Error(`response.status is not a number. value: "${response.status}" type: ${typeof response.status}`);
					const request_body = input instanceof Request && cloned_body ? await new Response(cloned_body).text() : init?.body;
					if (request_body && typeof request_body !== "string" && !ArrayBuffer.isView(request_body)) return;
					fetched.push({
						url: fetch_cache_url(url, event.url),
						method: event.request.method,
						request_body,
						request_headers: cloned_headers,
						response_body: body,
						response,
						is_b64
					});
				}
				if (key === "body") {
					if (response.body === null) return null;
					if (teed_body) return teed_body;
					const [a, b] = response.body.tee();
					(async () => {
						const result = new Uint8Array(await new Response(a).arrayBuffer());
						if (dependency) dependency.body = new Uint8Array(result);
						push_fetched(base64_encode$1(result), true);
					})().catch(noop$2);
					return teed_body = b;
				}
				if (key === "arrayBuffer") return async () => {
					const buffer = await response.arrayBuffer();
					const bytes = new Uint8Array(buffer);
					if (dependency) dependency.body = bytes;
					if (buffer instanceof ArrayBuffer) await push_fetched(base64_encode$1(bytes), true);
					return buffer;
				};
				async function text() {
					const body = await response.text();
					if (body === "" && NULL_BODY_STATUS.includes(response.status)) {
						await push_fetched(void 0, false);
						return;
					}
					if (!body || typeof body === "string") await push_fetched(body, false);
					if (dependency) dependency.body = body;
					return body;
				}
				if (key === "text") return text;
				if (key === "json") return async () => {
					const body = await text();
					return body ? JSON.parse(body) : void 0;
				};
				const value = Reflect.get(response, key, response);
				if (value instanceof Function) return Object.defineProperties(
					/**
					* @this {any}
					*/
					function() {
						return Reflect.apply(value, this === receiver ? response : this, arguments);
					},
					{
						name: { value: value.name },
						length: { value: value.length }
					}
				);
				return value;
			} });
			if (csr) {
				const get = response.headers.get;
				response.headers.get = (key) => {
					const lower = key.toLowerCase();
					const value = get.call(response.headers, lower);
					if (value && !lower.startsWith("x-sveltekit-")) {
						if (!resolve_opts.filterSerializedResponseHeaders(lower, value)) load_response_header_not_serialized({
							name: lower,
							id: event.route.id
						});
					}
					return value;
				};
				const get_set_cookie = response.headers.getSetCookie;
				response.headers.getSetCookie = () => {
					const values = get_set_cookie.call(response.headers);
					for (const value of values) if (!resolve_opts.filterSerializedResponseHeaders("set-cookie", value)) load_response_header_not_serialized({
						name: "set-cookie",
						id: event.route.id
					});
					return values;
				};
			}
			return proxy;
		};
		return (input, init) => {
			const response = universal_fetch(input, init);
			response.catch(noop$2);
			return response;
		};
	}
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
		if (error instanceof SvelteKitError) return {
			status: error.status,
			message: error.text
		};
		if (error instanceof ValidationError) return {
			status: 400,
			message: "Bad Request"
		};
		return {
			status: 500,
			message: "Internal Error"
		};
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
	var bytes_to_base64 = (bytes) => {
		let binary = "";
		for (let i = 0; i < bytes.length; i += 32768) binary += String.fromCharCode.apply(null, bytes.subarray(i, i + 32768));
		return btoa(binary);
	};
	var base64_to_bytes = (encoded) => {
		const binary = atob(encoded);
		const bytes = new Uint8Array(binary.length);
		for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i);
		return bytes;
	};
	var NULL_BODY = [
		101,
		204,
		205,
		304
	];
	/**
	* Every response header a fetch answered with, and whether the app's
	* `filterSerializedResponseHeaders` lets Go replay it into the document. Go
	* evaluates that filter, so the one kit hands its serializer answers from what
	* Go said: the same name and value always get the same answer.
	*
	* @param {Set<string>} serialized
	*/
	function resolve_options(serialized) {
		return { filterSerializedResponseHeaders: (name, value) => serialized.has(name + ":" + value) };
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
		return async function(input, init) {
			const request = input instanceof Request ? input : new Request(new URL(input, url).href, init);
			const target = new URL(request.url, url);
			const headers = {};
			request.headers.forEach((value, name) => {
				headers[name] = value;
			});
			const envelope = {
				method: request.method || "GET",
				url: target.href,
				headers,
				credentials: request.credentials,
				mode: request.mode
			};
			if (request.body !== null) {
				const bytes = new Uint8Array(await request._consume());
				if (bytes.length) envelope.bodyBase64 = bytes_to_base64(bytes);
			}
			const answer = JSON.parse(await globalThis.__skgo_fetch(JSON.stringify(envelope)));
			if (answer.error) throw new TypeError(answer.error);
			const value = answer.response;
			for (const [name, header, allowed] of value.headers ?? []) if (allowed) serialized.add(name + ":" + header);
			let body = null;
			if (!NULL_BODY.includes(value.status) && request.method !== "HEAD") body = value.bodyBase64 ? base64_to_bytes(value.bodyBase64) : value.body ?? "";
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
				getAll: () => Object.entries(req.cookies ?? {}).map(([name, value]) => ({
					name,
					value
				})),
				set: () => {},
				delete: () => {},
				serialize: () => ""
			},
			fetch: create_fetch(req, url, serialized),
			getClientAddress: () => __skgo_client_address(),
			locals: {},
			params: req.params ?? {},
			platform: void 0,
			request: {
				headers: new Headers(req.headers),
				method: req.method ?? "GET"
			},
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
		return parse$1(node.data, {
			...decoders,
			Promise: () => new Promise(() => {})
		});
	}
	async function build_props(req, url, event, state, fetched, serialized) {
		const form = req.form ? parse$1(req.form, decoders) : null;
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
		const error_components = await Promise.all((req.error_components ?? []).map(async (i) => {
			try {
				return i == null ? void 0 : await componentLoaders[i]?.();
			} catch {
				return;
			}
		}));
		const props = new Props({
			page,
			tree: new RenderNode(await componentLoaders[branch[0].node]?.(), void 0),
			form,
			error: req.error ?? void 0
		});
		let current_node = props.tree;
		let data = props.page.data;
		for (let i = 0; i < branch.length; i += 1) {
			const server_data = node_data(branch[i]);
			const load = await universalLoaders[branch[i].node]?.();
			const parent_data = data;
			const result = load ? await with_request_store({
				event,
				state
			}, () => load.call(null, {
				url,
				params: req.params ?? {},
				data: server_data,
				route: { id: req.route_id ?? null },
				parent: async () => parent_data,
				depends: () => {},
				untrack: (fn) => fn(),
				fetch: create_universal_fetch(event, void 0, fetched, req.csr, resolve_options(serialized)),
				setHeaders: () => {
					throw new Error("skgo: universal load setHeaders is not supported during rendering");
				}
			})) : server_data;
			if (result != null && Object.getPrototypeOf(result) !== Object.prototype) throw new Error("skgo: universal load must return a plain object");
			data = {
				...data,
				...result
			};
			current_node.data = data;
			if (i < branch.length - 1) current_node = current_node.child = new RenderNode(await componentLoaders[branch[i + 1].node]?.(), error_components[i + 1]);
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
		const first_slash = seed.id.indexOf("/");
		const second_slash = seed.id.indexOf("/", first_slash + 1);
		const base_id = second_slash === -1 ? seed.id : seed.id.slice(0, second_slash);
		const key_json = second_slash === -1 ? void 0 : seed.id.slice(second_slash + 1);
		for (const instance of globalThis.__skgo_forms ?? []) {
			if (!instance.__ || instance.__.id !== base_id) continue;
			const target = key_json === void 0 ? instance : instance.for(JSON.parse(key_json));
			(state.remote.data ??= /* @__PURE__ */ new Map()).set(target.__, { "": parse$2(seed.output) });
			return;
		}
		throw new Error("skgo: no form is registered as " + seed.id);
	}
	/**
	* Renders one page. The result object is filled in as the promise chain
	* settles; the host drains the job queue when this call returns, so done is
	* true by then or the render never finished — which is a Go error, not a
	* partial document.
	*/
	globalThis.__skgo_render = function(req_json) {
		const result = {
			done: false,
			failure: "",
			redirect: null,
			status: 200,
			error: null,
			head: "",
			body: "",
			fetched: ""
		};
		try {
			const req = JSON.parse(req_json);
			const url = new URL(req.url);
			const state = make_state(req);
			const serialized = /* @__PURE__ */ new Set();
			const event = make_event(req, url, serialized);
			const fetched = [];
			const promise = build_props(req, url, event, state, fetched, serialized).then((props) => {
				result.status = props.page.status;
				result.error = req.error ?? null;
				const options = {
					context: /* @__PURE__ */ new Map([["__request__", { page: props.page }]]),
					csp: req.csp.nonce ? { nonce: req.csp.nonce } : { hash: !!req.csp.hash },
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
				return with_request_store({
					event,
					state
				}, () => {
					seed_form(req, state);
					return render(Root, {
						...options,
						props
					});
				});
			});
			Promise.resolve(promise).then((rendered) => {
				result.head = rendered.head;
				result.body = rendered.body;
				result.fetched = fetched.map((item) => serialize_data(item, resolve_options(serialized).filterSerializedResponseHeaders)).join("\n			");
				result.done = true;
			}, (err) => {
				if (err instanceof Redirect) result.redirect = {
					status: err.status,
					location: err.location
				};
				else if (err instanceof HandledHttpError) result.handled_error = err.body;
				else result.failure = err && (err.stack || err.message) || String(err);
				result.done = true;
			});
		} catch (err) {
			if (err instanceof Redirect) result.redirect = {
				status: err.status,
				location: err.location
			};
			else if (err instanceof HandledHttpError) result.handled_error = err.body;
			else result.failure = err && (err.stack || err.message) || String(err);
			result.done = true;
		}
		return result;
	};
	globalThis.__skgo_ping = function() {
		return "ok";
	};
	//#endregion
})();
