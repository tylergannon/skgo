<script lang="ts">
	import { getBanner, getNote, writeNotes } from './gate.remote';

	// Three query instances: two of one query, told apart by their argument,
	// and one of another. The page asks for all three back on every write.
	// What actually comes back is `writeNotes` in gate.remote.go deciding.
	const left = getNote('left');
	const right = getNote('right');
	const banner = getBanner();

	let leftText = $state('');
	let rightText = $state('');
	let bannerText = $state('');
	let busy = $state(false);
	let wrote = $state(0);

	async function write(event: SubmitEvent) {
		event.preventDefault();
		if (busy) return;
		busy = true;
		try {
			const ack = await writeNotes({
				left: leftText,
				right: rightText,
				banner: bannerText
			}).updates(left, right, banner);
			wrote = ack.wrote;
		} finally {
			busy = false;
		}
	}

	// Ordinary refetches, one request each. They are how this page shows that
	// the command wrote everything it said it wrote: whatever went stale here
	// comes back changed.
	async function reload() {
		if (busy) return;
		busy = true;
		try {
			await Promise.all([left.refresh(), right.refresh(), banner.refresh()]);
		} finally {
			busy = false;
		}
	}
</script>

<h1 data-testid="title">The refresh gate</h1>

<p>
	Writing changes both notes and the banner, and asks for all three back. The
	handler accepts one instance of <code>getNote</code> and never mentions
	<code>getBanner</code>.
</p>

<div class="panels">
	<article data-testid="gate-panel" data-id="left">
		<h2>left note</h2>
		{#if left.error}
			<p class="refused" data-testid="note-refused-left">{left.error.message}</p>
		{:else if left.current}
			<p data-testid="note-left">{left.current.text}</p>
		{:else}
			<p data-testid="gate-pending">loading…</p>
		{/if}
	</article>

	<article data-testid="gate-panel" data-id="right">
		<h2>right note</h2>
		{#if right.error}
			<p class="refused" data-testid="note-refused-right">{right.error.message}</p>
		{:else if right.current}
			<p data-testid="note-right">{right.current.text}</p>
		{:else}
			<p data-testid="gate-pending">loading…</p>
		{/if}
	</article>

	<article data-testid="gate-panel" data-id="banner">
		<h2>banner</h2>
		{#if banner.error}
			<p class="refused" data-testid="note-refused-banner">{banner.error.message}</p>
		{:else if banner.current}
			<p data-testid="note-banner">{banner.current.text}</p>
		{:else}
			<p data-testid="gate-pending">loading…</p>
		{/if}
	</article>
</div>

<form onsubmit={write}>
	<label>
		left
		<input data-testid="gate-left" name="left" bind:value={leftText} />
	</label>
	<label>
		right
		<input data-testid="gate-right" name="right" bind:value={rightText} />
	</label>
	<label>
		banner
		<input data-testid="gate-banner" name="banner" bind:value={bannerText} />
	</label>
	<button data-testid="gate-write" type="submit" disabled={busy}>Write all three</button>
</form>

<p>The command reported writing <strong data-testid="gate-wrote">{wrote}</strong> values.</p>

<button data-testid="gate-reload" onclick={reload} disabled={busy}>Reload all three</button>

<p><a href="/todos">Back to todos</a></p>

<style>
	/* Three boxes, because a screenshot of this page is how a human checks
	   which of them moved and which did not. */
	.panels {
		display: flex;
		gap: 1rem;
		flex-wrap: wrap;
	}
	article {
		border: 1px solid currentColor;
		padding: 0.25rem 1rem;
		min-width: 16rem;
	}
	.refused {
		font-style: italic;
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
