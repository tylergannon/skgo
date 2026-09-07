<script lang="ts">
	import { building } from '$app/env';
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

<!--
	No `pending` snippet, deliberately. Svelte's server compiler emits the
	pending block *instead of* the children of a boundary that has one
	(svelte/src/compiler/phases/3-transform/server/visitors/SvelteBoundary.js),
	so a boundary with a pending snippet is a boundary whose contents never
	reach the document — and the session would then be fetched again the moment
	the page hydrated. The session is on every page; it arrives with it.
-->
<svelte:boundary>
	<!--
		`building` is kit's own answer to a prerendered page that wants
		per-request data: there is no visitor while the build runs, so a page
		the build writes to disk shows no session and picks one up when it
		hydrates. Without it, kit refuses the query and the prerender fails.
	-->
	{#if !building}
		<SignIn />
	{/if}
	{#snippet failed(error)}
		<p data-testid="session-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<main>
	{@render children()}
</main>
