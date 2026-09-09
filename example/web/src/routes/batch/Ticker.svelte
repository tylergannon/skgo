<script lang="ts">
	import { getQuotes } from './quotes.remote';

	let { symbol }: { symbol: string } = $props();

	const quote = $derived(await getQuotes(symbol));
</script>

<tr data-testid="quote" data-symbol={symbol}>
	<td data-testid="quote-symbol">{quote.symbol}</td>
	<td data-testid="quote-price">{(quote.cents / 100).toFixed(2)}</td>
	<td data-testid="quote-batch">batch of {quote.batchSize}</td>
</tr>

<style>
	td {
		padding: 0.15rem 1rem 0.15rem 0;
		text-align: left;
	}
</style>
