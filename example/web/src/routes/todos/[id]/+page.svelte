<script lang="ts">
	import TodoDetail from '#lib/TodoDetail.svelte';
	import type { PageProps } from './$types';

	let { params }: PageProps = $props();
</script>

<h1 data-testid="title">Todo</h1>

<svelte:boundary>
	<TodoDetail id={params.id} />
	{#snippet pending()}
		<p data-testid="todo-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="todo-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<p><a href="/todos">Back to todos</a></p>
