<script lang="ts">
	import { getTodo } from '#lib/todos.remote';

	let { id }: { id: string } = $props();
</script>

<article data-testid="todo-detail">
	<p data-testid="todo-id">{id}</p>
	<p data-testid="todo-text">{(await getTodo(id)).text}</p>
</article>
