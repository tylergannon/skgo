<script lang="ts">
	import type { PageProps } from './$types';
	import { getPage } from './docs.remote';

	let { params }: PageProps = $props();
</script>

<h1 data-testid="title">Docs</h1>

<svelte:boundary>
	{@const doc = await getPage(params.rest ?? '')}
	<p data-testid="doc-title">{doc.title}</p>
	<p data-testid="doc-depth">{doc.depth}</p>
	{#snippet pending()}
		<p data-testid="doc-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="doc-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
