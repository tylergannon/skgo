// Only synchronous Kit ResolveOptions predicates cross this worker. Its event
// loop remains available while Kit's main SSR thread waits for the Go reply.
import { workerData } from "node:worker_threads";
import { request } from "node:http";
const port = workerData.port;
port.on("message", ({ id, cell, payload }) => {
  const done = (reply) => {
    port.postMessage({ id, ...reply });
    Atomics.store(cell, 0, 1);
    Atomics.notify(cell, 0);
  };
  const call = request(
    {
      socketPath: process.env.SKGO_DEV_GO_SOCKET,
      path: "/_skgo/dev/resolve-option",
      method: "POST",
      headers: {
        authorization: `Bearer ${process.env.SKGO_DEV_SERVICE_SECRET}`,
        "content-type": "application/json",
      },
    },
    (response) => {
      let body = "";
      response.setEncoding("utf8");
      response.on("data", (chunk) => (body += chunk));
      response.on("end", () => {
        try {
          if (response.statusCode !== 200)
            throw new Error(`HTTP ${response.statusCode}`);
          const answer = JSON.parse(body);
          if (typeof answer.value !== "boolean")
            throw new Error("missing predicate boolean");
          done({ value: answer.value });
        } catch (error) {
          done({ error: error.message });
        }
      });
    },
  );
  call.on("error", (error) => done({ error: error.message }));
  call.setTimeout(30_000, () =>
    call.destroy(new Error("Go predicate timed out")),
  );
  call.end(JSON.stringify(payload));
});
