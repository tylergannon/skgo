<script lang="ts">
	let { data } = $props();
</script>

<h1>Streaming</h1>
<p data-testid="fast">{data.fast}</p>
{#await data.slow}
	<p data-testid="slow">loading…</p>
{:then value}
	<p data-testid="slow">{value}</p>
{/await}
