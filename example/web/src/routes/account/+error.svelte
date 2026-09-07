<script lang="ts">
	import { page } from '$app/state';
</script>

<!--
	The account section's own error page. It exists so that a failure under
	/account renders *inside* the account layout: kit walks outward from the
	node that failed to the nearest `+error.svelte` and keeps every layout down
	to the depth that declared it (`runtime/error-chain.js`). Without this file
	the root error page answers instead, and the layout above it is dropped.
-->
<h1 data-testid="title">Account error {page.status}</h1>
<p data-testid="error-message">{page.error?.message}</p>
