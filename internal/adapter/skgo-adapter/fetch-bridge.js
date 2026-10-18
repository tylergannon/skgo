// Kit owns the fetch continuation in development and during prerendering.
// This shared transport lets the Go hook surround it without replacing it.
// HTTP framing belongs to each socket hop. Node already decoded chunked
// framing; carrying it into a Web Response can conflict with Kit's own length.
const hopHeaders = new Set([
  "connection",
  "keep-alive",
  "proxy-authenticate",
  "proxy-authorization",
  "te",
  "trailer",
  "transfer-encoding",
  "upgrade",
]);
const bridgeHeaders = new Set([
  "x-skgo-fetch",
  "x-skgo-fetch-state",
  "x-skgo-fetch-metadata",
  "x-skgo-effects",
  "x-skgo-tracing",
  "x-skgo-tracing-after",
  "x-skgo-native-error",
]);
function incomingHeaders(upstream) {
  const headers = new Headers();
  const connectionHeaders = new Set(
    String(upstream.headers.connection ?? "")
      .split(",")
      .map((value) => value.trim().toLowerCase()),
  );
  for (let i = 0; i < upstream.rawHeaders.length; i += 2) {
    const name = upstream.rawHeaders[i].toLowerCase();
    if (
      !hopHeaders.has(name) &&
      !connectionHeaders.has(name) &&
      !bridgeHeaders.has(name)
    )
      headers.append(upstream.rawHeaders[i], upstream.rawHeaders[i + 1]);
  }
  return headers;
}
export async function incomingResponse(upstream, method, finish = async () => {}) {
  const headers = incomingHeaders(upstream);
  if (method === "HEAD" || [204, 205, 304].includes(upstream.statusCode)) {
    upstream.resume();
    await finish();
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

export async function bridgeFetch({ event, request, fetch }, call, handle, ownerSignal) {
  const continuation = new AbortController();
  const signal = AbortSignal.any([
    request.signal,
    continuation.signal,
    ...(ownerSignal ? [ownerSignal] : []),
  ]);
  let id;
  const finish = async () => {
    const current = id;
    id = undefined;
    try {
      if (current && !ownerSignal?.aborted) {
        const response = await call("fetch-end", null, undefined, {
          headers: { "x-skgo-fetch": current },
        });
        response.resume();
      }
    } finally {
      continuation.abort();
    }
  };
  try {
    let response = await call(
      "fetch-begin",
      {
        handle,
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
      undefined,
      signal,
    );
    id = response.headers["x-skgo-fetch"];
    while (response.headers["x-skgo-fetch-state"] === "next") {
      let payload = "";
      for await (const chunk of response) payload += chunk;
      const input = JSON.parse(payload);
      const headers = new Headers();
      for (const [key, values] of Object.entries(input.headers ?? {}))
        for (const value of values) headers.append(key, value);
      const nextRequest = new Request(input.url, {
        method: input.method,
        headers,
        credentials: request.credentials,
        mode: request.mode,
        redirect: request.redirect,
        signal,
        ...(!["GET", "HEAD"].includes(input.method)
          ? { body: Buffer.from(input.body ?? "", "base64") }
          : {}),
      });
      const result = await fetch(nextRequest);
      const resultHeaders = Object.fromEntries(
        [...result.headers].map(([key, value]) => [key, [value]]),
      );
      resultHeaders["set-cookie"] = result.headers.getSetCookie();
      // Upload ownership lasts until Go releases this body. Negotiate the
      // next step separately so another continuation can run in the meantime.
      void call(
        "fetch-response",
        null,
        event,
        {
          headers: {
            "content-type": "application/octet-stream",
            "x-skgo-fetch": id,
            "x-skgo-response": Buffer.from(
              JSON.stringify({
                status: result.status,
                headers: resultHeaders,
                url: result.url,
                redirected: result.redirected,
                type: result.type,
              }),
            ).toString("base64"),
          },
          body: result.body,
        },
        signal,
      )
        .then((acknowledgment) => acknowledgment.resume())
        .catch((error) => {
          if (!signal.aborted) continuation.abort(error);
        });
      response = await call(
        "fetch-next",
        null,
        event,
        { headers: { "x-skgo-fetch": id } },
        signal,
      );
    }
    const encoded = response.headers["x-skgo-fetch-metadata"];
    const metadata = encoded
      ? JSON.parse(Buffer.from(encoded, "base64").toString())
      : undefined;
    return fetchResponseMetadata(
      await incomingResponse(response, request.method, finish),
      metadata,
    );
  } catch (error) {
    await finish();
    throw error;
  }
}

// These Fetch properties have no Response constructor options. Define the
// relayed values on the response and its clones, leaving the native body and
// header implementation (including brand checks) intact.
function fetchResponseMetadata(response, metadata) {
  if (!metadata) return response;
  for (const name of ["url", "redirected", "type"])
    Object.defineProperty(response, name, {
      value: metadata[name],
      configurable: true,
    });
  const clone = response.clone.bind(response);
  response.clone = () => fetchResponseMetadata(clone(), metadata);
  return response;
}
