<script lang="ts">
	let { children } = $props();
</script>

<nav>
	<a href="/">Home</a>
	<a href="/about">About</a>
	<a href="/items/42">Item 42</a>
</nav>

<main>
	{@render children()}
</main>
