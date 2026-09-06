<script lang="ts">
	let { data } = $props();
</script>

<h1 data-testid="greeting">{data.greeting}</h1>
<p>There are <span data-testid="message-count">{data.messageCount}</span> messages.</p>
<img data-testid="logo" src="/logo.svg" alt="guestbook logo" width="64" height="64" />
