// Loaded through Vite together with Kit's live transport module. The imports
// are supplied by dev-modules, so transport classes share the app's identity.
import { request as httpRequest } from "node:http";
import {
  Worker,
  MessageChannel,
  receiveMessageOnPort,
} from "node:worker_threads";
import { error, redirect, fail } from "@sveltejs/kit";

let predicates;
let predicateID = 0;
function predicate(handle, option, args) {
  if (!predicates) {
    const { port1, port2 } = new MessageChannel();
    const worker = new Worker(new URL(predicateWorkerURL), {
      workerData: { port: port2 },
      execArgv: [],
      transferList: [port2],
    });
    worker.unref();
    port1.unref();
    predicates = { worker, port: port1 };
  }
  const id = ++predicateID,
    cell = new Int32Array(new SharedArrayBuffer(4));
  predicates.port.postMessage({
    id,
    cell,
    payload: { handle, option, ...args },
  });
  if (Atomics.wait(cell, 0, 0, 31_000) === "timed-out")
    throw new Error("skgo: Go predicate relay timed out");
  const answer = receiveMessageOnPort(predicates.port)?.message;
  if (answer?.id !== id)
    throw new Error("skgo: Go predicate relay lost its reply");
  if (answer.error) throw new Error(answer.error);
  return answer.value;
}
function headers(event) {
  const result = Object.fromEntries(
    [...event.request.headers].map(([k, v]) => [k, [v]]),
  );
  const cookies = event.cookies.getAll();
  if (cookies.length)
    result.cookie = [
      cookies
        .map(
          ({ name, value }) =>
            `${encodeURIComponent(name)}=${encodeURIComponent(value)}`,
        )
        .join("; "),
    ];
  return result;
}
function call(kind, payload, event, raw) {
  return new Promise((resolve, reject) => {
    const request = httpRequest(
      {
        socketPath: process.env.SKGO_DEV_GO_SOCKET,
        path: "/_skgo/dev/" + kind,
        method: "POST",
        headers: {
          authorization: `Bearer ${process.env.SKGO_DEV_SERVICE_SECRET}`,
          ...(raw ? raw.headers : { "content-type": "application/json" }),
        },
      },
      (response) => {
        if (
          (response.statusCode < 200 || response.statusCode >= 300) &&
          !["response", "endpoint"].includes(kind) &&
          response.headers["x-skgo-fetch-state"] !== "final"
        ) {
          response.resume();
          reject(
            new Error(`skgo: development ${kind} HTTP ${response.statusCode}`),
          );
          return;
        }
        resolve(response);
      },
    );
    request.on("error", reject);
    const signal = event?.request.signal;
    const abort = () => request.destroy(new Error("skgo: request aborted"));
    signal?.addEventListener("abort", abort, { once: true });
    request.on("close", () => signal?.removeEventListener("abort", abort));
    if (signal?.aborted) {
      abort();
      return;
    }
    if (raw?.body) {
      (async () => {
        try {
          for await (const chunk of raw.body) {
            if (!request.write(chunk))
              await new Promise((resolve) => request.once("drain", resolve));
          }
          request.end();
        } catch (e) {
          request.destroy(e);
        }
      })();
    } else request.end(raw ? undefined : JSON.stringify(payload));
  });
}
async function json(kind, payload, event) {
  const response = await call(kind, payload, event);
  let text = "";
  for await (const chunk of response) text += chunk;
  return JSON.parse(text);
}
function effects(event, answer) {
  for (const [name, values] of Object.entries(answer.headers ?? {}))
    event.setHeaders({ [name]: values.join(", ") });
  for (const cookie of answer.cookies ?? [])
    event.cookies.set(cookie.name, cookie.value, {
      path: cookie.path,
      ...(cookie.domain ? { domain: cookie.domain } : {}),
      ...(cookie.maxAge !== undefined ? { maxAge: cookie.maxAge } : {}),
      ...(cookie.expires ? { expires: new Date(cookie.expires) } : {}),
      httpOnly: cookie.httpOnly,
      secure: cookie.secure,
      sameSite:
        [false, false, "lax", "strict", "none"][cookie.sameSite] ?? false,
      ...(cookie.partitioned ? { partitioned: true } : {}),
      ...(cookie.priority ? { priority: cookie.priority } : {}),
      ...(cookie.raw ? { encode: (value) => value } : {}),
    });
}
function exceptional(answer) {
  if (answer.redirect)
    redirect(answer.redirect.status, answer.redirect.location);
  if (answer.error) error(answer.error.status, answer.body ?? answer.error.message);
  if (answer.failure && typeof answer.failure === "string")
    throw new Error(answer.failure);
}
const owner = (event) => event.platform?.skgoRequestHandle;
export async function load(module, event) {
  const metadata = event.untrack(() => ({
    url: event.url.href, routeId: event.route.id, params: {...event.params}, parent: event.parent()
  }));
  const response = await call("load", {
    handle: owner(event), module, ...metadata, parent: await metadata.parent, headers: headers(event)
  }, event);
  const iterator = response[Symbol.asyncIterator]();
  let buffered = "";
  async function line() {
    for (;;) {
      const end = buffered.indexOf("\n");
      if (end >= 0) {
        const text = buffered.slice(0, end);
        buffered = buffered.slice(end + 1);
        return JSON.parse(text);
      }
      const next = await iterator.next();
      if (next.done) {
        if (buffered) {
          const text = buffered;
          buffered = "";
          return JSON.parse(text);
        }
        return null;
      }
      buffered += next.value.toString();
    }
  }
  const answer = await line();
  effects(event, answer);
  exceptional(answer);
  for (const dependency of answer.uses?.dependencies ?? []) event.depends(dependency);
  for (const name of answer.uses?.params ?? []) void event.params[name];
  for (const name of answer.uses?.search_params ?? []) event.url.searchParams.get(name);
  if (answer.uses?.parent) await event.parent();
  if (answer.uses?.route) void event.route.id;
  if (answer.uses?.url) void event.url.href;
  const pending = new Map();
  const revivers = {
    ...transport.decoders,
    Promise: (id) =>
      new Promise((resolve, reject) => pending.set(id, { resolve, reject })),
  };
  const data = devalue.unflatten(answer.data, revivers);
  const accept = (chunk) => {
    const item = pending.get(chunk.id);
    if (!item) throw new Error(`skgo: unknown deferred ${chunk.id}`);
    pending.delete(chunk.id);
    chunk.error
      ? item.reject(new Error(chunk.error))
      : item.resolve(devalue.unflatten(chunk.data, revivers));
  };
  for (const chunk of answer.chunks ?? []) accept(chunk);
  void (async () => {
    try {
      let chunk;
      while ((chunk = await line()) !== null) accept(chunk);
      if (pending.size) throw new Error("skgo: incomplete deferred load");
    } catch (e) {
      for (const item of pending.values()) item.reject(e);
    }
  })();
  return data;
}
export async function remote(module, name, arg, event) {
  const answer = await json(
    "remote",
    {
      handle: owner(event),
      module,
      name,
      url: event.url.href,
      headers: headers(event),
      payload:
        arg === undefined
          ? ""
          : Buffer.from(transport.stringify(arg)).toString("base64url"),
    },
    event,
  );
  if (answer.type === "redirect")
    redirect(answer.redirect.status, answer.redirect.location);
  if (answer.type === "error") {
    if (answer.kind === "app") error(answer.error.status, answer.error.message);
    throw new Error(answer.diagnostic ?? answer.error?.message);
  }
  if (answer.type !== "result")
    throw new Error("skgo: malformed remote result");
  const value = transport.parse(answer.data);
  if (!Object.hasOwn(value, "_"))
    throw new Error("skgo: missing remote result");
  return value._;
}
export async function action(module, name, event) {
  const answer = await json(
    "action",
    { handle: owner(event), module, name },
    event,
  );
  effects(event, answer);
  if (answer.type === "redirect")
    redirect(answer.status, answer.location ?? answer.redirect?.location);
  if (answer.type === "error")
    error(answer.status, answer.body ?? answer.error?.message ?? answer.message);
  exceptional(answer);
  const data =
    answer.data === undefined ? undefined : transport.parse(answer.data);
  return answer.type === "failure" ? fail(answer.status, data) : data;
}
function responseFrom(value, method) {
  const headers = new Headers();
  for (const [name, values] of Object.entries(value.headers ?? {}))
    for (const val of values) headers.append(name, val);
  return new Response(
    method === "HEAD" || [204, 205, 304].includes(value.status)
      ? null
      : Buffer.from(value.body ?? "", "base64"),
    { status: value.status, headers },
  );
}
function incomingResponse(upstream, method, finish = async () => {}) {
  const headers = new Headers();
  for (let i = 0; i < upstream.rawHeaders.length; i += 2) {
    if (!upstream.rawHeaders[i].toLowerCase().startsWith("x-skgo-"))
      headers.append(upstream.rawHeaders[i], upstream.rawHeaders[i + 1]);
  }
  if (method === "HEAD" || [204, 205, 304].includes(upstream.statusCode)) {
    upstream.resume();
    void finish();
    return new Response(null, { status: upstream.statusCode, headers });
  }
  const iterator = upstream[Symbol.asyncIterator]();
  const body = new ReadableStream(
    {
      async pull(controller) {
        try {
          const next = await iterator.next();
          if (next.done) {
            await finish();
            controller.close();
          } else controller.enqueue(next.value);
        } catch (error) {
          await finish();
          controller.error(error);
        }
      },
      async cancel() {
        upstream.destroy();
        await finish();
      },
    },
    { highWaterMark: 0 },
  );
  return new Response(body, { status: upstream.statusCode, headers });
}
export async function endpoint(event) {
  const response = await call("endpoint", { handle: owner(event) }, event);
  const encoded = response.headers["x-skgo-effects"];
  if (encoded)
    effects(event, JSON.parse(Buffer.from(encoded, "base64").toString()));
  return incomingResponse(response, event.request.method);
}
export async function requestHandle(event, resolve) {
  const address = event.request.headers.get("x-skgo-client-address");
  if (address) event.getClientAddress = () => address;
  const answer = await json(
    "begin",
    {
      method: event.request.method,
      url: event.request.url,
      logicalUrl: event.url.href,
      headers: headers(event),
      body: ["GET", "HEAD"].includes(event.request.method)
        ? ""
        : Buffer.from(await event.request.clone().arrayBuffer()).toString(
            "base64",
          ),
      routeId: event.route.id ?? "",
      params: event.params,
      isSubRequest: event.isSubRequest,
      clientAddress: address ?? "",
    },
    event,
  );
  const handle = answer.handle;
  if (!handle) throw new Error("skgo: begin omitted request owner");
  let ended = false;
  const end = async () => {
    if (!ended) {
      ended = true;
      await json("end", { handle });
    }
  };
  try {
    effects(event, answer);
    exceptional(answer);
    if (!answer.resolve) {
      const response = responseFrom(answer.response, event.request.method);
      await end();
      return response;
    }
    event.platform ??= {};
    event.platform.skgoRequestHandle = handle;
    const options = {};
    options.transformPageChunk = async ({ html, done }) => {
      if (answer.options?.transform) {
        html = (
          await json(
            "resolve-option",
            { handle, option: "transform", html, done },
            event,
          )
        ).html;
      }
      return html.replace(
        "</head>",
        `<script type="module" src="${devBase}/__skgo/dev/client.js"></script></head>`,
      );
    };
    if (answer.options?.filter)
      options.filterSerializedResponseHeaders = (name, value) =>
        predicate(handle, "filter", { name, value });
    if (answer.options?.preload)
      options.preload = (input) => predicate(handle, "preload", { input });
    const response = await resolve(event, options);
    const responseHeaders = Object.fromEntries(
      [...response.headers].map(([k, v]) => [k, [v]]),
    );
    responseHeaders["set-cookie"] = response.headers.getSetCookie();
    const upstream = await call("response", null, event, {
      headers: {
        "content-type": "application/octet-stream",
        "x-skgo-handle": handle,
        "x-skgo-response": Buffer.from(
          JSON.stringify({ status: response.status, headers: responseHeaders }),
        ).toString("base64"),
      },
      body: response.body,
    });
    const outHeaders = new Headers();
    for (let i = 0; i < upstream.rawHeaders.length; i += 2)
      outHeaders.append(upstream.rawHeaders[i], upstream.rawHeaders[i + 1]);
    const iterator = upstream[Symbol.asyncIterator]();
    const body = new ReadableStream(
      {
        async pull(controller) {
          try {
            const next = await iterator.next();
            if (next.done) {
              await end();
              controller.close();
            } else controller.enqueue(next.value);
          } catch (e) {
            await end();
            controller.error(e);
          }
        },
        async cancel() {
          upstream.destroy();
          await end();
        },
      },
      { highWaterMark: 0 },
    );
    if (
      event.request.method === "HEAD" ||
      [204, 205, 304].includes(upstream.statusCode)
    ) {
      upstream.resume();
      await end();
      return new Response(null, {
        status: upstream.statusCode,
        headers: outHeaders,
      });
    }
    return new Response(body, {
      status: upstream.statusCode,
      headers: outHeaders,
    });
  } catch (e) {
    await end();
    throw e;
  }
}
export async function requestFetch({ event, request, fetch }) {
  // The Go hook surrounds Kit's supplied continuation rather than replacing
  // Kit's credential inheritance, cookie handling or live page dispatcher.
  let id;
  const finish = async () => {
    if (!id) return;
    const current = id;
    id = undefined;
    const response = await call("fetch-end", null, undefined, {
      headers: { "x-skgo-fetch": current },
    });
    response.resume();
  };
  try {
    let response = await call(
      "fetch-begin",
      {
        handle: owner(event),
        url: request.url,
        method: request.method,
        headers: Object.fromEntries(
          [...request.headers].map(([key, value]) => [key, [value]]),
        ),
        body: ["GET", "HEAD"].includes(request.method)
          ? ""
          : Buffer.from(await request.clone().arrayBuffer()).toString("base64"),
      },
      event,
    );
    id = response.headers["x-skgo-fetch"];
    while (response.headers["x-skgo-fetch-state"] === "next") {
      const input = JSON.parse(
        Buffer.from(
          response.headers["x-skgo-fetch-request"],
          "base64",
        ).toString(),
      );
      response.resume();
      const headers = new Headers();
      for (const [key, values] of Object.entries(input.headers ?? {}))
        for (const value of values) headers.append(key, value);
      const nextRequest = new Request(input.url, {
        method: input.method,
        headers,
        credentials: request.credentials,
        mode: request.mode,
        redirect: request.redirect,
        signal: request.signal,
        ...(!["GET", "HEAD"].includes(input.method)
          ? { body: Buffer.from(input.body ?? "", "base64") }
          : {}),
      });
      const result = await fetch(nextRequest);
      const resultHeaders = Object.fromEntries(
        [...result.headers].map(([key, value]) => [key, [value]]),
      );
      resultHeaders["set-cookie"] = result.headers.getSetCookie();
      response = await call("fetch-response", null, event, {
        headers: {
          "content-type": "application/octet-stream",
          "x-skgo-fetch": id,
          "x-skgo-response": Buffer.from(
            JSON.stringify({ status: result.status, headers: resultHeaders }),
          ).toString("base64"),
        },
        body: result.body,
      });
    }
    return incomingResponse(response, request.method, finish);
  } catch (error) {
    await finish();
    throw error;
  }
}
