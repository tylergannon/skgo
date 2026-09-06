<script lang="ts">
	import { greet, status } from './hello.remote';

	let name = $state('world');
	let busy = $state(false);

	async function submit(event: SubmitEvent) {
		event.preventDefault();
		if (busy) return;
		busy = true;
		try {
			// A command is a POST, so this is also the check that the app's
			// origin and the binary's agree. Single flight: the command's
			// response carries the refreshed query.
			await greet(name).updates(status());
		} finally {
			busy = false;
		}
	}
</script>

<svelte:boundary>
	{@const s = await status()}
	<h1 data-testid="title">{s.name}</h1>
	<p data-testid="answered-by">Served by {s.goVersion}.</p>
	<p>Greetings so far: <strong data-testid="greetings">{s.greetings}</strong></p>
	<p data-testid="last-greeting">Last greeting: {s.lastGreeting || '(none yet)'}</p>
	{#snippet pending()}
		<p data-testid="status-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="status-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<form onsubmit={submit}>
	<input data-testid="name" bind:value={name} />
	<button data-testid="greet" type="submit" disabled={busy}>Greet</button>
</form>
