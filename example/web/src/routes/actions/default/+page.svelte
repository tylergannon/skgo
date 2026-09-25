<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	let { form }: PageProps = $props();
	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}
</script>

<svelte:head><title>Default action | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions">Actions</a></p>
	<h1 data-testid="default-title">Default action, no page load</h1>
	<p>This page declares only an unnamed Go action. Its receipt is the typed form result.</p>
	<p><a href="/actions/default/saved">Read the stored default profile</a></p>
	<noscript><p data-testid="default-noscript">JavaScript is disabled. The form still works.</p></noscript>
	<section aria-label="Action result">
		<p data-testid="default-status">Page status {page.status}</p>
		{#if form && 'receipt' in form}
			<h2 data-testid="default-receipt">{form.receipt}</h2>
			<p data-testid="default-saved-name">{form.name}</p>
			<p data-testid="default-saved-email">{form.email}</p>
			<p data-testid="default-saved-biography">{form.biography}</p>
			<p data-testid="default-saved-state">{form.state}</p>
		{/if}
	</section>
	{#each ['native', 'enhanced'] as mode}
		<form method="POST" data-testid={mode + '-default-form'} use:onlyEnhance={mode}>
			<h2>{mode === 'native' ? 'Native submission' : 'Enhanced submission'}</h2>
			<label>Name <input name="name" value="Grace Hopper" /></label>
			<label>Email <input name="email" value="grace@example.test" /></label>
			<label>Biography <input name="biography" value="Compiler pioneer" /></label>
			<button type="submit">Save Grace</button>
		</form>
	{/each}
</main>

<style>
	.card { max-width: 45rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	form { border-top: 1px solid var(--line); padding: 0.7rem 0; }
	label { display: grid; margin: 0.4rem 0; }
	input { padding: 0.5rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
</style>
