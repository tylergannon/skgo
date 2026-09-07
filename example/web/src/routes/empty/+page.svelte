<script lang="ts">
	import { getKnownReport, getModels, getReport, reparse } from './empty.remote';

	// The load's own slice, which travelled inside the document.
	let { data } = $props();

	// What the command said. Nothing is on screen until the button is pressed,
	// so this cannot show a number the page had already.
	let reparsed = $state<{ title: string; diagnostics: unknown[] } | null>(null);
</script>

<h1 data-testid="title">Empty</h1>

<!--
	No `pending` snippet on any boundary here, deliberately. Svelte's server
	compiler renders a boundary's pending snippet *instead of* its children
	(compiler/phases/3-transform/server/visitors/SvelteBoundary.js), so a
	boundary with one is a boundary whose list never reaches the document — and
	this page's whole claim is about what a cold load already has in it.

	Every count below is `.length` on a value the Go type declares as a slice.
	A null there throws a TypeError, the boundary's `failed` snippet renders,
	and the suite's "every part of the page loaded" step fails on it.
-->

<section>
	<h2>The load's notes</h2>
	<p data-testid="notes-count">{data.notes.length} notes</p>
	<ul data-testid="notes">
		{#each data.notes as note (note)}
			<li data-testid="note">{note}</li>
		{:else}
			<li data-testid="notes-empty">No notes.</li>
		{/each}
	</ul>
</section>

<svelte:boundary>
	{@const report = await getReport()}
	<section>
		<h2 data-testid="report-title">{report.title}</h2>
		<p data-testid="report-count">{report.diagnostics.length} diagnostics</p>
		<ul data-testid="report-diagnostics">
			{#each report.diagnostics as diagnostic (diagnostic.message)}
				<li data-testid="report-diagnostic">{diagnostic.message}</li>
			{:else}
				<li data-testid="report-empty">No diagnostics.</li>
			{/each}
		</ul>
		<p data-testid="report-models">{report.models.length} models</p>
	</section>
	{#snippet failed(error)}
		<p data-testid="report-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<svelte:boundary>
	{@const models = await getModels()}
	<section>
		<p data-testid="models-count">{models.length} models</p>
		<ul data-testid="models">
			{#each models as model (model)}
				<li data-testid="model">{model}</li>
			{:else}
				<li data-testid="models-empty">No models.</li>
			{/each}
		</ul>
	</section>
	{#snippet failed(error)}
		<p data-testid="models-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<svelte:boundary>
	{@const known = await getKnownReport()}
	<section>
		<h2 data-testid="known-title">{known.title}</h2>
		<p data-testid="known-count">{known.diagnostics.length} diagnostics</p>
		<ul data-testid="known-diagnostics">
			{#each known.diagnostics as diagnostic (diagnostic.message)}
				<li data-testid="known-diagnostic">{diagnostic.message}</li>
			{:else}
				<li data-testid="known-empty">No diagnostics.</li>
			{/each}
		</ul>
	</section>
	{#snippet failed(error)}
		<p data-testid="known-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<section>
	<button data-testid="reparse" onclick={async () => (reparsed = await reparse())}>
		Reparse
	</button>
	{#if reparsed}
		<p data-testid="reparse-count">
			{reparsed.title}: {reparsed.diagnostics.length} diagnostics
		</p>
	{/if}
</section>
