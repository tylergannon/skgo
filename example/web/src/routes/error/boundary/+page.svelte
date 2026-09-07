<script lang="ts">
	import { readSensor } from './boundary.remote';
</script>

<h1 data-testid="title">Sensor</h1>

<!--
	The page handles its own failure: the boundary's `failed` snippet shows the
	message and the rest of the page is untouched. Kit still runs the render's
	`transformError` on the way in, so `page.status` becomes the caught error's
	and the document is answered with it.
-->
<svelte:boundary>
	<p data-testid="reading">{(await readSensor()).celsius}</p>
	{#snippet failed(error)}
		<p data-testid="sensor-failed">{(error as { message: string }).message}</p>
	{/snippet}
</svelte:boundary>
