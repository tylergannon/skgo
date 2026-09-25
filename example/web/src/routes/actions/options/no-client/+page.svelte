<script lang="ts">
	import { page } from '$app/state';
	import type { PageProps } from './$types';
	let { data, form }: PageProps = $props();
	let rejected = $derived(form && 'emailError' in form ? form : null);
</script>

<svelte:head><title>Action without client JavaScript | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions">Actions</a></p>
	<h1 data-testid="no-client-title">Actions without client JavaScript</h1>
	<p>The Go action and page load render this document. This page sets csr=false.</p>
	<noscript><p data-testid="no-client-noscript">JavaScript is disabled; the native form still works.</p></noscript>
	<div class="columns">
	<div>
	<form data-testid="no-client-form" method="POST" action="?/save" novalidate>
		<label>Name <input name="name" value={rejected ? rejected.name : data.profile.name} /></label>
		<label>Email <input name="email" value={rejected ? rejected.email : data.profile.email} /></label>
		{#if rejected}<p data-testid="no-client-email-error" class="error">{rejected.emailError}</p>{/if}
		<label>Biography <textarea name="biography">{rejected ? rejected.biography : data.profile.biography}</textarea></label>
		<button type="submit">Save profile</button>
	</form>
	<form method="POST" action="?/forbidden"><button type="submit">Try forbidden action</button></form>
	<form method="POST" action="?/unavailable"><button type="submit">Try unavailable action</button></form>
	</div>
	<aside aria-label="Action result and saved profile">
		<h2>Action result</h2>
		<p data-testid="no-client-status">Page status {page.status}</p>
		{#if form && 'receipt' in form}<p data-testid="no-client-receipt">{form.receipt}</p>{/if}
		<h2>Saved profile</h2>
		<p data-testid="no-client-saved-name">{data.profile.name}</p>
		<p data-testid="no-client-saved-email">{data.profile.email}</p>
		<p data-testid="no-client-saved-biography">{data.profile.biography}</p>
		<p data-testid="no-client-saved-state">{data.profile.state}</p>
	</aside>
	</div>
	<section aria-label="Client rendering probe">
		<h2>Try a client-rendered destination</h2>
		<form method="POST" action="/actions/options/no-ssr?/save" novalidate>
			<input type="hidden" name="name" value="Grace Hopper" />
			<input type="hidden" name="email" value="grace@example.test" />
			<input type="hidden" name="biography" value="Compiler pioneer" />
			<button type="submit">Post valid client-rendered edit</button>
		</form>
		<form method="POST" action="/actions/options/no-ssr?/save" novalidate>
			<input type="hidden" name="name" value="Grace Hopper" />
			<input type="hidden" name="email" value="grace-at-example" />
			<input type="hidden" name="biography" value="Keep this biography" />
			<button type="submit">Post invalid client-rendered edit</button>
		</form>
		<form method="POST" action="/actions/options/no-ssr?/forbidden"><button type="submit">Post forbidden client-rendered edit</button></form>
		<form method="POST" action="/actions/options/no-ssr?/unavailable"><button type="submit">Post unavailable client-rendered edit</button></form>
	</section>
</main>

<style>
	.card { max-width: 68rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	.columns { display: grid; grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr); gap: 1rem; }
	aside { padding: 0.8rem; border: 1px solid var(--line); align-self: start; }
	aside h2 { margin: 0.2rem 0 0.5rem; }
	aside p { overflow-wrap: anywhere; }
	form { border-top: 1px solid var(--line); margin: 1rem 0; padding-top: 0.7rem; }
	label { display: grid; gap: 0.25rem; margin: 0.5rem 0; }
	input, textarea { padding: 0.5rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
	.error { color: #a32020; font-weight: 700; }
	@media (max-width: 700px) { .columns { grid-template-columns: 1fr; } }
</style>
