<script lang="ts">
	import { page } from '$app/state';
	let clientInteraction = $state(false);
</script>

<svelte:head><title>Action error | skgo example</title></svelte:head>

<section class="card">
	<h1 data-testid="action-error-title">Action error {page.status}</h1>
	<p data-testid="action-error-message">{page.error?.message}</p>
	{#if page.error?.supportId}<p data-testid="action-error-support-id">{page.error.supportId}</p>{/if}
	<p><a data-testid="action-error-return" href="/actions">Return to the profile editor</a></p>
	<button data-testid="client-interaction" type="button" onclick={() => (clientInteraction = true)}>Check client interaction</button>
	{#if clientInteraction}<p data-testid="client-interaction-done">Client interaction complete</p>{/if}
</section>

<style>
	.card { max-width: 42rem; margin: 1rem auto; padding: 1rem 1.4rem; border: 1px solid var(--line); background: white; }
	button { padding: 0.55rem 0.85rem; border: 0; background: var(--green); color: white; font: inherit; cursor: pointer; }
</style>
