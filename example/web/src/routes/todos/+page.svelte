<script lang="ts">
	import AddTodo from './AddTodo.svelte';
	import TodoCount from './TodoCount.svelte';
	import TodoList from './TodoList.svelte';
</script>

<h1 data-testid="title">Todos</h1>

<svelte:boundary>
	<TodoCount />
	{#snippet failed(error)}
		<p data-testid="count-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<svelte:boundary>
	<TodoList />
	{#snippet failed(error)}
		<p data-testid="todos-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<AddTodo />
