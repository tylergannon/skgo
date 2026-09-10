<script lang="ts">
	import { routes } from '$app/manifest';

	// This page talks to /api/todos with `fetch`, the way anything outside kit
	// would. Nothing here is a remote function: the route is raw HTTP, and what
	// arrives is whatever the Go handler in src/routes/api/todos/server.go
	// wrote — status, headers and body.
	type Todo = { id: string; text: string; private: boolean };

	let todos = $state<Todo[] | null>(null);
	let draft = $state('');
	// The record the endpoint said it created, out of the 201's own body. The
	// list below is a separate GET, so naming the id here is what lets a
	// caller say "the thing I created is in the list" rather than "something
	// with that text is".
	let created = $state<Todo | null>(null);
	let matches = $state<Todo[] | null>(null);
	let last = $state<{ method: string; status: number; type: string; allow: string } | null>(null);
	const apiRoutes = routes.filter((route) => route.id.startsWith('/api'));

	function record(method: string, response: Response) {
		last = {
			method,
			status: response.status,
			type: response.headers.get('content-type') ?? '',
			allow: response.headers.get('allow') ?? ''
		};
	}

	async function load(remember: boolean) {
		const response = await fetch('/api/todos');
		if (remember) record('GET', response);
		if (response.ok) todos = await response.json();
	}

	async function add(event: SubmitEvent) {
		event.preventDefault();
		const response = await fetch('/api/todos', {
			method: 'POST',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ text: draft })
		});
		record('POST', response);
		if (response.status === 201) {
			created = await response.json();
			draft = '';
			await load(false);
		}
	}

	// The route declares GET and POST and nothing else, so this is what a
	// caller sees when it asks for a method the route does not answer.
	async function tryDelete() {
		record('DELETE', await fetch('/api/todos', { method: 'DELETE' }));
	}

	async function query() {
		const response = await fetch('/api/todos', {
			method: 'QUERY',
			headers: { 'content-type': 'application/json' },
			body: JSON.stringify({ contains: 'adapter' })
		});
		record('QUERY', response);
		if (response.ok) matches = await response.json();
	}

	$effect(() => {
		void load(true);
	});
</script>

<h1 data-testid="title">API</h1>

<p>
	Everything below came from <code>/api/todos</code>, an ordinary Go HTTP handler written in
	<code>src/routes/api/todos/server.go</code> beside the route it serves.
</p>

<h2>SvelteKit 3 route manifest</h2>
<ul data-testid="api-manifest">
	{#each apiRoutes as route (route.id)}
		<li data-testid="api-manifest-route">
			<code>{route.id}</code> — page: {route.page}, endpoint: {route.endpoint}
		</li>
	{/each}
</ul>

{#if last}
	<p data-testid="api-response">
		{last.method} → {last.status}
		{#if last.type}· {last.type}{/if}
		{#if last.allow}· Allow: {last.allow}{/if}
	</p>
{:else}
	<p data-testid="api-pending">calling the endpoint…</p>
{/if}

{#if created}
	<p data-testid="api-created">created {created.id}</p>
{/if}

{#if todos}
	<ul data-testid="api-todos">
		{#each todos as todo (todo.id)}
			<li data-testid="api-todo" data-id={todo.id}>{todo.text}</li>
		{/each}
	</ul>
{/if}

<form onsubmit={add}>
	<label>
		New todo
		<input data-testid="api-draft" bind:value={draft} />
	</label>
	<button data-testid="api-post" type="submit">POST it</button>
</form>

<button data-testid="api-delete" type="button" onclick={tryDelete}>
	DELETE it (a method this route does not declare)
</button>

<button data-testid="api-query" type="button" onclick={query}>QUERY for “adapter”</button>

{#if matches}
	<ul data-testid="api-query-results">
		{#each matches as todo (todo.id)}
			<li data-testid="api-query-result">{todo.text}</li>
		{/each}
	</ul>
{/if}
