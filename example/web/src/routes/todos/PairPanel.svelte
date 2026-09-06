<script lang="ts">
	import { getTodo } from './todos.remote';

	let { id }: { id: string } = $props();
</script>

<article data-testid="pair-panel" data-id={id}>
	<h2 data-testid="pair-id">{id}</h2>
	<p data-testid="pair-text">{(await getTodo(id)).text}</p>
</article>

<style>
	/* The two panels are the whole point of this page, and a screenshot of it
	   is how a human checks that only one of them moved — so they are drawn as
	   two boxes rather than as two paragraphs that run together. */
	article {
		border: 1px solid currentColor;
		padding: 0.25rem 1rem;
		min-width: 18rem;
	}
</style>
