import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import skgo from './skgo-adapter.js';

export default defineConfig({
	plugins: [
		sveltekit({
			adapter: skgo(),
			// Kit 3 fixes the app's origin at build time.
			paths: { origin: process.env.ORIGIN ?? 'http://127.0.0.1:8080' }
		})
	]
});
