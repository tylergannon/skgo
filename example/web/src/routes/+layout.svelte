<script lang="ts">
	import { building } from '$app/env';
	import { page } from '$app/state';
	import SignIn from '#lib/SignIn.svelte';

	let { children } = $props();

	/**
	 * The root layout is the one node in a branch that no `+error.svelte` can
	 * guard: kit wraps the root error page *inside* this layout rather than
	 * this layout inside it (`runtime/error-chain.js`), so the boundary around
	 * it has no `failed` snippet and a throw here has nowhere to go. That makes
	 * it the only place a page can be made to fail the way an engine failure
	 * fails, and /error/render is the fixture that does it.
	 */
	const failTheWholeDocument = () => {
		throw new Error('skgo: the root layout cannot render /error/render');
	};
</script>

{#if page.route.id === '/error/render'}
	{failTheWholeDocument()}
{/if}

<nav data-testid="app-nav">
	<a href="/">Home</a>
	<a href="/about">About</a>
	<a href="/items/42">Item 42</a>
	<a href="/todos">Todos</a>
	<a href="/empty">Empty</a>
	<a href="/api">API</a>
	<a href="/pricing">Pricing</a>
	<a href="/docs/guide/getting-started">Docs</a>
	<a href="/account">Account</a>
	<a href="/stream">Stream</a>
	<a href="/live">Live</a>
	<a href="/batch">Batch</a>
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
