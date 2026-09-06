<script lang="ts">
	let { data } = $props();
</script>

<h1>About</h1>
<p data-testid="built-at">This page was rendered at {data.builtAt}.</p>
