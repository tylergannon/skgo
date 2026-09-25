<script lang="ts">
	import { enhance } from '$app/forms';
	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}
</script>

<svelte:head><title>Cross-page actions | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions">Actions</a> · <a href="/actions/cross/receive">View destination profile</a></p>
	<h1 data-testid="cross-source-title">Submit to another page</h1>
	<p>These forms post to a different Go page. The destination displays its own load and the action result.</p>
	<noscript><p data-testid="cross-noscript">JavaScript is disabled. Native cross-page actions still work.</p></noscript>
	{#each ['native', 'enhanced'] as mode}
		<section aria-label={mode + ' cross-page actions'}>
			<h2>{mode === 'native' ? 'Native forms' : 'Enhanced forms'}</h2>
			<form method="POST" action="/actions/cross/receive?/save&source=invite" novalidate data-testid={mode + '-cross-save'} use:onlyEnhance={mode}>
				<label>Name <input name="name" value="Grace Hopper" /></label>
				<label>Email <input name="email" value="grace@example.test" /></label>
				<label>Biography <input name="biography" value="Compiler pioneer" /></label>
				<button type="submit">Save Grace on destination</button>
			</form>
			<form method="POST" action="/actions/cross/receive?/save&source=invite" novalidate data-testid={mode + '-cross-invalid'} use:onlyEnhance={mode}>
				<label>Name <input name="name" value="Grace Hopper" /></label>
				<label>Email <input name="email" value="grace-at-example" /></label>
				<label>Biography <input name="biography" value="Keep this biography" /></label>
				<button type="submit">Reject invalid email on destination</button>
			</form>
			<form method="POST" action="/actions/cross/receive?/signIn&source=invite" data-testid={mode + '-cross-signin'} use:onlyEnhance={mode}>
				<input type="hidden" name="username" value="ada" />
				<button type="submit">Sign in from another page</button>
			</form>
			<form method="POST" action="/actions/cross/receive?/forbidden&source=invite" data-testid={mode + '-cross-forbidden'} use:onlyEnhance={mode}>
				<button type="submit">Attempt forbidden cross-page edit</button>
			</form>
			<form method="POST" action="/actions/cross/receive?/unavailable&source=invite" data-testid={mode + '-cross-unavailable'} use:onlyEnhance={mode}>
				<button type="submit">Trigger cross-page service failure</button>
			</form>
		</section>
	{/each}
</main>

<style>
	.card { max-width: 62rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
	section { border-top: 1px solid var(--line); margin-top: 0.8rem; }
	form { border-top: 1px solid var(--line); padding: 0.7rem 0; }
	label { display: grid; margin: 0.4rem 0; }
	input { padding: 0.5rem; font: inherit; }
	button { background: var(--green); color: white; border: 0; padding: 0.6rem; cursor: pointer; }
</style>
