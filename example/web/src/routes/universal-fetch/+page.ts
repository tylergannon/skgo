import type { PageLoad } from "./$types";

// A universal load that reads every shape of response Kit knows how to replay:
// text, JSON, bytes that are not UTF-8, a body read as a stream, no body, a
// rendered page of the app, requests told apart only by a header or a body, a
// body that cannot be replayed, and a response the browser may keep. During
// the document render each one goes through Kit's own universal fetch; in the
// browser the same calls find what the document carried, or go to the network.
//
// `?probe=` runs one fetch in isolation, for the Go tests that read the
// document: an external URL under every CORS mode, a header the app's filter
// withholds, a request given as a Request object.

const hex = (bytes: Uint8Array) =>
  Array.from(bytes, (b) => b.toString(16).padStart(2, "0")).join("");

type Fetch = typeof fetch;

async function stream(response: Response) {
  const reader = response.body!.getReader();
  const decoder = new TextDecoder();
  let text = "";
  for (;;) {
    const { value, done } = await reader.read();
    if (done) return text;
    text += decoder.decode(value);
  }
}

async function external(fetch: Fetch, url: URL) {
  const query = url.searchParams;
  const target = query.get("target") ?? "";
  const out = { status: 0, body: "", header: "", cookies: "", error: "" };
  if (!/^http:\/\/127\.0\.0\.1:\d+$/.test(target)) {
    out.error = "target must be a loopback http origin";
    return out;
  }
  try {
    const response = await fetch(`${target}${query.get("path") ?? "/"}`, {
      mode: (query.get("mode") ?? "cors") as RequestMode,
    });
    out.status = response.status;
    const header = query.get("header");
    if (header) {
      try {
        out.header = String(response.headers.get(header));
      } catch (e) {
        out.header = "denied";
      }
    }
    if (query.get("cookies")) {
      try {
        out.cookies = response.headers.getSetCookie().join(" | ");
      } catch (e) {
        out.cookies = "denied";
      }
    }
    switch (query.get("read") ?? "text") {
      case "text":
        out.body = (await response.text()) ?? "";
        break;
      case "json":
        out.body = JSON.stringify(await response.json());
        break;
      case "bytes":
        out.body = hex(new Uint8Array(await response.arrayBuffer()));
        break;
      case "stream":
        out.body = await stream(response);
        break;
    }
  } catch (e) {
    out.error = (e as Error).message;
  }
  return out;
}

async function request(fetch: Fetch, url: URL, q: string) {
  const kind = url.searchParams.get("kind");
  const echo = `/api/replay/echo?${q}`;
  let response: Response;
  if (kind === "object") {
    response = await fetch(
      new Request(echo, { method: "POST", headers: { "x-replay": "r" }, body: "object-body" }),
    );
  } else if (kind === "bytes") {
    response = await fetch(echo, { method: "POST", body: new TextEncoder().encode("bytes-body") });
  } else {
    response = await fetch(echo, { method: "POST", headers: { "x-replay": "h" }, body: "both" });
  }
  return await response.json();
}

async function secret(fetch: Fetch, q: string) {
  const response = await fetch(`/api/replay/binary?${q}`);
  try {
    return `read:${response.headers.get("x-replay-secret")}`;
  } catch (e) {
    return "denied";
  }
}

async function cookies(fetch: Fetch, q: string) {
  const first = await fetch(`/api/replay/cookies?${q}&set=1`);
  const second = await fetch(`/api/replay/cookies?${q}`);
  return { first: await first.text(), second: await second.text() };
}

export const load: PageLoad = async ({ fetch, url }) => {
  const run = url.searchParams.get("run") ?? "none";
  const step = url.searchParams.get("step") ?? "1";
  const q = `run=${encodeURIComponent(run)}&step=${encodeURIComponent(step)}`;
  const probe = url.searchParams.get("probe");
  if (probe === "external") return { run, step, main: null, probe: await external(fetch, url) };
  if (probe === "request") return { run, step, main: null, probe: await request(fetch, url, q) };
  if (probe === "cookies") return { run, step, main: null, probe: await cookies(fetch, q) };
  if (probe === "secret") return { run, step, main: null, probe: await secret(fetch, q) };

  const text = await (await fetch(`/api/replay/text?${q}`)).text();
  const json = await (await fetch(`/api/replay/json?${q}`)).json();
  const aliased = await (
    await fetch(`/api/replay/json?run=${encodeURIComponent(run)}&alias=1`)
  ).json();
  const binaryResponse = await fetch(`/api/replay/binary?${q}`);
  const binary = hex(new Uint8Array(await binaryResponse.arrayBuffer()));
  const shown = binaryResponse.headers.get("x-replay-allowed");
  const streamed = await stream(await fetch(`/api/replay/stream?${q}`));
  const breaking = await (await fetch(`/api/replay/breaking?${q}`)).text();
  const emptyResponse = await fetch(`/api/replay/empty?${q}`);
  const empty = `${emptyResponse.status}:${(await emptyResponse.text()) ? "body" : "none"}`;
  const missingResponse = await fetch(`/api/replay/missing?${q}`);
  const missing = `${missingResponse.status}:${missingResponse.ok}:${await missingResponse.text()}`;
  const echo = `/api/replay/echo?${q}`;
  const headerA = await (await fetch(echo, { headers: { "x-replay": "a" } })).json();
  const headerB = await (await fetch(echo, { headers: { "x-replay": "b" } })).json();
  const bodyAlpha = await (await fetch(echo, { method: "POST", body: "alpha" })).json();
  const bodyBeta = await (await fetch(echo, { method: "POST", body: "beta" })).json();
  const form = await (
    await fetch(`/api/replay/form?${q}`, {
      method: "POST",
      body: new URLSearchParams({ lamp: "one" }),
    })
  ).json();
  const cached = await (await fetch(`/api/replay/cached?run=${encodeURIComponent(run)}`)).json();
  const page = await (await fetch(`/request-fetch?run=${encodeURIComponent(run)}`)).text();
  const nested = /data-testid="request-fetch-fact">([^<]+)</.exec(page)?.[1] ?? "not found";

  return {
    run,
    step,
    probe: null,
    main: {
      text,
      json: `${json.lamp}@${json.step}`,
      aliased: `${aliased.lamp}@${aliased.step}`,
      binary,
      shown,
      streamed,
      breaking,
      empty,
      missing,
      headerA: `${headerA.header}@${headerA.step}`,
      headerB: `${headerB.header}@${headerB.step}`,
      bodyAlpha: `${bodyAlpha.method}:${bodyAlpha.body}@${bodyAlpha.step}`,
      bodyBeta: `${bodyBeta.method}:${bodyBeta.body}@${bodyBeta.step}`,
      form: `${form.method}:${form.body}:${form.contentType}`,
      cached: cached.lamp,
      nested,
    },
  };
};
