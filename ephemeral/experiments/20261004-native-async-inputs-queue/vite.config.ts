import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import { appendFileSync } from 'node:fs';
export default defineConfig({ plugins: [sveltekit({
 adapter: { name: 'native-inputs-proof', adapt() {} },
 paths: { origin: 'http://127.0.0.1:19341' },
 experimental: { remoteFunctions: true },
 compilerOptions: { experimental: { async: true } },
 prerender: { handleHttpError: 'fail' }
	})] });