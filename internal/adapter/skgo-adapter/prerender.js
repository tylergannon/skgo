import { mkdtempSync, mkdirSync, readFileSync, realpathSync, rmSync } from "node:fs";
import { createHash, randomUUID } from "node:crypto";
import { createServer, createConnection } from "node:net";
import { spawn } from "node:child_process";
import { createRequire } from "node:module";
import process from "node:process";
import { tmpdir } from "node:os";
import { dirname, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import { isMainThread } from "node:worker_threads";

const owners = new Map();
const SERVICE_URL = "SKGO_PRERENDER_SERVICE_URL";
const SERVICE_SECRET = "SKGO_PRERENDER_SERVICE_SECRET";
const CALLBACK_TIMEOUT = 30_000;
const STARTUP_TIMEOUT = 60_000;
const SHUTDOWN_GRACE = 150;
const CLEANUP_TIMEOUT = 5_000;
function appRoot() {
  return resolve(process.cwd());
}
function reportOwnerFailure(error) {
  const cleanup = error?.cleanupError ? `; cleanup failed: ${error.cleanupError.message}` : "";
  process.stderr.write(`skgo prerender owner failure: ${error?.message ?? error}${cleanup}\n`);
}
function kitRootFor(root) {
  return realpathSync(join(root, "node_modules/@sveltejs/kit"));
}

function kitQueueFailure(detail) {
  const error = new Error(
    `SKGO_KIT_PRERENDER_QUEUE: ${detail}. Run "go tool skgo kit-patch --web web --apply" from the Go app root, then "cd web && node_modules/.bin/vp install --no-frozen-lockfile", then "go tool skgo kit-patch --web web --check".`,
  );
  error.name = "SKGO_KIT_PRERENDER_QUEUE";
  error.code = "SKGO_KIT_PRERENDER_QUEUE";
  return error;
}

function verifyKitQueue(root) {
  let metadata;
  try {
    const appRequire = createRequire(join(root, "package.json"));
    const adapterPackage = appRequire.resolve("@skgo/sveltekit-adapter/package.json");
    metadata = JSON.parse(
      readFileSync(
        join(dirname(adapterPackage), "skgo-adapter/compat/kit-3.0.0-queue.json"),
        "utf8",
      ),
    );
  } catch (error) {
    throw kitQueueFailure(`adapter compatibility metadata could not be read: ${error.message}`);
  }
  try {
    const kitRoot = kitRootFor(root);
    const pkg = JSON.parse(readFileSync(join(kitRoot, "package.json"), "utf8"));
    const queuePath = join(kitRoot, metadata.queuePath);
    const queue = readFileSync(queuePath);
    const digest = createHash("sha256").update(queue).digest("hex");
    if (
      pkg.name !== metadata.package ||
      pkg.version !== metadata.version ||
      digest !== metadata.correctedSHA256
    ) {
      throw kitQueueFailure(
        `resolved ${pkg.name ?? "unknown"}@${pkg.version ?? "unknown"} queue SHA-256 ${digest}; expected ${metadata.package}@${metadata.version} with SHA-256 ${metadata.correctedSHA256}`,
      );
    }
  } catch (error) {
    if (error?.code === "SKGO_KIT_PRERENDER_QUEUE") throw error;
    throw kitQueueFailure(`resolved Kit queue could not be verified: ${error.message}`);
  }
}

function vitePlusBuildParent(root) {
  if (
    !isMainThread ||
    process.env.NODE_PACKAGE_MANAGER !== "vite-plus" ||
    process.argv[2] !== "build" ||
    !process.argv[1]
  )
    return null;
  try {
    const appRequire = createRequire(join(root, "package.json"));
    const vpRequire = createRequire(appRequire.resolve("vite-plus/package.json"));
    const expected = realpathSync(join(dirname(vpRequire.resolve("vite")), "cli.js"));
    return realpathSync(process.argv[1]) === expected && process.ppid > 1 ? process.ppid : null;
  } catch {
    return null;
  }
}

function validateEnvelope(request, raw) {
  let value;
  try {
    value = JSON.parse(raw);
  } catch (error) {
    throw new Error(`skgo prerender command returned malformed JSON: ${error.message}`);
  }
  if (!value || typeof value !== "object" || Array.isArray(value))
    throw new Error("skgo prerender command returned a non-object response");
  if (request.kind === "remote-inputs") {
    if (typeof value.inputs !== "string")
      throw new Error("skgo prerender inputs response has no devalue payload");
  } else if (request.kind === "remote") {
    if (value.type === "result" && typeof value.data === "string") return;
    if (
      value.type === "error" &&
      value.kind === "app" &&
      value.error &&
      Number.isInteger(value.error.status) &&
      typeof value.error.message === "string"
    )
      return;
    if (
      value.type === "error" &&
      value.kind === "unknown" &&
      value.error &&
      Number.isInteger(value.error.status) &&
      typeof value.error.message === "string" &&
      typeof value.diagnostic === "string"
    )
      return;
    if (
      value.type === "redirect" &&
      Number.isInteger(value.redirect?.status) &&
      value.redirect.status >= 300 &&
      value.redirect.status <= 308 &&
      typeof value.redirect.location === "string"
    )
      return;
    throw new Error("skgo prerender remote response has an invalid envelope");
  } else if (request.kind === "load") {
    const allowed = ["data", "chunks", "error", "failure", "redirect", "headers", "cookies"];
    if (!allowed.some((key) => key in value))
      throw new Error("skgo prerender load response has no recognized fields");
    if (
      value.data !== undefined &&
      typeof value.data !== "string" &&
      !(value.data && typeof value.data === "object")
    )
      throw new Error("skgo prerender load response has invalid data");
    if (value.failure !== undefined && typeof value.failure !== "string")
      throw new Error("skgo prerender load response has invalid failure");
  } else {
    throw new Error(`skgo prerender owner received unsupported request kind ${request.kind}`);
  }
}

function alive(job) {
  if (!job.child.pid) return false;
  try {
    process.kill(process.platform === "win32" ? job.child.pid : -job.child.pid, 0);
    return true;
  } catch (error) {
    if (error.code === "ESRCH") return false;
    // EPERM is an existence result for signal 0; actual termination below still
    // reports permission failures rather than mistaking this probe for a drain.
    if (error.code === "EPERM") return true;
    error.message = `skgo process group ${job.child.pid} probe failed: ${error.message}`;
    throw error;
  }
}
async function signalTree(job, force) {
  if (!job.child.pid) return;
  if (process.platform === "win32") {
    await bounded(
      new Promise((resolve, reject) => {
        const args = ["/PID", String(job.child.pid), "/T", ...(force ? ["/F"] : [])];
        const killer = spawn("taskkill", args, { stdio: "ignore", windowsHide: true });
        killer.once("error", reject);
        killer.once("close", resolve);
      }),
      CLEANUP_TIMEOUT,
      "skgo prerender taskkill did not finish",
    );
  } else {
    try {
      process.kill(-job.child.pid, force ? "SIGKILL" : "SIGTERM");
    } catch (error) {
      if (error.code === "ESRCH") return;
      // Darwin can reject signals before Node observes an exited child.
      // Retain the error; only bounded reaping and group absence prove cleanup.
      if (error.code === "EPERM") job.signalError ??= error;
      else throw error;
    }
  }
}
function delay(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}
async function bounded(promise, ms, message) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => {
        timer = setTimeout(() => reject(new Error(message)), ms);
      }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}
