<script lang="ts">
	import { page } from '$app/state';
</script>

<h1 data-testid="title">Error {page.status}</h1>
<p data-testid="error-message">{page.error?.message}</p>
{#if page.error?.supportId}
	<!--
		supportId is not part of kit's App.Error — it is whatever the app's
		own handleError hook (example.HandleError, in example/server.go)
		chose to add. This is the proof that the hook ran and this page is
		rendering what it returned, not kit's own default.
	-->
	<p data-testid="error-support-id">{page.error.supportId}</p>
{/if}
