<script lang="ts">
	import PairPanel from '../PairPanel.svelte';
	import { retitleTodo } from '../todos.remote';

	// Two instances of one query, told apart only by their argument. Kit's
	// client caches each under its own key, so a refresh that names one of
	// them must leave the other exactly where it was.
	const left = 'p1';
	const right = 'p2';

	let id = $state(left);
	let text = $state('');
	let refreshId = $state(left);
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy) return;
		busy = true;
		try {
			// No `.updates(...)`. The browser names nothing it wants refreshed,
			// so anything that changes on this page after the command landed
			// changed because the Go handler asked for it by name.
			await retitleTodo({ id, text, refreshId });
		} finally {
			busy = false;
		}
	}
</script>

<h1 data-testid="title">A pair of todos</h1>

<div class="pair">
	<svelte:boundary>
		<PairPanel id={left} />
		{#snippet pending()}
			<p data-testid="pair-pending">loading…</p>
		{/snippet}
		{#snippet failed(error)}
			<p data-testid="pair-failed">{(error as Error).message}</p>
		{/snippet}
	</svelte:boundary>

	<svelte:boundary>
		<PairPanel id={right} />
		{#snippet pending()}
			<p data-testid="pair-pending">loading…</p>
		{/snippet}
		{#snippet failed(error)}
			<p data-testid="pair-failed">{(error as Error).message}</p>
		{/snippet}
	</svelte:boundary>
</div>

<form onsubmit={submit}>
	<label>
		retitle
		<input data-testid="retitle-id" name="id" bind:value={id} />
	</label>
	<label>
		to
		<input data-testid="retitle-text" name="text" bind:value={text} placeholder="new text" />
	</label>
	<label>
		then refresh
		<input data-testid="retitle-refresh" name="refreshId" bind:value={refreshId} />
	</label>
	<button data-testid="retitle-save" type="submit" disabled={busy}>Retitle</button>
</form>

<p><a href="/todos">Back to todos</a></p>

<style>
	.pair {
		display: flex;
		gap: 2rem;
	}
	form {
		margin-top: 1rem;
		display: flex;
		gap: 1rem;
		align-items: end;
		flex-wrap: wrap;
	}
	label {
		display: flex;
		flex-direction: column;
	}
</style>
