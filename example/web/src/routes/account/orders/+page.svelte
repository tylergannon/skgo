<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="title">Orders</h1>
<p data-testid="order-total">{data.orderTotal} orders</p>

{#await data.orders}
	<p data-testid="orders-pending">fetching the orders…</p>
{:then orders}
	<ul data-testid="orders">
		{#each orders as order}
			<li data-testid="order">{order.item}</li>
		{/each}
	</ul>
{/await}
