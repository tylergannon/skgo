<script lang="ts">
	import { getMessages, sendMessage } from './contact.remote';

	// Only the one thing the form instance cannot say for itself: an enhanced
	// submission whose request never arrived. Whether the submission succeeded
	// or was refused is read off `sendMessage` below, because a visitor with
	// scripting off never runs this file at all and the page has to say the
	// same thing either way.
	let unreachable = $state(false);

	const fields = sendMessage.fields;

	// A keyed instance of the very same form. `form.for(key)` gives each key
	// its own `fields`/`result`/`issues`, cached separately from the bare
	// `sendMessage` above — that is the whole reason a keyed instance exists,
	// so a page can run several independent copies of one form at once. Go
	// receives the key as `id` on the argument (contact.remote.go's
	// `Draft.ForKey`), which is where kit's own `form.for(key)` delivers it,
	// not as a control this markup puts on the page.
	const keyed = sendMessage.for('k1');
	const keyedFields = keyed.fields;
</script>

<h1 data-testid="title">Contact</h1>

<form
	data-testid="contact-form"
	enctype="multipart/form-data"
	{...sendMessage.enhance(async (form) => {
		unreachable = false;
		try {
			// Single flight: the submission asks the server to refresh the
			// message list and send it back in the same response, so the page
			// updates without a second round trip.
			if (await form.submit().updates(getMessages())) {
				form.element.reset();
			}
			// A rejected submission is deliberately left alone. Kit does not
			// reset an enhanced form, so everything the visitor typed is still
			// in the inputs.
		} catch {
			unreachable = true;
		}
	})}
>
	<label>
		Your name
		<input data-testid="field-from" {...fields.from.as('text')} />
	</label>
	{#each fields.from.issues() ?? [] as issue}
		<p class="issue" data-testid="issue-from">{issue.message}</p>
	{/each}

	<label>
		Email
		<input data-testid="field-email" {...fields.email.as('text')} />
	</label>
	{#each fields.email.issues() ?? [] as issue}
		<p class="issue" data-testid="issue-email">{issue.message}</p>
	{/each}

	<label>
		Message
		<textarea data-testid="field-body" {...fields.body.as('text')}></textarea>
	</label>
	{#each fields.body.issues() ?? [] as issue}
		<p class="issue" data-testid="issue-body">{issue.message}</p>
	{/each}

	<label>
		Attachment
		<input data-testid="field-attachment" {...fields.attachment.as('file')} />
	</label>

	<button data-testid="send" type="submit">Send</button>
</form>

<!--
	Neither of these reads a client-side variable. The receipt is shown
	whenever the form has a result and the refusal whenever it has issues,
	and the server puts both of those on the form instance — so the page says
	the same thing whether kit's client applied the submission or Go
	re-rendered the page around it, which is what a visitor with scripting off
	depends on.
-->
{#if sendMessage.result}
	<p data-testid="receipt">{sendMessage.result.summary}</p>
{/if}
{#if (sendMessage.fields.allIssues() ?? []).length > 0}
	<p data-testid="rejected">That message was not sent.</p>
{/if}
{#if unreachable}
	<p data-testid="unreachable">The server could not be reached.</p>
{/if}

<h2>Reply (keyed)</h2>

<form
	data-testid="keyed-contact-form"
	{...keyed.enhance(async (form) => {
		unreachable = false;
		try {
			if (await form.submit().updates(getMessages())) {
				form.element.reset();
			}
		} catch {
			unreachable = true;
		}
	})}
>
	<label>
		Your name
		<input data-testid="keyed-field-from" {...keyedFields.from.as('text')} />
	</label>
	{#each keyedFields.from.issues() ?? [] as issue}
		<p class="issue" data-testid="keyed-issue-from">{issue.message}</p>
	{/each}

	<label>
		Email
		<input data-testid="keyed-field-email" {...keyedFields.email.as('text')} />
	</label>
	{#each keyedFields.email.issues() ?? [] as issue}
		<p class="issue" data-testid="keyed-issue-email">{issue.message}</p>
	{/each}

	<label>
		Message
		<textarea data-testid="keyed-field-body" {...keyedFields.body.as('text')}></textarea>
	</label>
	{#each keyedFields.body.issues() ?? [] as issue}
		<p class="issue" data-testid="keyed-issue-body">{issue.message}</p>
	{/each}

	<button data-testid="keyed-send" type="submit">Send</button>
</form>

<!--
	`keyed` is its own instance, cached under its own key — its `result` and
	`fields` never reflect the bare `sendMessage` above and vice versa, which
	is the property this whole section exists to demonstrate.
-->
{#if keyed.result}
	<p data-testid="keyed-receipt">{keyed.result.summary}</p>
	<p data-testid="keyed-receipt-key">carried key: {keyed.result.key}</p>
{/if}
{#if (keyed.fields.allIssues() ?? []).length > 0}
	<p data-testid="keyed-rejected">That message was not sent.</p>
{/if}

<h2>Inbox</h2>
<svelte:boundary>
	<ul data-testid="messages">
		{#each await getMessages() as message (message.id)}
			<li data-testid="message">
				<span data-testid="message-from">{message.from}</span>
				<span data-testid="message-body">{message.body}</span>
				{#if message.attachment}
					<span data-testid="message-attachment">{message.attachment}</span>
					<span data-testid="message-attachment-bytes">{message.attachmentBytes}</span>
					<span data-testid="message-attachment-digest">{message.attachmentDigest}</span>
				{/if}
			</li>
		{/each}
	</ul>
	{#snippet pending()}
		<p data-testid="messages-pending">loading…</p>
	{/snippet}
	{#snippet failed(error)}
		<p data-testid="messages-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

<style>
	.issue {
		color: #b00020;
		margin: 0.125rem 0 0.5rem;
	}

	label {
		display: block;
		margin-top: 0.75rem;
	}
</style>
