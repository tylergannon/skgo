<script lang="ts">
	import Board from './Board.svelte';
</script>

<h1 data-testid="title">Live</h1>

<p>
	The board below is a <code>query.live</code>. Go answered it while this page was
	being rendered, so the number was in the HTML before any script ran, and Go
	pushes every later value down the same stream.
</p>

<svelte:boundary>
	<Board />
	{#snippet failed(error)}
		<p data-testid="board-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
