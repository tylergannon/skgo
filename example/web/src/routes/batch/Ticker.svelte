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
