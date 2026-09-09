<script lang="ts">
	import Ticker from './Ticker.svelte';

	// Four symbols, four components, one call into Go. Each row says how many
	// symbols were in the call that answered it.
	const symbols = ['SKGO', 'GOJA', 'KITX', 'SVLT'];
</script>

<h1 data-testid="title">Batch</h1>

<p>
	Each row below asked for one symbol on its own. They were answered by a single
	call to one Go function, holding every symbol at once — which is what each row
	reports.
</p>

<svelte:boundary>
	<table data-testid="quotes">
		<thead>
			<tr><th>Symbol</th><th>Price</th><th>Answered in a call of</th></tr>
		</thead>
		<tbody>
			{#each symbols as symbol (symbol)}
				<Ticker {symbol} />
			{/each}
		</tbody>
	</table>
	{#snippet failed(error)}
		<p data-testid="quotes-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<style>
	th {
		padding: 0.15rem 1rem 0.15rem 0;
		text-align: left;
	}
</style>
