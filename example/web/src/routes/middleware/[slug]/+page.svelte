<script lang="ts">
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
</script>

<svelte:head><title>Middleware {data.slug} | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/middleware">Middleware</a></p>
	<h1 data-testid="title">Middleware {data.slug}</h1>
	<p data-testid="mw-slug-token">{data.token}</p>
	<p data-testid="mw-slug-route">{data.route}</p>
	<p data-testid="mw-slug-param">{data.slug}</p>
	<p data-testid="mw-slug-data">{data.data ? 'data request' : 'document request'}</p>
</main>

<style>
	.card { max-width: 45rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
</style>
