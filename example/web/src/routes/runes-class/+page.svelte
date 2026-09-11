<script lang="ts">
	import { Editor } from './editor.svelte';

	// Svelte compiles the class-field `$derived` in Editor to a private field
	// initialised by an arrow that reads `this` — the shape that used to take
	// the render down (#117).
	const editor = new Editor('one two three four five');
</script>

<h1 data-testid="title">Runes class</h1>
<p data-testid="word-count">Words: {editor.count}</p>
