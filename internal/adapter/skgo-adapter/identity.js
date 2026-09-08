/**
 * Which skgo this adapter is.
 *
 * The adapter writes `skgo.manifest.json` and the Go binary reads it: one
 * contract in two languages, published as one npm package and one Go module at
 * one version. The Go half refuses a manifest written by any adapter but its
 * own, so the manifest has to carry a name for this one.
 *
 * The name is not written down anywhere, because a written-down name is a thing
 * that can be wrong. It is taken from the package's own bytes — the entry vite
 * imports, and every runtime file beside it — the same way, in the same order,
 * as the Go module takes it from the copy it embeds. Two identical fingerprints
 * therefore mean the files really are identical, byte for byte, and a package
 * that lost or gained a file cannot claim to be the one the binary expects.
 */

import { createHash } from 'node:crypto';
import { readdirSync, readFileSync } from 'node:fs';
import { basename, dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const here = dirname(fileURLToPath(import.meta.url));

/** The package root: the directory holding package.json and the entry. */
const root = dirname(here);

/**
 * The runtime directory's name, which is also the entry's name plus `.js` —
 * `skgo-adapter/` beside `skgo-adapter.js`. Reading it off this file's own
 * location rather than writing it twice keeps the two spellings from drifting.
 */
const dir = basename(here);

/**
 * Every runtime file, as a slash-separated path relative to the package root,
 * sorted. Sorted rather than as-read, so the fingerprint is a fact about the
 * bytes and not about the order a directory happened to be listed in.
 *
 * @param {string} at
 * @param {string} prefix
 * @returns {string[]}
 */
function runtimeFiles(at, prefix) {
	/** @type {string[]} */
	const names = [];
	for (const entry of readdirSync(at, { withFileTypes: true })) {
		const name = prefix + '/' + entry.name;
		if (entry.isDirectory()) {
			names.push(...runtimeFiles(join(at, entry.name), name));
		} else {
			names.push(name);
		}
	}
	return names.sort();
}

/**
 * The identity the manifest carries: the published version of this package, and
 * the fingerprint of the files it is made of.
 *
 * @returns {{ version: string, adapter: string }}
 */
export function identity() {
	const sum = createHash('sha256');
	sum.update(readFileSync(join(root, dir + '.js')));
	for (const name of runtimeFiles(here, dir)) {
		// The name goes in too, so that moving a file between paths changes the
		// fingerprint even when no byte of it does.
		sum.update(Buffer.from(name + '\0', 'utf-8'));
		sum.update(readFileSync(join(root, name)));
	}

	const pkg = JSON.parse(readFileSync(join(root, 'package.json'), 'utf-8'));
	return { version: pkg.version, adapter: sum.digest('hex').slice(0, 12) };
}
