<script lang="ts">
	// resolve and asset are kit's own (runtime/app/paths/server.js), unchanged:
	// pure string logic over the app's compiled-in base and paths.relative
	// default, so calling them here needs no await and no boundary. match is
	// the one skgo answers itself — see internal/adapter's SSR_APP_PATHS — by
	// asking Go's own route table rather than a manifest this engine has none
	// of, which is why it is awaited below.
	import { resolve, asset, match } from '$app/paths';

	// /api/todos has no dynamic segments; /items/[id] does, and the app
	// already has a real /items/[id] route, so this exercises resolve's
	// parameter substitution against a route that actually exists.
	const toApiTodos = resolve('/api/todos');
	const toItem42 = resolve('/items/[id]', { id: '42' });
	const robots = asset('robots.txt');
	const dynamicLabel = "resolve('/items/[id]', id: '42') =";
</script>

<h1 data-testid="title">Render-time paths</h1>

<p>
	Every href below was computed by kit's own <code>$app/paths</code> functions while this
	page was rendered, not written into the markup by hand.
</p>

<p data-testid="resolve-static">resolve('/api/todos') = {toApiTodos}</p>
<p data-testid="resolve-dynamic">{dynamicLabel} {toItem42}</p>
<p data-testid="asset-href">asset('robots.txt') = {robots}</p>

<svelte:boundary>
	{@const matched = await match('/items/77')}
	<p data-testid="match-result">
		match('/items/77') = {matched ? `${matched.id} ${JSON.stringify(matched.params)}` : 'null'}
	</p>
	{#snippet failed(error)}
		<p data-testid="match-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>