async function stopJob(job, cooperative = false) {
  if (cooperative) {
    job.control?.end();
  }
  if (cooperative || job.hasExited) {
    // Join the child's close before probing its just-exited process group.
    // Descendants holding inherited output pipes still hit the same fixed grace.
    await bounded(job.closed, SHUTDOWN_GRACE, "shutdown grace elapsed").catch((error) => {
      if (error.message !== "shutdown grace elapsed") throw error;
    });
  }
  if (alive(job)) {
    await signalTree(job, false);
    await delay(150);
    if (alive(job)) await signalTree(job, true);
  }
  await bounded(
    job.closed,
    CLEANUP_TIMEOUT,
    `skgo prerender owned process did not close${job.signalError ? ": " + job.signalError.message : ""}`,
  );
  const deadline = Date.now() + CLEANUP_TIMEOUT;
  while (alive(job) && Date.now() < deadline) await delay(20);
  if (alive(job))
    throw new Error(`skgo could not drain process group ${job.child.pid}`, {
      cause: job.signalError,
    });
}
// Node's generated stdin pipe is write-only at the parent. On Windows, pass
// a connected duplex pipe as stdin so Go receives its native inherited handle.
async function windowsControl() {
  const listener = createServer();
  const path = `\\\\.\\pipe\\skgo-prerender-${randomUUID()}`;
  let childEnd, parentEnd;
  const accepted = new Promise((resolve, reject) => {
    listener.once("connection", (socket) => {
      parentEnd = socket;
      resolve(socket);
    });
    listener.once("error", reject);
  });
  accepted.catch(() => {});
  try {
    await bounded(
      new Promise((resolve, reject) => {
        listener.once("error", reject);
        listener.listen(path, resolve);
      }),
      STARTUP_TIMEOUT,
      "skgo prerender control pipe startup timed out",
    );
    childEnd = createConnection(path);
    const connected = new Promise((resolve, reject) => {
      childEnd.once("connect", resolve);
      childEnd.once("error", reject);
    });
    await bounded(
      Promise.all([accepted, connected]),
      STARTUP_TIMEOUT,
      "skgo prerender control pipe startup timed out",
    );
    listener.close();
    return { childEnd, parentEnd };
  } catch (error) {
    childEnd?.destroy();
    parentEnd?.destroy();
    listener.close();
    throw error;
  }
}

