<script lang="ts">
	import { page } from '$app/state';
	let clientInteraction = $state(false);
</script>

<svelte:head><title>Cross-page action error | skgo example</title></svelte:head>
<main class="card">
	<h1 data-testid="cross-error-title">Cross-page action error {page.status}</h1>
	<noscript><p data-testid="cross-error-noscript">JavaScript is disabled. This error page was rendered by Go.</p></noscript>
	<p data-testid="cross-error-message">{page.error?.message}</p>
	{#if page.error?.supportId}<p data-testid="cross-error-support-id">{page.error.supportId}</p>{/if}
	<p><a data-testid="cross-error-return" href="/actions/cross/send">Return to cross-page forms</a></p>
	<button data-testid="client-interaction" type="button" onclick={() => (clientInteraction = true)}>Check client interaction</button>
	{#if clientInteraction}<p data-testid="client-interaction-done">Client interaction complete</p>{/if}
</main>

<style>
	.card { max-width: 42rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
</style>
