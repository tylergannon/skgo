<script lang="ts">
	import AddTodo from './AddTodo.svelte';
	import TodoCount from './TodoCount.svelte';
	import TodoList from './TodoList.svelte';
</script>

<h1 data-testid="title">Todos</h1>

<svelte:boundary>
	<TodoCount />
	{#snippet pending()}
		<p data-testid="count-pending">counting…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="count-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<svelte:boundary>
	<TodoList />
	{#snippet pending()}
		<p data-testid="todos-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="todos-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<AddTodo />