function startJob(owner, command, args, options = {}) {
  const child = spawn(command, args, {
    cwd: owner.cwd,
    env: options.env ?? process.env,
    stdio: options.service
      ? options.control
        ? [options.control.childEnd, "pipe", "pipe"]
        : ["ignore", "pipe", "pipe", "pipe"]
      : ["ignore", "pipe", "pipe"],
    detached: process.platform !== "win32",
    windowsHide: true,
  });
  const job = {
    child,
    stderr: "",
    control: options.service ? (options.control?.parentEnd ?? child.stdio[3]) : null,
  };
  options.control?.childEnd.destroy();
  owner.jobs.add(job);
  job.exited = new Promise((resolve, reject) => {
    child.once("error", reject);
    child.once("exit", (code, signal) => {
      job.hasExited = true;
      resolve({ code, signal });
    });
  });
  job.closed = new Promise((resolve) => {
    child.once("close", resolve);
  });
  job.exited.catch(() => {});
  child.stdout.on("data", (chunk) => {
    if (options.service) process.stdout.write(chunk);
  });
  child.stderr.on("data", (chunk) => {
    if (options.service) process.stderr.write(chunk);
    else job.stderr += chunk;
  });
  job.control?.on("error", () => {});
  // Descendants can inherit stdout/stderr and hold close open after the leader exits.
  child.once("exit", () => {
    void signalTree(job, false).catch(reportOwnerFailure);
  });
  return job;
}

