import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite-plus';
import { appendFileSync } from 'node:fs';
export default defineConfig({ plugins: [sveltekit({
 adapter: { name: 'native-inputs-proof', adapt() {} },
 paths: { origin: 'https://prerender-proof.invalid' },
 experimental: { remoteFunctions: true },
 compilerOptions: { experimental: { async: true } },
 prerender: { handleHttpError: 'fail' }
	})] });