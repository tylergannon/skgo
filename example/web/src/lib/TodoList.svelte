<script lang="ts">
	import { getTodos } from '#lib/todos.remote';
</script>

<ul data-testid="todos">
	{#each await getTodos() as todo (todo.id)}
		<li data-testid="todo">
			<a href="/todos/{todo.id}">{todo.text}</a>
		</li>
	{/each}
</ul>
