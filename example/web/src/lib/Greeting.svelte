<script lang="ts">
	let { name = 'world' }: { name?: string } = $props();
</script>

<p data-testid="greeting">Hello, {name}! This is Svelte, served by Go.</p>
