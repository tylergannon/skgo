<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="title">Secret</h1>
<p data-testid="secret">{data.secret}</p>
