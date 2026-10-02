<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="title">Universal fetch without a script</h1>
<pre data-testid="uf-probe">{JSON.stringify(data.probe)}</pre>
