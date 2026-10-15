// Native Kit owns development rendering and the live module graph.
import { createHash } from "node:crypto";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { identity } from "./identity.js";
import { validateGenerated } from "./generated.js";
import { goEnvironmentValues } from "./env.js";

export function developmentEnvironment(config) {
  const file = join(process.env.SKGO_DEV_DIR, "skgo-env-values.js");
  mkdirSync(dirname(file), { recursive: true });
  if (!existsSync(file))
    writeFileSync(file, "export const values = {};\n", { mode: 0o600 });
  config.alias = { ...config.alias, "../.svelte-kit/skgo-env-values.js": file };
  return goEnvironmentValues("build", file);
}
export function development({ config }) {
  const root = process.cwd(),
    work = resolve(process.env.SKGO_DEV_DIR),
    origin = new URL(process.env.SKGO_DEV_ORIGIN);
  config.outDir = join(work, "kit");
  config.paths.origin = origin.origin;
  const kit = realpathSync(join(root, "node_modules/@sveltejs/kit"));
  const version = JSON.parse(
    readFileSync(join(kit, "package.json"), "utf8"),
  ).version;
  if (version !== "3.0.0")
    throw new Error(
      `skgo development requires Kit 3.0.0, installed ${version}`,
    );
  const environment = developmentEnvironment(config);
  const hash = () => {
    const h = createHash("sha256");
    function walk(dir) {
      if (!existsSync(dir)) return;
      for (const name of readdirSync(dir).sort()) {
        const file = join(dir, name);
        if (statSync(file).isDirectory()) walk(file);
        else if (!file.endsWith(".go")) {
          h.update(file);
          h.update(readFileSync(file));
        }
      }
    }
    walk(join(root, "src"));
    walk(config.files.assets);
    for (const f of [
      "skgo.remotes.json",
      "skgo.env.json",
      ...[
        ".env",
        ".env.local",
        ".env.development",
        ".env.development.local",
      ].map((name) => resolve(root, config.env.dir, name)),
    ])
      if (existsSync(resolve(root, f))) {
        h.update(resolve(root, f));
        h.update(readFileSync(resolve(root, f)));
      }
    return h.digest("hex");
  };
  async function snapshot(server) {
    environment.refresh(server.config);
    const { default: scan } = await import(
      pathToFileURL(join(kit, "src/core/sync/create_manifest_data/index.js"))
        .href
    );
    const data = scan(config, root);
    const metadata = {
      Dirs: [],
      IDs: [],
      LoadKinds: {},
      LoadDirs: {},
      routes: [],
      names: [],
      children: {},
      Empty: false,
    };
    const pages = new Map(
      data.routes.filter((r) => r.leaf).map((r) => [r.leaf, r.id]),
    );
    for (const r of data.routes) {
      const dir = join(config.files.routes, r.id);
      metadata.Dirs.push(dir);
      metadata.IDs.push(r.id);
      metadata.routes.push(r.params);
      if (r.layout)
        metadata.children[r.id] = r.layout.child_pages.map((p) => pages.get(p));
      if (existsSync(dir)) {
        for (const f of readdirSync(dir))
          if (f === "page.server.go" || f === "layout.server.go") {
            metadata.LoadDirs[dir] = true;
            (metadata.LoadKinds[dir] ??= {})[f] = true;
          }
      }
    }
    metadata.Empty = metadata.Dirs.length === 0;
    const params = data.params ? [data.params] : [];
    for (const file of params) {
      const mod = await server.environments.ssr.runner.import(
        "/@fs" + resolve(root, file),
      );
      metadata.names.push(...Object.keys(mod.params ?? {}));
    }
    // Go constructors consume registration/routing configuration. No renderer
    // artifact or frontend node graph is published to the Go process.
    const generated = validateGenerated(
      JSON.parse(readFileSync(join(root, "skgo.remotes.json"), "utf8")),
    );
    const stamp = identity(),
      dist = join(work, "application", hash());
    const manifest = {
      development: true,
      skgo: stamp.version,
      skgoAdapter: stamp.adapter,
      appDir: config.appDir,
      base: config.paths.base,
      version: config.version.name,
      trustedOrigins: config.csrf.trustedOrigins,
      nodes: data.nodes.map((n) => n.server ?? ""),
      loads: generated.loads,
      actions: generated.actions,
      remotes: generated.remotes,
      routes: data.routes
        .filter((r) => r.page || r.endpoint)
        .map((r) => ({
          id: r.id,
          pattern: r.pattern.source,
          params: r.params,
          endpoint: r.endpoint
            ? { methods: generated.endpoints[r.id] ?? [] }
            : null,
          page: r.page
            ? {
                layouts: Array.from(r.page.layouts, (n) => n ?? -1),
                errors: Array.from(r.page.errors, (n) => n ?? -1),
                leaf: r.page.leaf,
              }
            : null,
        })),
      prerendered: [],
      precompressed: false,
    };
    mkdirSync(dist, { recursive: true });
    writeFileSync(join(dist, "skgo.manifest.json"), JSON.stringify(manifest));
    writeFileSync(join(dist, "index.html"), "");
    return {
      dist,
      metadata,
      remotePrefix: `${config.paths.base}/${config.appDir}/remote/`,
      revision: hash(),
    };
  }
  return {
    plugins: {
      post: [
        environment.plugin,
        {
          name: "skgo-development",
          config() {
            return {
              cacheDir: join(work, "vite-cache"),
              server: {
                fs: { allow: [root, work] },
                ws: {
                  path: "/__skgo/dev/hmr",
                  host: origin.hostname,
                  clientPort:
                    Number(origin.port) ||
                    (origin.protocol === "https:" ? 443 : 80),
                  protocol: origin.protocol === "https:" ? "wss" : "ws",
                },
              },
            };
          },
          resolveId(id) {
            if (id === "/__skgo/dev/client.js")
              return "\0skgo:development-client";
          },
          load(id) {
            if (id === "\0skgo:development-client")
              return `import {refreshAll} from '$app/navigation';
import {ErrorOverlay} from '/@vite/client';
import.meta.hot.on('skgo:go-ready',async()=>{for(const overlay of document.querySelectorAll('vite-error-overlay'))if(overlay instanceof ErrorOverlay)overlay.close();await refreshAll();});`;
          },
          configureServer(server) {
            server.middlewares.use("/__skgo/dev", async (req, res, next) => {
              if (req.url === "/client.js") return next();
              try {
                res.setHeader("Content-Type", "application/json");
                if (req.url === "/snapshot") {
                  res.end(JSON.stringify(await snapshot(server)));
                  return;
                }
                if (req.url === "/state") {
                  res.end(JSON.stringify({ revision: hash() }));
                  return;
                }
                if (req.url === "/published") {
                  let raw = "";
                  for await (const chunk of req) raw += chunk;
                  const message = JSON.parse(raw || "{}");
                  if (message.error)
                    server.hot.send({
                      type: "error",
                      err: { message: message.error, stack: message.error },
                    });
                  else
                    server.hot.send({
                      type: "custom",
                      event: "skgo:go-ready",
                      data: message,
                    });
                  res.end("{}");
                  return;
                }
                res.statusCode = 404;
                res.end("{}");
              } catch (error) {
                res.statusCode = 500;
                res.end(
                  JSON.stringify({ error: error.stack ?? String(error) }),
                );
              }
            });
          },
        },
      ],
    },
  };
}
