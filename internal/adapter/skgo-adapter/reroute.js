// Kit owns the hook's invocation and browser cache. This is transport only;
// route-selection logic and all of its I/O execute in the Go application.
export async function reroute({ url, fetch }) {
 const endpoint = `${__SVELTEKIT_PATHS_BASE__}/${__SVELTEKIT_APP_DIR__}/reroute`;
 if (url.pathname === endpoint) return;
 const response=await fetch(`${endpoint}?url=${encodeURIComponent(url.href)}`);
 if(!response.ok) throw new Error(`skgo: reroute failed (${response.status})`);
 return (await response.json()).path ?? undefined;
}
