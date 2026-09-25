<script lang="ts">
	import type { PageProps } from './$types';
	let { data }: PageProps = $props();
</script>

<svelte:head><title>Stored default profile | skgo example</title></svelte:head>
<main class="card">
	<p><a href="/actions/default">Default action</a></p>
	<h1 data-testid="default-stored-title">Stored default profile</h1>
	{#if data.saved}
		<p data-testid="default-stored-name">{data.profile.name}</p>
		<p data-testid="default-stored-email">{data.profile.email}</p>
		<p data-testid="default-stored-biography">{data.profile.biography}</p>
		<p data-testid="default-stored-state">{data.profile.state}</p>
	{:else}
		<p data-testid="default-stored-empty">No default profile has been saved.</p>
	{/if}
</main>

<style>
	.card { max-width: 45rem; margin: 1rem auto; padding: 1.2rem; border: 1px solid var(--line); background: white; }
</style>
