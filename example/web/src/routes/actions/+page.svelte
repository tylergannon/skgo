<script lang="ts">
	import { enhance } from '$app/forms';
	import { page } from '$app/state';
	import { sendRemoteNote } from './coexist.remote';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let clientInteraction = $state(false);
	let endpointAnswer = $state('No endpoint POST yet.');
	const remoteFields = sendRemoteNote.fields;
	let rejected = $derived(form && 'emailError' in form ? form : null);
	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}
</script>

<svelte:head><title>Actions | skgo example</title></svelte:head>

<div class="actions-page">
	<header>
		<p class="eyebrow">Page form actions</p>
		<h1 data-testid="title">Actions</h1>
		<p>Save a profile, reject an invalid edit, archive without a receipt, sign in, or try an error. The same editor works as a native form or with Kit enhancement.</p>
		<p><a href="/actions/default">Default action without a load</a> · <a href="/actions/profiles/ada">Two profile editor</a> · <a href="/actions/cross/send">Submit to another page</a> · <a href="/actions/options/no-client">No client JavaScript</a> · <a href="/actions/options/no-ssr">Client rendering</a> · <a href="#shared-route">Shared route and remote form</a></p>
	</header>

	<div class="columns">
		<section class="card" aria-labelledby="editor-title">
			<h2 id="editor-title">Profile editor</h2>
			<p>Choose a native document POST or Kit enhancement. Try each outcome.</p>
			<noscript><p data-testid="actions-noscript">JavaScript is disabled. Native actions still work.</p></noscript>
			{#each ['native', 'enhanced'] as mode}
				<form data-testid={mode + '-save-form'} method="POST" action="?/save" novalidate use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native save' : 'Enhanced save'}</h3>
					<label>Name <input name="name" value={rejected ? rejected.name : data.profile.name} required /></label>
					<label>Email <input name="email" type="email" value={rejected ? rejected.email : data.profile.email} required /></label>
					{#if rejected}<p class="field-error" data-testid={mode + '-email-error'}>{rejected.emailError}</p>{/if}
					<label>Biography <textarea name="biography" required>{rejected ? rejected.biography : data.profile.biography}</textarea></label>
					<button type="submit">Save profile</button>
				</form>
				<form data-testid={mode + '-upload-form'} method="POST" action="?/upload" enctype="multipart/form-data" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native upload' : 'Enhanced upload'}</h3>
					<label>Poem <input type="file" name="upload" required /></label>
					<label class="check"><input type="checkbox" name="interest" value="math" /> Math</label>
					<label class="check"><input type="checkbox" name="interest" value="computing" /> Computing</label>
					<button type="submit" name="uploadButton" value="send-poem">Send poem</button>
				</form>
				<form data-testid={mode + '-encoding-form'} method="POST" action="?/inspect" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native submitter encoding' : 'Enhanced submitter encoding'}</h3>
					<input type="hidden" name="interest" value="math" />
					<input type="hidden" name="interest" value="computing" />
					<button type="submit" name="encodingButton" value="standard">Send urlencoded</button>
					<button type="submit" name="encodingButton" value="multipart" formenctype="multipart/form-data">Send multipart override</button>
				</form>
				<form data-testid={mode + '-archive-form'} method="POST" action="?/archive" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native archive' : 'Enhanced archive'}</h3>
					<button type="submit">Archive profile</button>
				</form>
				<form data-testid={mode + '-signin-form'} method="POST" action="?/signIn" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native sign-in' : 'Enhanced sign-in'}</h3>
					<input type="hidden" name="username" value="ada" />
					<button type="submit">Sign in as ada</button>
				</form>
				<form data-testid={mode + '-forbidden-form'} method="POST" action="?/forbidden" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native permission error' : 'Enhanced permission error'}</h3>
					<button type="submit">Attempt forbidden edit</button>
				</form>
				<form data-testid={mode + '-unavailable-form'} method="POST" action="?/unavailable" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native unexpected error' : 'Enhanced unexpected error'}</h3>
					<button type="submit">Trigger demo service failure</button>
				</form>
				<form data-testid={mode + '-choice-form'} method="POST" novalidate use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native chosen action' : 'Enhanced chosen action'}</h3>
					<label>Name <input name="name" value={data.profile.name} required /></label>
					<label>Email <input name="email" type="email" value={data.profile.email} required /></label>
					<label>Biography <textarea name="biography" required>{data.profile.biography}</textarea></label>
					<button type="submit" name="choice" value="save" formaction="?/save">Save selected profile</button>
					<button type="submit" name="choice" value="archive" formaction="?/archive">Archive selected profile</button>
				</form>
				<form data-testid={mode + '-cookie-form'} method="POST" action="?/remember" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native immediate cookie' : 'Enhanced immediate cookie'}</h3>
					<button type="submit">Remember violet-42</button>
				</form>
				<form data-testid={mode + '-hook-redirect-form'} method="POST" action="?hook=sign-in&/save" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native hook redirect' : 'Enhanced hook redirect'}</h3>
					<input type="hidden" name="name" value="Grace Hopper" /><input type="hidden" name="email" value="grace@example.test" /><input type="hidden" name="biography" value="Compiler pioneer" />
					<button type="submit">Try guarded sign-in</button>
				</form>
				<form data-testid={mode + '-hook-error-form'} method="POST" action="?hook=forbidden&/save" use:onlyEnhance={mode}>
					<h3>{mode === 'native' ? 'Native hook refusal' : 'Enhanced hook refusal'}</h3>
					<input type="hidden" name="name" value="Grace Hopper" /><input type="hidden" name="email" value="grace@example.test" /><input type="hidden" name="biography" value="Compiler pioneer" />
					<button type="submit">Try guarded edit</button>
				</form>
			{/each}
		</section>

		<div class="results">
			<section class="card" aria-labelledby="saved-title">
				<h2 id="saved-title">Saved profile</h2>
				<p data-testid="saved-name">{data.profile.name}</p>
				<p data-testid="saved-email">{data.profile.email}</p>
				<p data-testid="saved-biography">{data.profile.biography}</p>
				<p data-testid="saved-state">{data.profile.state}</p>
			</section>
			<section class="card" aria-labelledby="receipt-title">
				<h2 id="receipt-title">Action receipt</h2>
				<p data-testid="page-status">Page status {page.status}</p>
				{#if form && 'receipt' in form && form.price}
					<p data-testid="action-status">Status {page.status}</p>
					<p data-testid="action-receipt">{form.receipt}</p>
					<p data-testid="action-money">{form.price.format()}</p>
				{:else if form && 'filename' in form}
					<p data-testid="upload-name">{form.filename}</p>
					<p data-testid="upload-bytes">{form.bytes} bytes</p>
					<p data-testid="upload-hash">SHA-256 prefix {form.sha256}</p>
					<p data-testid="upload-interests">{form.interests?.join(', ')}</p>
					<p data-testid="upload-submitter">uploadButton={form.submitter}</p>
					<p data-testid="action-money">{form.price?.format()}</p>
				{:else if form && 'encoding' in form}
					<p data-testid="encoding-interests">{form.interests?.join(', ')}</p>
					<p data-testid="encoding-submitter">encodingButton={form.submitter}</p>
					<p data-testid="encoding-type">{form.encoding}</p>
					<p data-testid="action-money">{form.price?.format()}</p>
				{:else if form && 'receipt' in form}
					<p data-testid="action-receipt">{form.receipt}</p>
				{:else if rejected}
					<p data-testid="action-status">Status {page.status}</p>
					<p data-testid="validation-summary">Edit rejected; correct the email and try again.</p>
				{:else}
					<p data-testid="no-receipt">No action receipt is present.</p>
				{/if}
			</section>
			<section class="card" aria-labelledby="cookie-title">
				<h2 id="cookie-title">Action cookie read by Go load</h2>
				<p data-testid="action-cookie-value">{data.actionCookie || 'No action cookie yet'}</p>
			</section>
			<section class="card" id="shared-route" aria-labelledby="shared-title">
				<h2 id="shared-title">Shared route and remote form</h2>
				<p>The classic save above, the endpoint POST, and this remote form have separate Go handlers.</p>
				<button data-testid="endpoint-post" type="button" onclick={async () => {
					const response = await fetch('/actions', { method: 'POST', headers: { Accept: 'application/json', 'Content-Type': 'application/json' }, body: '{}' });
					endpointAnswer = (await response.json()).answer;
				}}>Ask the sibling endpoint</button>
				<p data-testid="endpoint-answer">{endpointAnswer}</p>
				{#if form && 'receipt' in form}<p>Classic result: {form.receipt}</p>{/if}
				<noscript><p data-testid="coexist-noscript">JavaScript is disabled. Both native forms still work.</p></noscript>
				<form data-testid="native-remote-note" method="POST" action={sendRemoteNote.action}>
					<h3>Native remote form</h3>
					<label>Name <input {...remoteFields.name.as('text')} value="Grace Hopper" /></label>
					<button type="submit">Send native remote note</button>
				</form>
				<form data-testid="enhanced-remote-note" {...sendRemoteNote.enhance(async (submission) => { await submission.submit(); })}>
					<h3>Enhanced remote form</h3>
					<label>Name <input {...remoteFields.name.as('text')} value="Grace Hopper" /></label>
					<button type="submit">Send enhanced remote note</button>
				</form>
				{#if sendRemoteNote.result}<p data-testid="remote-note-receipt">{sendRemoteNote.result.message}</p>{/if}
			</section>
			<button data-testid="client-interaction" type="button" onclick={() => (clientInteraction = true)}>Check client interaction</button>
			{#if clientInteraction}<p data-testid="client-interaction-done">Client interaction complete</p>{/if}
		</div>
	</div>
</div>

<style>
	.actions-page { max-width: 70rem; margin: 0 auto; }
	header { padding: 0.7rem 1rem; border: 1px solid var(--line); background: linear-gradient(135deg, #eff9f4, #fff6ee); }
	header h1 { margin: 0; font-size: clamp(2rem, 4vw, 2.5rem); letter-spacing: -0.06em; }
	header p { margin: 0.35rem 0; }
	header p:last-child { margin-bottom: 0; }
	.eyebrow { color: var(--green); font-size: 0.75rem; font-weight: 800; letter-spacing: 0.12em; text-transform: uppercase; }
	.columns { display: grid; grid-template-columns: 1.3fr 1fr; gap: 0.7rem; margin-top: 0.7rem; }
	.card { padding: 0.75rem 0.9rem; border: 1px solid var(--line); background: white; }
	.card h2 { margin: 0 0 0.45rem; }
	form { border-top: 1px solid var(--line); padding-top: 0.55rem; margin-top: 0.65rem; }
	form h3 { margin: 0 0 0.4rem; }
	label { display: grid; gap: 0.2rem; margin: 0.35rem 0; font-weight: 600; }
	label.check { display: flex; align-items: center; gap: 0.5rem; }
	label.check input { width: auto; }
	input, textarea { width: 100%; padding: 0.4rem 0.55rem; border: 1px solid var(--line); font: inherit; }
	textarea { min-height: 3rem; }
	button { padding: 0.45rem 0.75rem; border: 0; background: var(--green); color: white; font: inherit; cursor: pointer; }
	.results { display: grid; align-content: start; gap: 0.7rem; }
	.results p { margin: 0.35rem 0; }
	.field-error { color: #a32020; font-weight: 700; margin: 0.2rem 0 0.7rem; }
	@media (max-width: 780px) { .columns { grid-template-columns: 1fr; } }
</style>
