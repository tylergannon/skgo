<script lang="ts">
	import { submit } from './optional.remote';

	let withCount = $state(false);
	let withEnabled = $state(false);
	let withLabel = $state(false);
	const fields = submit.fields;
</script>

<h1>Optional form values</h1>
<form data-testid="optional-form" {...submit.enhance(async (form) => { await form.submit(); })}>
	<label>Name <input data-testid="optional-name" {...fields.name.as('text')} /></label>
	{#each fields.name.issues() ?? [] as issue}
		<p data-testid="optional-name-issue">{issue.message}</p>
	{/each}
	<label><input data-testid="include-count" type="checkbox" bind:checked={withCount} /> Include count</label>
	{#if withCount}
		<label>Count <input data-testid="optional-count" {...fields.count.as('number')} /></label>
	{/if}
	<label><input data-testid="include-enabled" type="checkbox" bind:checked={withEnabled} /> Include false</label>
	{#if withEnabled}
		<input {...fields.enabled.as('hidden', false)} />
	{/if}
	<label><input data-testid="include-label" type="checkbox" bind:checked={withLabel} /> Include label</label>
	{#if withLabel}
		<label>Label <input data-testid="optional-label" {...fields.label.as('text')} /></label>
	{/if}
	<button data-testid="optional-submit" type="submit">Submit</button>
</form>

{#if submit.result}
	<section data-testid="optional-result">
		<p data-testid="optional-result-name">Name: {submit.result.name}</p>
		<p data-testid="optional-result-count">Count: {submit.result.count}</p>
		<p data-testid="optional-result-enabled">Enabled: {submit.result.enabled}</p>
		<p data-testid="optional-result-label">Label: {submit.result.label}</p>
	</section>
{/if}
