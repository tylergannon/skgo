/** Internal bridge used by generated declared-prerender input modules. */
export function remoteInputs<Input = unknown>(module: string, name: string): Promise<Input[]>;

/** Build-only logical request lifecycle used by generated server hooks. */
export function requestHandle(event: import('@sveltejs/kit').RequestEvent, resolve: (event: import('@sveltejs/kit').RequestEvent, options?: import('@sveltejs/kit/hooks').ResolveOptions) => Promise<Response>): Promise<Response>;
export function requestFetch(input: Parameters<import('@sveltejs/kit/hooks').HandleFetch>[0]): Promise<Response>;
