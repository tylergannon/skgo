<script lang="ts">
	import { getBanner, getMessageCount, getMessages, postMessage } from './messages.remote';

	let text = $state('');
	let posting = $state(false);
	const count = getMessageCount();

	async function post(event: SubmitEvent) {
		event.preventDefault();
		posting = true;
		try {
			// skgo has no server-side `getMessages().refresh()`: the *client*
			// must name what the command invalidates.
			await postMessage(text).updates(getMessages());
			text = '';
		} finally {
			posting = false;
		}
	}
</script>

<h1>Messages</h1>
<p data-testid="banner">{(await getBanner()).text}</p>
<p>live count: <span data-testid="live-count">{await count}</span></p>

<form onsubmit={post}>
	<input data-testid="new-text" bind:value={text} placeholder="say something" />
	<button data-testid="post" disabled={posting}>Post</button>
</form>

<ul data-testid="messages">
	{#each await getMessages() as message (message.id)}
		<li data-testid="message">
			<a href="/messages/{message.id}">#{message.id}</a>
			<span data-testid="message-author">{message.author}</span>:
			<span data-testid="message-text">{message.text}</span>
			{#if message.attachment}
				<span data-testid="message-attachment">
					📎 {message.attachment.name} ({message.attachment.size} bytes)
				</span>
			{/if}
		</li>
	{/each}
</ul>
