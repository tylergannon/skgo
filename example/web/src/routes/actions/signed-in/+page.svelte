<script lang="ts">
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
	let clientInteraction = $state(false);
</script>

<svelte:head><title>Signed in | skgo example</title></svelte:head>

<section class="card">
	{#if data.required}<h1 data-testid="hook-sign-in-title">Sign in required</h1>
		<p data-testid="hook-sign-in-message">The request was intercepted before the profile action ran.</p>
	{:else}<h1 data-testid="signed-in-title">Signed in as {data.user}</h1>{/if}
	<noscript><p data-testid="signed-in-noscript">JavaScript is disabled. The Go session still works.</p></noscript>
	{#if !data.required}<p data-testid="signed-in-cookie">The session cookie was read by the Go page load.</p>{/if}
	<p><a href="/actions">Return to the profile editor</a></p>
	<button data-testid="client-interaction" type="button" onclick={() => (clientInteraction = true)}>Check client interaction</button>
	{#if clientInteraction}<p data-testid="client-interaction-done">Client interaction complete</p>{/if}
</section>

<style>
	.card { max-width: 42rem; margin: 1rem auto; padding: 1rem 1.4rem; border: 1px solid var(--line); background: white; }
	button { padding: 0.55rem 0.85rem; border: 0; background: var(--green); color: white; font: inherit; cursor: pointer; }
</style>
