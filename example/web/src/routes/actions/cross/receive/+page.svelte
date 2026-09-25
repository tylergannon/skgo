<script lang="ts">
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	let { data, form }: PageProps = $props();
	let clientInteraction = $state(false);
	let rejected = $derived(form && 'emailError' in form ? form : null);
</script>

<svelte:head><title>Cross-page result | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions/cross/send">Return to cross-page forms</a> · <a href="/actions">Actions</a></p>
	<h1 data-testid="cross-destination-title">Cross-page destination</h1>
	<p data-testid="cross-source-query">Source {page.url.searchParams.get('source') ?? 'none'}</p>
	<noscript><p data-testid="cross-noscript">JavaScript is disabled. The destination is rendered by Go.</p></noscript>
	<div class="columns">
		<section aria-label="Destination action result">
			<h2>Destination result</h2>
			<p data-testid="cross-status">Page status {page.status}</p>
			{#if form && 'receipt' in form && form.price}
				<p data-testid="cross-receipt">{form.receipt}</p>
				<p data-testid="cross-money">{form.price.format()}</p>
			{:else if rejected}
				<p data-testid="cross-validation">Edit rejected; correct the email and try again.</p>
				<p data-testid="cross-email-error">{rejected.emailError}</p>
			{:else}
				<p>No action result on this GET.</p>
			{/if}
			{#if rejected}
				<form method="POST" action="?/save&source=invite" novalidate>
					<h3>Correct the rejected edit</h3>
					<label>Name <input name="name" value={rejected.name} /></label>
					<label>Email <input name="email" value={rejected.email} /></label>
					<label>Biography <input name="biography" value={rejected.biography} /></label>
					<button type="submit">Try the edit again</button>
				</form>
			{/if}
		</section>
		<section aria-label="Saved destination profile">
			<h2>Saved destination profile</h2>
			<p data-testid="cross-saved-name">{data.profile.name}</p>
			<p data-testid="cross-saved-email">{data.profile.email}</p>
			<p data-testid="cross-saved-biography">{data.profile.biography}</p>
			<p data-testid="cross-saved-state">{data.profile.state}</p>
		</section>
	</div>
	<button data-testid="client-interaction" type="button" onclick={() => (clientInteraction = true)}>Check client interaction</button>
	{#if clientInteraction}<p data-testid="client-interaction-done">Client interaction complete</p>{/if}
</main>

<style>
	.card { max-width: 55rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	.columns { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
	.columns section { border: 1px solid var(--line); padding: 0.8rem; }
	.columns p { margin: 0.35rem 0; }
	form { margin-top: 0.6rem; }
	form h3 { margin: 0.3rem 0; }
	label { display: grid; margin: 0.25rem 0; }
	input { padding: 0.35rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
	@media (max-width: 700px) { .columns { grid-template-columns: 1fr; } }
</style>
