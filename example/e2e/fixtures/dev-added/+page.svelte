<script lang="ts">
	let { data } = $props();
</script>

<svelte:head><title>Added while running</title></svelte:head>
<h1 data-testid="title">Added while running</h1>
<p data-testid="live-route-load">{data.message}</p>

<style>
	h1 { color: #176b47; }
</style>
