// SPIKE. The web globals kit's runtime touches at module-evaluation time that
// a bare ECMAScript engine does not have. Deliberately minimal and inside the
// bundle, so the Node oracle and goja execute the identical code.
//
// Forced by:
//   kit/src/runtime/utils.js:3   `export const text_encoder = new TextEncoder()`
//   kit/src/runtime/utils.js:45  `export const text_decoder = new TextDecoder()`
//   kit/src/runtime/utils.js:*   base64_encode/base64_decode -> btoa/atob
// All three are on the module's top level, so they run whether or not a render
// ever reaches them.

if (typeof globalThis.TextEncoder === 'undefined') {
	globalThis.TextEncoder = class TextEncoder {
		get encoding() {
			return 'utf-8';
		}
		/** @param {string} str */
		encode(str) {
			const bytes = [];
			for (let i = 0; i < str.length; i++) {
				let code = str.charCodeAt(i);
				if (code >= 0xd800 && code <= 0xdbff && i + 1 < str.length) {
					const next = str.charCodeAt(i + 1);
					if (next >= 0xdc00 && next <= 0xdfff) {
						code = (code - 0xd800) * 0x400 + next - 0xdc00 + 0x10000;
						i++;
					}
				}
				if (code < 0x80) bytes.push(code);
				else if (code < 0x800) bytes.push(0xc0 | (code >> 6), 0x80 | (code & 0x3f));
				else if (code < 0x10000)
					bytes.push(0xe0 | (code >> 12), 0x80 | ((code >> 6) & 0x3f), 0x80 | (code & 0x3f));
				else
					bytes.push(
						0xf0 | (code >> 18),
						0x80 | ((code >> 12) & 0x3f),
						0x80 | ((code >> 6) & 0x3f),
						0x80 | (code & 0x3f)
					);
			}
			return new Uint8Array(bytes);
		}
	};
}

if (typeof globalThis.TextDecoder === 'undefined') {
	globalThis.TextDecoder = class TextDecoder {
		get encoding() {
			return 'utf-8';
		}
		/** @param {ArrayBufferView | ArrayBuffer} input */
		decode(input) {
			const bytes =
				input instanceof Uint8Array
					? input
					: new Uint8Array(input.buffer ?? input, input.byteOffset ?? 0, input.byteLength);
			let out = '';
			for (let i = 0; i < bytes.length; ) {
				const b = bytes[i];
				let code, len;
				if (b < 0x80) (code = b), (len = 1);
				else if (b < 0xe0) (code = b & 0x1f), (len = 2);
				else if (b < 0xf0) (code = b & 0x0f), (len = 3);
				else (code = b & 0x07), (len = 4);
				for (let j = 1; j < len; j++) code = (code << 6) | (bytes[i + j] & 0x3f);
				i += len;
				if (code > 0xffff) {
					code -= 0x10000;
					out += String.fromCharCode(0xd800 + (code >> 10), 0xdc00 + (code & 0x3ff));
				} else {
					out += String.fromCharCode(code);
				}
			}
			return out;
		}
	};
}

const B64 = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/';

if (typeof globalThis.btoa === 'undefined') {
	globalThis.btoa = (binary) => {
		let out = '';
		for (let i = 0; i < binary.length; i += 3) {
			const a = binary.charCodeAt(i);
			const b = binary.charCodeAt(i + 1);
			const c = binary.charCodeAt(i + 2);
			out += B64[a >> 2];
			out += B64[((a & 3) << 4) | (isNaN(b) ? 0 : b >> 4)];
			out += isNaN(b) ? '=' : B64[((b & 15) << 2) | (isNaN(c) ? 0 : c >> 6)];
			out += isNaN(c) ? '=' : B64[c & 63];
		}
		return out;
	};
}

if (typeof globalThis.atob === 'undefined') {
	globalThis.atob = (b64) => {
		const clean = b64.replace(/=+$/, '');
		let out = '';
		let bits = 0;
		let acc = 0;
		for (const ch of clean) {
			const v = B64.indexOf(ch);
			if (v < 0) continue;
			acc = (acc << 6) | v;
			bits += 6;
			if (bits >= 8) {
				bits -= 8;
				out += String.fromCharCode((acc >> bits) & 0xff);
			}
		}
		return out;
	};
}

// `URL` is reached at module-evaluation time by kit's public export surface:
//   kit/src/utils/url.js:9  `const internal = new URL('a://')`
// which `@sveltejs/kit`'s `error()` pulls in, which the remote-function
// wrappers import. Nothing in a page render calls through it, but the module
// will not evaluate without a constructor. This is a small absolute/relative
// parser, not a WHATWG-conformant one — the real request URL is parsed by Go
// and handed to the render already split.
if (typeof globalThis.URL === 'undefined') {
	const ABS = /^([a-zA-Z][a-zA-Z0-9+.-]*:)\/\/([^/?#]*)([^?#]*)(\?[^#]*)?(#.*)?$/;
	const SCHEME_ONLY = /^([a-zA-Z][a-zA-Z0-9+.-]*:)(.*)$/;

	globalThis.URL = class URL {
		constructor(input, base) {
			let href = String(input);
			if (base !== undefined && !SCHEME_ONLY.test(href)) {
				const b = base instanceof URL ? b : new URL(String(base));
				if (href.startsWith('//')) href = b.protocol + href;
				else if (href.startsWith('/')) href = b.origin + href;
				else {
					const dir = b.pathname.slice(0, b.pathname.lastIndexOf('/') + 1);
					const parts = (dir + href).split('/');
					const out = [];
					for (const p of parts) {
						if (p === '.') continue;
						if (p === '..') out.pop();
						else out.push(p);
					}
					href = b.origin + out.join('/');
				}
			}

			const m = ABS.exec(href);
			if (m) {
				this.protocol = m[1];
				this.host = m[2];
				this.hostname = m[2].split(':')[0];
				this.port = m[2].includes(':') ? m[2].split(':')[1] : '';
				this.pathname = m[3] || '/';
				this.search = m[4] || '';
				this.hash = m[5] || '';
				this.origin = this.protocol + '//' + this.host;
			} else {
				const s = SCHEME_ONLY.exec(href);
				if (!s) throw new TypeError(`Invalid URL: ${href}`);
				this.protocol = s[1];
				this.host = '';
				this.hostname = '';
				this.port = '';
				this.pathname = s[2];
				this.search = '';
				this.hash = '';
				this.origin = 'null';
			}
			this.searchParams = new Map();
			for (const pair of this.search.replace(/^\?/, '').split('&')) {
				if (!pair) continue;
				const i = pair.indexOf('=');
				this.searchParams.set(
					decodeURIComponent(i < 0 ? pair : pair.slice(0, i)),
					i < 0 ? '' : decodeURIComponent(pair.slice(i + 1))
				);
			}
		}
		get href() {
			return this.origin === 'null'
				? this.protocol + this.pathname + this.search + this.hash
				: this.origin + this.pathname + this.search + this.hash;
		}
		toString() {
			return this.href;
		}
	};
}
