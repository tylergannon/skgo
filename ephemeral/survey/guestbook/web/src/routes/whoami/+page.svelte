<script lang="ts">
	let { data } = $props();
</script>

<h1>Who am I</h1>
<dl>
	<dt>url</dt>
	<dd data-testid="url">{data.url}</dd>
	<dt>client address</dt>
	<dd data-testid="client-address">{data.clientAddress}</dd>
	<dt>request id</dt>
	<dd data-testid="request-id">{data.requestId}</dd>
	<dt>user</dt>
	<dd data-testid="whoami-user">{data.user || 'anonymous'}</dd>
	<dt>host</dt>
	<dd data-testid="host">{data.host}</dd>
</dl>
