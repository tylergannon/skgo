<script lang="ts">
	/**
	 * A component that can only fail on the server.
	 *
	 * `document` exists in a browser and does not exist in the rendering engine,
	 * so this throw happens while Go builds the document and never once the page
	 * is live. It is the failure dev could not have before Go rendered in dev:
	 * kit's own dev server never ran this component on the server, so a
	 * component that breaks only there broke for the first time in production.
	 *
	 * What a visitor gets is the app's error page, at the status handleError
	 * chose — not a blank document, and not a page that quietly leaves this out.
	 */
	function failOnTheServer(): never {
		throw new Error('skgo: /error/server-only cannot render on the server');
	}
</script>

{#if typeof document === 'undefined'}
	{failOnTheServer()}
{/if}

<p data-testid="server-only">this page only renders in the browser</p>