/** The main build thread owns its compiler, HTTP service and scratch directory. */
export function createPrerenderOwner(root = appRoot()) {
  if (!isMainThread) return null;
  root = resolve(root);
  if (owners.has(root)) return owners.get(root);
  const manifest = JSON.parse(readFileSync(join(root, "skgo.remotes.json"), "utf8"));
  // The generator emits this entry only when loads or prerender remotes exist.
  if (!manifest.prerender) return null;
  const owner = {
    root,
    cwd: resolve(root, manifest.prerender.root),
    pkg: manifest.prerender.package,
    jobs: new Set(),
    closing: false,
    failed: null,
    dir: null,
    service: null,
    previousEnv: null,
    signals: new Map(),
    cleanupPromise: null,
    preparePromise: null,
    fail(error) {
      owner.failed ??= error;
      return owner.cleanup().catch((cleanupError) => {
        owner.cleanupFailure = cleanupError;
        if (typeof owner.failed === "object" && owner.failed !== cleanupError) {
          if (!owner.failed.cleanupError) {
            const detail =
              cleanupError.errors?.map((error) => error.message).join("; ") ?? cleanupError.message;
            const diagnostic = `cleanup failed: ${detail}`;
            owner.failed.message += `; ${diagnostic}`;
            if (owner.failed.stack) owner.failed.stack += `\n${diagnostic}`;
          }
          owner.failed.cleanupError = cleanupError;
        }
        throw owner.failed;
      });
    },
    prepare() {
      return (owner.preparePromise ??= (async () => {
        owner.dir = mkdtempSync(join(tmpdir(), "skgo-prerender-"));
        mkdirSync(join(owner.dir, "go-tmp"));
        const binary = join(owner.dir, process.platform === "win32" ? "build.exe" : "build");
        try {
          const compiler = startJob(owner, "go", ["build", "-o", binary, owner.pkg], {
            env: { ...process.env, GOTMPDIR: join(owner.dir, "go-tmp") },
          });
          const result = await compiler.exited;
          await stopJob(compiler);
          owner.jobs.delete(compiler);
          if (result.code !== 0)
            throw new Error(`skgo prerender compiler failed: ${compiler.stderr.trim()}`);
          if (owner.closing) throw owner.failed ?? new Error("skgo prerender owner closed");
          const control = process.platform === "win32" ? await windowsControl() : null;
          if (owner.closing) {
            control?.childEnd.destroy();
            control?.parentEnd.destroy();
            throw owner.failed ?? new Error("skgo prerender owner closed");
          }
          let service;
          try {
            service = owner.service = startJob(owner, binary, [], { service: true, control });
          } catch (error) {
            control?.childEnd.destroy();
            control?.parentEnd.destroy();
            throw error;
          }
          const ready = new Promise((resolve, reject) => {
            let data = "";
            const receive = (chunk) => {
              data += chunk;
              if (!data.includes("\n")) return;
              service.control.off("data", receive);
              try {
                const value = JSON.parse(data.slice(0, data.indexOf("\n")));
                const url = new URL(value.url);
                if (
                  url.protocol !== "http:" ||
                  url.hostname !== "127.0.0.1" ||
                  !url.port ||
                  typeof value.secret !== "string" ||
                  !value.secret
                )
                  throw new Error("invalid readiness");
                resolve(value);
              } catch {
                reject(new Error("skgo prerender service returned invalid readiness"));
              }
            };
            service.control.setEncoding("utf8");
            service.control.on("data", receive);
            service.control.once("error", () =>
              reject(new Error("skgo prerender service control channel failed")),
            );
            service.exited.then(
              ({ code, signal }) =>
                reject(
                  new Error(`skgo prerender service exited before readiness: ${code ?? signal}`),
                ),
              reject,
            );
          });
          owner.connection = await bounded(
            ready,
            STARTUP_TIMEOUT,
            "skgo prerender service startup timed out",
          );
          process.stderr.write(`skgo prerender service started (pid ${service.child.pid})\n`);
          service.exited.then(
            ({ code, signal }) => {
              if (!owner.closing || code !== 0) {
                const error = new Error(
                  `skgo prerender service exited unexpectedly: ${code ?? signal}`,
                );
                owner.failed ??= error;
                if (!owner.closing) void owner.fail(error).catch(reportOwnerFailure);
              }
            },
            (error) => {
              if (!owner.closing) void owner.fail(error).catch(reportOwnerFailure);
            },
          );
        } catch (error) {
          await owner.fail(error);
          throw error;
        }
      })());
    },
    publish() {
      if (owner.closing) throw owner.failed ?? new Error("skgo prerender owner closed");
      if (owner.previousEnv) return;
      owner.previousEnv = new Map([
        [SERVICE_URL, process.env[SERVICE_URL]],
        [SERVICE_SECRET, process.env[SERVICE_SECRET]],
      ]);
      process.env[SERVICE_URL] = owner.connection.url;
      process.env[SERVICE_SECRET] = owner.connection.secret;
    },
    cleanup() {
      if (owner.cleanupPromise) return owner.cleanupPromise;
      owner.closing = true;
      owner.cleanupPromise = (async () => {
        const errors = [];
        for (const job of owner.jobs) {
          try {
            await stopJob(job, job === owner.service);
          } catch (error) {
            errors.push(error);
          } finally {
            job.control?.destroy();
          }
        }
        if (owner.previousEnv)
          for (const [key, value] of owner.previousEnv) {
            if (value === undefined) delete process.env[key];
            else process.env[key] = value;
          }
        for (const [signal, handler] of owner.signals) process.off(signal, handler);
        if (owner.parentObserver) clearInterval(owner.parentObserver);
        process.off("exit", owner.exitHandler);
        if (!errors.length && owner.dir) rmSync(owner.dir, { recursive: true, force: true });
        owners.delete(root);
        if (owner.service && !errors.length)
          process.stderr.write(`skgo prerender service stopped (pid ${owner.service.child.pid})\n`);
        if (errors.length)
          throw new AggregateError(errors, "skgo could not drain owned prerender processes");
      })();
      return owner.cleanupPromise;
    },
  };
  owner.exitHandler = () => {
    for (const job of owner.jobs) {
      if (process.platform !== "win32") {
        try {
          process.kill(-job.child.pid, "SIGKILL");
        } catch {}
      }
    }
  };
  process.once("exit", owner.exitHandler);
  const interrupted = async (error, signal) => {
    try {
      await owner.fail(error);
    } catch (error) {
      reportOwnerFailure(error);
    }
    process.exitCode = 1;
    try {
      process.kill(process.pid, signal);
    } catch {
      process.exit(1);
    }
  };
  for (const signal of ["SIGINT", "SIGTERM"]) {
    const handler = () => {
      void interrupted(new Error(`skgo prerender build interrupted by ${signal}`), signal);
    };
    owner.signals.set(signal, handler);
    process.on(signal, handler);
  }
  const parent = vitePlusBuildParent(root);
  if (parent !== null) {
    owner.parentObserver = setInterval(() => {
      if (process.ppid === parent) return;
      clearInterval(owner.parentObserver);
      void interrupted(new Error("skgo prerender Vite build parent exited"), "SIGTERM");
    }, 50);
    owner.parentObserver.unref();
  }
  owners.set(root, owner);
  return owner;
}

