<script lang="ts">
	import Greeting from '#lib/Greeting.svelte';
	import { getSite } from './site.remote';
</script>

<h1 data-testid="title">Home</h1>
<Greeting name="skgo" />

<svelte:boundary>
	{@const site = await getSite()}
	<p data-testid="site-name">{site.name}</p>
	<p data-testid="colocated">{site.colocated}</p>
	{#snippet failed(error)}
		<p data-testid="site-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
