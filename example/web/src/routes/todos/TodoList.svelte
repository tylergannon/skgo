<script lang="ts">
	import { getTodos } from './todos.remote';
</script>

<ul data-testid="todos">
	{#each await getTodos() as todo (todo.id)}
		<li data-testid="todo" class:private={todo.private}>
			<a href="/todos/{todo.id}">{todo.text}</a>
		</li>
	{/each}
</ul>

<style>
	.private::after {
		content: ' (private)';
	}
</style>
