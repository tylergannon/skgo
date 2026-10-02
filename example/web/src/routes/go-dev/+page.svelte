<script lang="ts">
	import { enhance } from '$app/forms';
	import { dependencyRevision } from '#lib/go-dev-dependency.ts';
	import { getRevision } from './go-dev.remote';
	import type { PageProps } from './$types';

	let { data, form }: PageProps = $props();
	let endpoint = $state('No endpoint request yet.');

	function onlyEnhance(node: HTMLFormElement, mode: string) {
		if (mode === 'enhanced') return enhance(node);
	}

	async function callEndpoint() {
		const response = await fetch('/go-dev/endpoint');
		endpoint = JSON.stringify({
			status: response.status,
			body: await response.text(),
			revision: response.headers.get('X-Go-Revision')
		});
	}
</script>

<h1 data-testid="title">Go development updates</h1>
<p data-testid="go-dev-load">{data.revision}</p>
<p data-testid="go-dev-dependency">{dependencyRevision}</p>

<svelte:boundary>
	<p data-testid="go-dev-remote">{JSON.stringify(await getRevision())}</p>
	{#snippet failed(error)}
		<p data-testid="go-dev-failed">{(error as Error).message}</p>
	{/snippet}
</svelte:boundary>

{#if form && 'message' in form}
	<p data-testid="go-dev-action">{form.message}</p>
{/if}
{#each ['native', 'enhanced'] as mode}
	<form method="POST" action="?/revise" data-testid={mode + '-go-dev-form'} use:onlyEnhance={mode}>
		<button type="submit">{mode === 'native' ? 'Native revise' : 'Enhanced revise'}</button>
	</form>
{/each}

<button type="button" data-testid="go-dev-endpoint-button" onclick={callEndpoint}>Call endpoint</button>
<p data-testid="go-dev-endpoint">{endpoint}</p>
