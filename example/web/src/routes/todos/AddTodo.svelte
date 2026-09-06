<script lang="ts">
	import { addTodo, getTodos } from './todos.remote';

	let text = $state('');
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (text.trim() === '' || busy) return;
		busy = true;
		try {
			// Single flight: the command response carries the refreshed list.
			await addTodo(text).updates(getTodos());
			text = '';
		} finally {
			busy = false;
		}
	}
</script>

<form onsubmit={submit}>
	<input data-testid="new-todo" name="text" bind:value={text} placeholder="new todo" />
	<button data-testid="add-todo" type="submit" disabled={busy}>Add</button>
</form>