/** Vite 8's native buildApp has no failure-finally and its CLI exits immediately.
 * Wrap only those whole-app hooks so their rejection waits for the owner. */
export function installPrerenderFailureBoundary(config, owner) {
  if (!owner || typeof config.getSortedPlugins !== "function") return;
  const wrapped = (owner.wrapped ??= new WeakSet());
  const wrap = (handler) => {
    if (wrapped.has(handler)) return handler;
    const replacement = async function (...args) {
      try {
        const result = await handler.apply(this, args);
        if (owner.failed) throw owner.failed;
        return result;
      } catch (error) {
        await owner.fail(error);
        throw error;
      }
    };
    wrapped.add(replacement);
    return replacement;
  };
  for (const plugin of config.getSortedPlugins("buildApp")) {
    const hook = plugin.buildApp;
    if (typeof hook === "function") plugin.buildApp = wrap(hook);
    else if (hook?.handler) plugin.buildApp = { ...hook, handler: wrap(hook.handler) };
  }
  if (typeof config.builder?.buildApp === "function")
    config.builder.buildApp = wrap(config.builder.buildApp);
}

async function invokeService(request) {
  const url = process.env[SERVICE_URL];
  const secret = process.env[SERVICE_SECRET];
  const identity = `${request.kind} ${request.module}/${request.name ?? ""}`;
  if (!url || !secret) throw new Error(`skgo prerender service unavailable for ${identity}`);
  const { kind, ...body } = request;
  const path = { load: "/load", remote: "/remote", "remote-inputs": "/inputs" }[kind];
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), CALLBACK_TIMEOUT);
  try {
    const response = await fetch(url + path, {
      method: "POST",
      headers: { authorization: `Bearer ${secret}`, "content-type": "application/json" },
      body: JSON.stringify(body),
      signal: controller.signal,
    });
    const raw = await response.text();
    if (!response.ok) throw new Error(`HTTP ${response.status}: ${raw}`);
    validateEnvelope(request, raw);
    return raw;
  } catch (error) {
    if (controller.signal.aborted) {
      const message = `skgo prerender ${identity} timed out after ${CALLBACK_TIMEOUT}ms`;
      process.stderr.write(`${message}\n`);
      throw new Error(message);
    }
    throw new Error(`skgo prerender ${identity} failed: ${error.message}`);
  } finally {
    clearTimeout(timer);
  }
}

async function kitRuntime(root) {
  const kitRoot = kitRootFor(root);
  const transportURL = pathToFileURL(join(kitRoot, "src/runtime/app/internal/transport.js")).href;
  const internalURL = pathToFileURL(join(kitRoot, "src/exports/internal/shared.js")).href;
  const transport = await import(transportURL);
  const hooks = join(root, ".svelte-kit/output/server/entries/hooks.universal.js");
  let definitions = {};
  try {
    ({ transport: definitions = {} } = await import(pathToFileURL(hooks).href));
  } catch (error) {
    if (error?.code !== "ERR_MODULE_NOT_FOUND") throw error;
  }
  transport.init_transport(definitions);
  const internal = await import(internalURL);
  const require = createRequire(join(kitRoot, "package.json"));
  const devalue = await import(pathToFileURL(require.resolve("devalue")).href);
  return { transport, internal, devalue };
}

let runtimePromise;
function runtime() {
  if (!runtimePromise) runtimePromise = kitRuntime(appRoot());
  return runtimePromise;
}

/** Called by the generated native Kit input producer. */
export async function remoteInputs(module, name) {
  verifyKitQueue(appRoot());
  const raw = await invokeService({ kind: "remote-inputs", module, name });
  const answer = JSON.parse(raw);
  const { transport } = await runtime();
  const values = transport.parse(answer.inputs);
  if (!Array.isArray(values))
    throw new Error("skgo prerender inputs response did not decode to an array");
  return values;
}

