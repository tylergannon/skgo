<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="message-title">Message #{data.message.id}</h1>
<p data-testid="message-author">{data.message.author}</p>
<p data-testid="message-text">{data.message.text}</p>
<a href="/messages">Back to messages</a>
