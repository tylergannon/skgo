<script lang="ts">
	import { getSite } from '../site.remote';
</script>

<h1 data-testid="title">SPA</h1>

<svelte:boundary>
	<p data-testid="site-name">{(await getSite()).name}</p>
	{#snippet pending()}
		<p data-testid="spa-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="spa-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
