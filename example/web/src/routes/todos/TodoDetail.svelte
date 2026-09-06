<script lang="ts">
	import { getTodo, renameTodo } from './todos.remote';

	let { id }: { id: string } = $props();

	let draft = $state('');
	let busy = $state(false);

	async function rename(event: SubmitEvent) {
		event.preventDefault();
		if (draft.trim() === '' || busy) return;
		busy = true;
		try {
			// Single flight again: the command's own response carries the new
			// value of the query this page is showing, so nothing re-fetches.
			await renameTodo({ id, text: draft }).updates(getTodo(id));
			draft = '';
		} finally {
			busy = false;
		}
	}
</script>

<article data-testid="todo-detail">
	<p data-testid="todo-id">{id}</p>
	<p data-testid="todo-text">{(await getTodo(id)).text}</p>
</article>

<form onsubmit={rename}>
	<input data-testid="rename-todo" name="text" bind:value={draft} placeholder="rename" />
	<button data-testid="save-todo" type="submit" disabled={busy}>Rename</button>
</form>
