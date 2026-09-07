<script lang="ts">
	import { getSite } from '../site.remote';
</script>

<h1 data-testid="title">Plain</h1>

<svelte:boundary>
	<p data-testid="site-name">{(await getSite()).name}</p>
	{#snippet failed(error)}
		<p data-testid="plain-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
