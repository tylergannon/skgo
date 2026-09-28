import { createHash } from 'node:crypto';
import { readFileSync, writeFileSync } from 'node:fs';

// Kit writes streamed promise settlements after the closing HTML tag. Its
// prerendered CSP meta tag hashes the boot script but does not hash those later
// inline scripts. Add only the hashes of scripts present in the finished file.
export function authorizePrerenderedScripts(file) {
	const html = readFileSync(file, 'utf8');
	const meta = /<meta http-equiv="content-security-policy" content="([^"]*)">/i.exec(html);
	if (!meta) return;
	const additions = [];
	for (const [, attributes, body] of html.matchAll(/<script([^>]*)>([\s\S]*?)<\/script>/gi)) {
		if (/\bsrc\s*=/.test(attributes)) continue;
		const hash = `'sha256-${createHash('sha256').update(body).digest('base64')}'`;
		if (!meta[1].includes(hash)) additions.push(hash);
	}
	if (!additions.length) return;
	writeFileSync(file, html.replace(meta[0], meta[0].replace(meta[1], `${meta[1]} ${[...new Set(additions)].join(' ')}`)));
}
