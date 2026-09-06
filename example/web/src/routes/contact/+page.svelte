<script lang="ts">
	import { getMessages, sendMessage } from './contact.remote';

	// The submission's outcome, so the page can say which of the two things
	// happened without the test having to infer it from what is missing.
	let outcome = $state<'idle' | 'sent' | 'rejected' | 'error'>('idle');

	const fields = sendMessage.fields;
</script>

<h1 data-testid="title">Contact</h1>

<form
	data-testid="contact-form"
	enctype="multipart/form-data"
	{...sendMessage.enhance(async (form) => {
		outcome = 'idle';
		try {
			// Single flight: the submission asks the server to refresh the
			// message list and send it back in the same response, so the page
			// updates without a second round trip.
			if (await form.submit().updates(getMessages())) {
				outcome = 'sent';
				form.element.reset();
			} else {
				// A rejected submission is deliberately left alone. Kit does
				// not reset an enhanced form, so everything the visitor typed
				// is still in the inputs.
				outcome = 'rejected';
			}
		} catch {
			outcome = 'error';
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

{#if outcome === 'sent' && sendMessage.result}
	<p data-testid="receipt">{sendMessage.result.summary}</p>
{/if}
{#if outcome === 'rejected'}
	<p data-testid="rejected">That message was not sent.</p>
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
