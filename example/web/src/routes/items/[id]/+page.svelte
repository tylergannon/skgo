<script lang="ts">
	import type { PageProps } from './$types';
	import { getItem } from './item.remote';

	let { params }: PageProps = $props();
</script>

<h1 data-testid="title">Item {params.id}</h1>

<svelte:boundary>
	{#if params.id}
		{@const item = await getItem(params.id)}
		<p data-testid="item-name">{item.name}</p>
		<p data-testid="colocated">{item.colocated}</p>
	{/if}
	{#snippet failed(error)}
		<p data-testid="item-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
