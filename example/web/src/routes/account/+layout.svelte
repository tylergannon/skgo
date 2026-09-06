<script lang="ts">
	import { invalidate } from '$app/navigation';

	let { data, children } = $props();
</script>

<section>
	<p data-testid="account-user">Account of {data.accountUser}</p>
	<p data-testid="account-serial">{data.accountSerial}</p>
	<nav>
		<a href="/account">Overview</a>
		<a href="/account/orders">Orders</a>
	</nav>
	<button data-testid="refresh-account" onclick={() => invalidate('app:account')}>
		Refresh the account
	</button>

	{@render children()}
</section>
