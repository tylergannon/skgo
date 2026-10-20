import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vite-plus";
import skgo from "@skgo/sveltekit-adapter";
export default defineConfig({
  plugins: [
    sveltekit({
      adapter: skgo(),
      paths: { origin: "http://127.0.0.1:8080" },
      experimental: { remoteFunctions: true },
    }),
  ],
});
