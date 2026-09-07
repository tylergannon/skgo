<script lang="ts">
	import { getSite } from '../site.remote';
	import { getTodos } from '../todos/todos.remote';
</script>

<!--
	SPIKE ROUTE. Every other page in this app puts its awaits inside a
	`<svelte:boundary>` that has a `pending` snippet, and Svelte's server
	compiler emits *only* that snippet — the children are dropped from the SSR
	output entirely (svelte/src/compiler/phases/3-transform/server/visitors/
	SvelteBoundary.js:59-74). That is correct for an app built to run with
	`ssr = false`, but it means none of those pages can demonstrate a remote
	function being awaited on the server. This one has a `failed` snippet and no
	`pending` one, so its children do render on the server.
-->
<h1 data-testid="title">SSR probe</h1>

<svelte:boundary>
	{@const site = await getSite()}
	<p data-testid="site-name">{site.name}</p>
	<p data-testid="colocated">{site.colocated}</p>
	<ul data-testid="todos">
		{#each await getTodos() as todo (todo.id)}
			<li data-testid="todo" class:private={todo.private}>{todo.text}</li>
		{/each}
	</ul>
	{#snippet failed(error)}
		<p data-testid="probe-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
