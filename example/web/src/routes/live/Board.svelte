<script lang="ts">
	import { watchBoard } from './board.remote';

	const board = $derived(await watchBoard());
</script>

<section class="board">
	<p>Todos on the board: <strong data-testid="board-count">{board.count}</strong></p>
	<p>Newest: <strong data-testid="board-newest">{board.newest}</strong></p>
	<p>Stream frame: <strong data-testid="board-push">{board.push}</strong></p>
</section>

<style>
	.board {
		border: 1px solid currentColor;
		display: inline-block;
		padding: 0.5rem 1rem;
	}
</style>
