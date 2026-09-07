<script lang="ts">
	import { bumpTally } from './command.remote';
</script>

<h1 data-testid="title">Tally</h1>

<!--
	Calling a command while the page renders. Kit refuses it inside its own
	wrapper, the throw reaches the boundary the root `+error.svelte` guards,
	and `transformError` turns it into the 500 Internal Error the document is
	answered with. Nothing here reaches Go.
-->
<p data-testid="tally">{(await bumpTally()).runs}</p>
