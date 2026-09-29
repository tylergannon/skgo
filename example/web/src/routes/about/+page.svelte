<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="title">About</h1>
<p>skgo serves this SvelteKit app from a single Go binary.</p>
<p data-testid="prerender-parent">{data.parentDeployment}</p>
<p data-testid="prerender-price">{data.price.format()}</p>
{#await data.later then value}
	<p data-testid="prerender-deferred">{value}</p>
{/await}
<p data-testid="prerender-remote">{data.remoteReceipt}</p>
