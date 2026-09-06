<script lang="ts">
	import SignIn from '#lib/SignIn.svelte';

	let { children } = $props();
</script>

<nav data-testid="app-nav">
	<a href="/">Home</a>
	<a href="/about">About</a>
	<a href="/items/42">Item 42</a>
	<a href="/todos">Todos</a>
	<a href="/api">API</a>
	<a href="/pricing">Pricing</a>
	<a href="/docs/guide/getting-started">Docs</a>
	<a href="/account">Account</a>
</nav>

<svelte:boundary>
	<SignIn />
	{#snippet pending()}
		<p data-testid="session-pending">…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="session-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<main>
	{@render children()}
</main>
