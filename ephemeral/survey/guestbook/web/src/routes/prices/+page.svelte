<script lang="ts">
	import { Money } from '#lib/money';
	import { getListPrice, getSalePrice } from './prices.remote';
</script>

<h1>Prices</h1>
{#await getListPrice() then price}
	<p>
		List price: <span data-testid="price">{price.format()}</span>
		(<span data-testid="price-is-money">{price instanceof Money ? 'Money' : 'plain'}</span>)
	</p>
{/await}
{#await getSalePrice() then sale}
	<p>
		Sale price: <span data-testid="sale-price">{sale.format()}</span>
		(<span data-testid="sale-price-is-money">{sale instanceof Money ? 'Money' : 'plain'}</span>)
	</p>
{/await}
