<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	let { data, form }: PageProps = $props();
	let rejected = $derived(form && 'emailError' in form ? form : null);
	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}
</script>

<svelte:head><title>Action with client rendering | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions">Actions</a></p>
	<h1 data-testid="no-ssr-title">Client-rendered actions</h1>
	<p>This page sets ssr=false. A native POST returns a shell; Kit draws the ordinary page after boot.</p>
	<noscript><p data-testid="no-ssr-noscript">JavaScript is disabled; this page needs client rendering.</p></noscript>
	<div class="columns">
	<div>
	{#each ['native', 'enhanced'] as mode}
		<form data-testid={mode + '-no-ssr-form'} method="POST" action="?/save" novalidate use:onlyEnhance={mode}>
			<h2>{mode === 'native' ? 'Native save' : 'Enhanced save'}</h2>
			<label>Name <input name="name" value={rejected ? rejected.name : data.profile.name} /></label>
			<label>Email <input name="email" value={rejected ? rejected.email : data.profile.email} /></label>
			<label>Biography <textarea name="biography">{rejected ? rejected.biography : data.profile.biography}</textarea></label>
			<button type="submit">Save profile</button>
		</form>
	{/each}
	<form method="POST" action="?/signIn"><input type="hidden" name="username" value="ada" /><button type="submit">Native sign in</button></form>
	<form method="POST" action="?/forbidden"><button type="submit">Try forbidden action</button></form>
	<form method="POST" action="?/unavailable"><button type="submit">Try unavailable action</button></form>
	</div>
	<aside aria-label="Action result and saved profile">
		<h2>Action result</h2>
		<p data-testid="no-ssr-status">Page status {page.status}</p>
		{#if form && 'receipt' in form}<p data-testid="no-ssr-receipt">{form.receipt}</p>{/if}
		{#if rejected}<p data-testid="no-ssr-email-error" class="error">{rejected.emailError}</p>{/if}
		<h2>Saved profile</h2>
		<p data-testid="no-ssr-saved-name">{data.profile.name}</p>
		<p data-testid="no-ssr-saved-email">{data.profile.email}</p>
		<p data-testid="no-ssr-saved-biography">{data.profile.biography}</p>
		<p data-testid="no-ssr-saved-state">{data.profile.state}</p>
	</aside>
	</div>
</main>

<style>
	.card { max-width: 68rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	.columns { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 1rem; }
	aside { position: sticky; top: 0.5rem; padding: 0.8rem; border: 1px solid var(--line); align-self: start; }
	aside h2 { margin: 0.2rem 0 0.5rem; }
	aside p { overflow-wrap: anywhere; }
	form { border-top: 1px solid var(--line); margin: 1rem 0; padding-top: 0.7rem; }
	label { display: grid; gap: 0.25rem; margin: 0.5rem 0; }
	input, textarea { padding: 0.5rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
	.error { color: #a32020; font-weight: 700; }
	@media (max-width: 700px) { .columns { grid-template-columns: 1fr; } }
</style>
