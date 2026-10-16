// Vite's supported middleware server shares this private HTTP/upgrade listener.
import http from "node:http";
import { createRequire } from "node:module";
import { join } from "node:path";
import { pathToFileURL } from "node:url";
const app = createRequire(join(process.cwd(), "package.json"));
const { createServer } = await import(pathToFileURL(app.resolve("vite")).href);
let vite;
const server = http.createServer((req, res) => vite.middlewares(req, res));
vite = await createServer({
  server: { middlewareMode: { server }, ws: { server } },
});
server.listen(process.env.SKGO_DEV_VITE_SOCKET);
async function stop() {
  await vite.close();
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(0), 1000).unref();
}
process.on("SIGTERM", stop);
process.on("SIGINT", stop);
