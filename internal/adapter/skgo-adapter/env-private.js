import { resolve } from 'node:path';
import { realpathSync } from 'node:fs';

function canonical(file) {
	try { return realpathSync.native(file); } catch { return file; }
}

// Vite's raw/transform middleware can serve generated server files directly.
// Deny them before those middleware run, including absolute /@fs requests.
export function protectEnvironmentFiles({ root, kitOut, output, base = '/', token }) {
	const files = [
		resolve(root, '.svelte-kit/skgo-env-values.js'),
		resolve(root, '.svelte-kit/skgo-env-runtime.json'),
		resolve(root, kitOut, 'generated/build/env/config.js'),
		resolve(root, kitOut, 'generated/dev/env/config.js'),
		resolve(root, output, 'env.json')
	].map(canonical);
	const directories = [
		resolve(root, kitOut, 'output/server'),
		resolve(root, kitOut, 'generated/build/env/private'),
		resolve(root, kitOut, 'generated/dev/env/private'),
		resolve(root, output, 'ssr')
	].map(canonical);
	return (req, res, next) => {
		let path;
		try { path = decodeURIComponent((req.url || '').split('?')[0]); }
		catch { res.statusCode = 400; res.end('Bad request'); return; }
		if (base !== '/' && path.startsWith(base)) path = '/' + path.slice(base.length);
		if (token && /^\/__skgo_dev\/(?:module|styles)(?:\/|$)/.test(path) && req.headers['x-skgo-dev-token'] !== token) {
			res.statusCode = 403; res.end('Forbidden'); return;
		}
		let file;
		if (path.startsWith('/@id/')) {
			const id = path.slice(5).replace('__x00__', '\0');
			if (id === '$app/env/private' || id === '#app/env/private' || /<sveltekit:generated>\/env\/(?:config\.js|private(?:\/|$))/.test(id)) {
				res.statusCode = 403; res.end('Forbidden'); return;
			}
			file = resolve(id);
		} else {
			file = path.startsWith('/@fs/') ? resolve(path.slice(4)) : resolve(root, '.' + path);
		}
		file = canonical(file);
		if (files.includes(file) || directories.some(dir => file === dir || file.startsWith(dir + '/'))) {
			res.statusCode = 403; res.end('Forbidden'); return;
		}
		next();
	};
}
