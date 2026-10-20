import type { PageLoad } from "./$types";
import emitted from "./emitted.json?url";
import inlined from "./inlined.json?url";

// A universal load that fetches assets it imports, as a page reading prebaked
// data does. Vite emits emitted.json (4148 bytes) under the app's immutable
// assets and inlines inlined.json (32 bytes) as a data: URL, because it is
// under the default 4 KB assetsInlineLimit. During the document render both
// fetches go through Kit's universal fetch.

async function read(fetch: typeof globalThis.fetch, url: string) {
  const response = await fetch(url);
  const body = await response.text();
  return {
    url,
    status: response.status,
    bytes: body.length,
    lamp: (JSON.parse(body) as { lamp: string }).lamp,
  };
}

export const load: PageLoad = async ({ fetch }) => ({
  emitted: await read(fetch, emitted),
  inlined: await read(fetch, inlined),
});