/** The emulator callback for a Go load, executing entirely in its Kit worker. */
export async function remoteLoad(module, source, event) {
  const kit = await runtime();
  const { devalue, transport } = kit;
  const headers = Object.fromEntries(
    [...event.request.headers].map(([name, value]) => [name, [value]]),
  );
  const cookies = event.cookies.getAll();
  if (cookies.length) {
    headers.cookie = [
      cookies
        .map(({ name, value }) => `${encodeURIComponent(name)}=${encodeURIComponent(value)}`)
        .join("; "),
    ];
  }
  // Kit's JS matcher results cannot construct named Go values. Send its
  // actual route metadata and the unconverted path to the build-only Go load.
  const { manifest } = await import(
    pathToFileURL(join(appRoot(), ".svelte-kit/output/server/manifest-full.js")).href
  );
  const route = manifest.routes.find((candidate) => candidate.id === event.route.id);
  const prefix = manifest.app_path.slice(0, -manifest.app_dir.length);
  const base = prefix ? "/" + prefix.slice(0, -1) : "";
  const { decode_pathname } = await import(
    pathToFileURL(join(kitRootFor(appRoot()), "src/utils/url.js")).href
  );
  const request = {
    kind: "load",
    module,
    url: event.url.href,
    routeId: event.route.id,
    params: {},
    parent: await event.parent(),
    headers,
    ...(route
      ? {
          routePattern: route.pattern.source,
          routeParams: route.params,
          routePath: decode_pathname(event.url.pathname).slice(base.length) || "/",
        }
      : {}),
  };
  const raw = await invokeService(request);
  const answer = JSON.parse(raw);
  if (typeof answer.failure === "string") {
    throw new Error(
      `skgo: Go load failed during prerender: route ID ${event.route.id}, path ${event.url.pathname}, source ${source}: ${answer.failure}`,
    );
  }
  for (const [name, values] of Object.entries(answer.headers ?? {}))
    event.setHeaders({ [name]: values.join(", ") });
  for (const cookie of answer.cookies ?? []) {
    event.cookies.set(cookie.name, cookie.value, {
      path: cookie.path,
      ...(cookie.domain ? { domain: cookie.domain } : {}),
      ...(cookie.maxAge ? { maxAge: cookie.maxAge } : {}),
      httpOnly: cookie.httpOnly,
      secure: cookie.secure,
      sameSite: ["lax", "lax", "strict", "none"][cookie.sameSite] ?? "lax",
    });
  }
  if (answer.data) {
    const pending = new Map();
    const revivers = {
      ...transport.decoders,
      Promise: (id) => {
        let resolve;
        let reject;
        const promise = new Promise((yes, no) => {
          resolve = yes;
          reject = no;
        });
        pending.set(id, { promise, resolve, reject });
        return promise;
      },
    };
    answer.data = devalue.unflatten(answer.data, revivers);
    for (const chunk of answer.chunks ?? []) {
      const deferred = pending.get(chunk.id);
      if (!deferred) throw new Error(`skgo: unknown prerender deferred id ${chunk.id}`);
      if (chunk.error) deferred.reject(new Error(chunk.error));
      else deferred.resolve(devalue.unflatten(chunk.data, revivers));
    }
  }
  return answer;
}

/** The emulator callback for a Go prerender remote, executing in Kit's worker. */
export async function remoteFunction(module, name, arg, event) {
  const kit = await runtime();
  const { internal, transport } = kit;
  const payload =
    arg === undefined ? "" : Buffer.from(transport.stringify(arg), "utf8").toString("base64url");
  const headers = Object.fromEntries(
    [...event.request.headers].map(([key, value]) => [key, [value]]),
  );
  const raw = await invokeService({
    kind: "remote",
    module,
    name,
    payload,
    url: event.url.href,
    headers,
  });
  const answer = JSON.parse(raw);
  if (answer.type === "error" && answer.kind === "app") throw new internal.HttpError(answer.error);
  if (answer.type === "error" && answer.kind === "unknown") throw new Error(answer.diagnostic);
  if (answer.type === "redirect")
    throw new internal.Redirect(answer.redirect.status, answer.redirect.location);
  if (answer.type !== "result" || typeof answer.data !== "string")
    throw new Error("skgo prerender remote response is malformed");
  const result = transport.parse(answer.data);
  if (!result || typeof result !== "object" || !Object.hasOwn(result, "_")) {
    throw new Error("skgo prerender remote result omitted its value member");
  }
  return result._;
}
