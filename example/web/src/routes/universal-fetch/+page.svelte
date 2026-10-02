<script lang="ts">
	let { data } = $props();
	const m = $derived(data.main);
</script>

<h1 data-testid="title">Universal fetch</h1>
<p>
	This page's universal load fetches the app's replay endpoints. Each answer is
	in the document, and the browser reuses it instead of asking again.
</p>
<a data-testid="next-step" href={`/universal-fetch?run=${data.run}&step=${Number(data.step) + 1}`}>Next step</a>
<p data-testid="step">{data.step}</p>

{#if m}
	<dl>
		<dt>text</dt><dd data-testid="uf-text">{m.text}</dd>
		<dt>json</dt><dd data-testid="uf-json">{m.json}</dd>
		<dt>rewritten by the hook</dt><dd data-testid="uf-aliased">{m.aliased}</dd>
		<dt>binary</dt><dd data-testid="uf-binary">{m.binary}</dd>
		<dt>allowed header</dt><dd data-testid="uf-shown">{m.shown}</dd>
		<dt>stream</dt><dd data-testid="uf-streamed">{m.streamed}</dd>
		<dt>script-breaking text</dt><dd data-testid="uf-breaking">{m.breaking}</dd>
		<dt>no body</dt><dd data-testid="uf-empty">{m.empty}</dd>
		<dt>not found</dt><dd data-testid="uf-missing">{m.missing}</dd>
		<dt>header a</dt><dd data-testid="uf-header-a">{m.headerA}</dd>
		<dt>header b</dt><dd data-testid="uf-header-b">{m.headerB}</dd>
		<dt>body alpha</dt><dd data-testid="uf-body-alpha">{m.bodyAlpha}</dd>
		<dt>body beta</dt><dd data-testid="uf-body-beta">{m.bodyBeta}</dd>
		<dt>form body</dt><dd data-testid="uf-form">{m.form}</dd>
		<dt>cached</dt><dd data-testid="uf-cached">{m.cached}</dd>
		<dt>rendered page</dt><dd data-testid="uf-nested">{m.nested}</dd>
	</dl>
{:else}
	<pre data-testid="uf-probe">{JSON.stringify(data.probe)}</pre>
{/if}
