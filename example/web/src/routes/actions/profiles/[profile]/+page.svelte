<script lang="ts">
	import { enhance } from '$app/forms';
	import type { PageProps } from './$types';
	let { data, form }: PageProps = $props();
	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}
</script>

<svelte:head><title>Two profiles | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions">Actions</a></p>
	<h1 data-testid="profiles-title">Two visitor profiles</h1>
	<p>Choose a profile. Saving changes only the selected record.</p>
	<nav><a href="/actions/profiles/ada">Edit Ada</a> · <a href="/actions/profiles/grace">Edit Grace</a></nav>
	<p data-testid="selected-profile">Editing {data.selected}</p>
	<noscript><p data-testid="profiles-noscript">JavaScript is disabled. Native actions still work.</p></noscript>
	{#if form && 'receipt' in form}<p data-testid="profile-receipt">{form.receipt}</p>{/if}
	<div class="pair">
		<section aria-label="Ada profile">
			<h2>Ada record</h2>
			<p data-testid="ada-name">{data.ada.name}</p>
			<p data-testid="ada-email">{data.ada.email}</p>
			<p data-testid="ada-biography">{data.ada.biography}</p>
			<p data-testid="ada-state">{data.ada.state}</p>
		</section>
		<section aria-label="Grace profile">
			<h2>Grace record</h2>
			<p data-testid="grace-name">{data.grace.name}</p>
			<p data-testid="grace-email">{data.grace.email}</p>
			<p data-testid="grace-biography">{data.grace.biography}</p>
			<p data-testid="grace-state">{data.grace.state}</p>
		</section>
	</div>
	{#each ['native', 'enhanced'] as mode}
		<form method="POST" action="?/save" data-testid={mode + '-profile-form'} use:onlyEnhance={mode}>
			<h2>{mode === 'native' ? 'Native edit' : 'Enhanced edit'}</h2>
			<label>Name <input name="name" value={data.selected === 'ada' ? data.ada.name : data.grace.name} /></label>
			<label>Email <input name="email" value={data.selected === 'ada' ? data.ada.email : data.grace.email} /></label>
			<label>Biography <input name="biography" value={data.selected === 'ada' ? data.ada.biography : data.grace.biography} /></label>
			<button type="submit">Save selected profile</button>
		</form>
	{/each}
</main>

<style>
	.card { max-width: 55rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	form { border-top: 1px solid var(--line); padding: 0.7rem 0; }
	label { display: grid; margin: 0.4rem 0; }
	input { padding: 0.5rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
	.pair { display: grid; grid-template-columns: 1fr 1fr; gap: 1rem; }
	.pair section { border: 1px solid var(--line); padding: 0.7rem; }
</style>
