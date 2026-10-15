// Development uses Kit's installed scanner and generated client graph. Go owns
// application requests; this plugin serves tooling and builds the embedded renderer.
import { createRequire } from "node:module";
import { createHash, randomUUID } from "node:crypto";
import {
  existsSync,
  cpSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  realpathSync,
  renameSync,
  statSync,
  writeFileSync,
} from "node:fs";
import { dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { nodeTable, goEnvironmentValues } from "./env.js";
import { validateGenerated } from "./generated.js";
const { createBuilder } = await import(
  pathToFileURL(
    createRequire(join(process.cwd(), "package.json")).resolve("vite"),
  ).href
);
const ownerKey = Symbol.for("skgo.development.renderer");
export function rendererOwner(goja) {
  globalThis[ownerKey] = goja;
}

// The generated env declaration keeps its ordinary import. Kit resolves it
// through its own aliases into this session's private validated values.
export function developmentEnvironment(config) {
  const file = join(process.env.SKGO_DEV_DIR, "skgo-env-values.js");
  mkdirSync(dirname(file), { recursive: true });
  if (!existsSync(file))
    writeFileSync(file, "export const values = {};\n", { mode: 0o600 });
  config.alias = { ...config.alias, "../.svelte-kit/skgo-env-values.js": file };
  return goEnvironmentValues("build", file);
}

export function development({ config }, identity, hooks, globalName) {
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
  const instance = randomUUID();
  const environment = developmentEnvironment(config);
  let renderer,
    builder,
    pending,
    generation = 0,
    latest;
  const tooling = [
    "/@",
    "/src/",
    "/node_modules/",
    "/__skgo/dev/",
    `/@fs`,
    "/.skgo-vite/",
  ];
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
    const revision = hash();
    environment.refresh(server.config);
    if (latest?.revision === revision) return latest;
    const { default: scan } = await import(
      pathToFileURL(join(kit, "src/core/sync/create_manifest_data/index.js"))
        .href
    );
    const sync = await import(
      pathToFileURL(join(kit, "src/core/sync/sync.js")).href
    );
    const { get_global_name } = await import(
      pathToFileURL(join(kit, "src/core/utils.js")).href
    );
    const data = scan(config, root);
    sync.create(config, root, data, false);
    if (!builder) {
      const old = process.env.SKGO_DEV_RENDER_BUILD,
        mode = process.env.NODE_ENV;
      process.env.SKGO_DEV_RENDER_BUILD = "1";
      process.env.NODE_ENV = "development";
      try {
        builder = await createBuilder({
          configFile: server.config.configFile,
          mode: "development",
          logLevel: "warn",
        });
        renderer = globalThis[ownerKey];
      } finally {
        if (old === undefined) delete process.env.SKGO_DEV_RENDER_BUILD;
        else process.env.SKGO_DEV_RENDER_BUILD = old;
        if (mode === undefined) delete process.env.NODE_ENV;
        else process.env.NODE_ENV = mode;
      }
    }
    sync.create(config, root, data, false);
    if (existsSync(join(root, "skgo.env.json"))) {
      const { resolve_env_entry } = await import(
        pathToFileURL(join(kit, "src/core/env.js")).href
      );
      await sync.env(
        config,
        resolve_env_entry(config, root),
        root,
        "development",
      );
    }
    const { write_app_manifest } = await import(
      pathToFileURL(join(kit, "src/core/sync/write_app_manifest.js")).href
    );
    write_app_manifest(join(config.outDir, "generated/build"), data, false);
    const dist = join(work, "generations", String(++generation));
    mkdirSync(join(dist, "ssr"), { recursive: true });
    mkdirSync(join(dist, "client"), { recursive: true });
    if (existsSync(config.files.assets))
      cpSync(config.files.assets, join(dist, "client"), { recursive: true });
    writeFileSync(
      join(dist, "env.json"),
      JSON.stringify(environment.artifact()),
    );
    await renderer.build(
      { nodes: nodeTable(root, data.nodes), hooks: hooks({ config }) },
      join(dist, "ssr/bundle.js"),
      builder,
    );
    const generated = validateGenerated(
      JSON.parse(readFileSync(join(root, "skgo.remotes.json"), "utf8")),
    );
    const routes = data.routes
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
      }));
    const manifest = {
      development: true,
      skgo: identity.version,
      skgoAdapter: identity.adapter,
      appDir: config.appDir,
      base: config.paths.base,
      version: config.version.name,
      trustedOrigins: config.csrf.trustedOrigins,
      nodes: data.nodes.map((n) => n.server ?? ""),
      loads: generated.loads,
      actions: generated.actions,
      remotes: generated.remotes,
      routes,
      prerendered: [],
      precompressed: false,
      ssr: {
        bundle: "ssr/bundle.js",
        template: "app.html",
        errorTemplate: "error.html",
        target: "es2022",
        globalName: get_global_name(config.version.name, true),
        assets: config.paths.assets,
        relative: config.paths.relative,
        csp: config.csp,
        client: {
          usesEnvDynamicPublic: environment.artifact().fields.some(field => field.public && !field.static),
          start: `/@fs${kit}/src/runtime/client/entry.js`,
          app: `/@fs${config.outDir}/generated/dev/client/app.js`,
        },
        nodes: data.nodes.map((n, index) => ({
          index,
          component: !!n.component,
          ssr: n.page_options?.ssr ?? null,
          csr: n.page_options?.csr ?? null,
        })),
      },
    };
    writeFileSync(
      join(dist, "app.html"),
      readFileSync(config.files.appTemplate, "utf8").replace(
        "%sveltekit.head%",
        '%sveltekit.head%<script type="module" src="/__skgo/dev/client.js"></script>',
      ),
    );
    writeFileSync(
      join(dist, "error.html"),
      existsSync(config.files.errorTemplate)
        ? readFileSync(config.files.errorTemplate)
        : "<h1>%sveltekit.status% %sveltekit.error.message%</h1>",
    );
    writeFileSync(join(dist, "index.html"), "");
    writeFileSync(join(dist, "skgo.manifest.json"), JSON.stringify(manifest));
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
    if (hash() !== revision)
      throw new Error("skgo: frontend sources changed during snapshot; retry");
    latest = { dist, metadata, revision };
    return latest;
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
            if (id !== "\0skgo:development-client") return;
            return `import {refreshAll} from '$app/navigation';
import {ErrorOverlay} from '/@vite/client';
import.meta.hot.on('skgo:go-ready',async(message)=>{for(const overlay of document.querySelectorAll('vite-error-overlay'))if(overlay instanceof ErrorOverlay)overlay.close();if(!message.reload)await refreshAll();});`;
          },
          configureServer(server) {
            const hot = server.environments.client.hot,
              send = hot.send.bind(hot);
            hot.send = (payload, ...args) => {
              if (payload?.type === "full-reload") return;
              return send(payload, ...args);
            };
            server.middlewares.use("/__skgo/dev", async (req, res, next) => {
              if (req.url === "/client.js") return next();
              try {
                res.setHeader("Content-Type", "application/json");
                if (req.url === "/snapshot") {
                  pending ??= snapshot(server).finally(() => (pending = null));
                  res.end(JSON.stringify(await pending));
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
                  else {
                    server.hot.send({
                      type: "custom",
                      event: "skgo:go-ready",
                      data: message,
                    });
                    if (message.reload) {
                      server.environments.client.moduleGraph.invalidateAll();
                      send({ type: "full-reload" });
                    }
                  }
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
            const ready = () => {
              const address = server.httpServer.address();
              const info = {
                instance,
                url: `http://127.0.0.1:${address.port}`,
                dist: join(work, "generations"),
                tooling,
              };
              mkdirSync(dirname(process.env.SKGO_DEV_READY), {
                recursive: true,
              });
              writeFileSync(
                process.env.SKGO_DEV_READY + ".tmp",
                JSON.stringify(info),
              );
              renameSync(
                process.env.SKGO_DEV_READY + ".tmp",
                process.env.SKGO_DEV_READY,
              );
            };
            if (server.httpServer.listening) ready();
            else server.httpServer.once("listening", ready);
          },
        },
      ],
    },
  };
}
