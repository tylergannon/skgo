<script lang="ts">
  import { page } from '$app/state';
  import Failure from './Failure.svelte';
</script>

<h1 data-testid="component-boundary-title">Component boundary policy</h1>
<svelte:boundary>
  <Failure kind={page.url.searchParams.get('kind') ?? 'ok'} />
  {#snippet failed(error)}
    <p data-testid="component-boundary-message">{(error as App.Error).message}</p>
    <p data-testid="component-boundary-support">{(error as App.Error).supportId ?? 'no hook field'}</p>
    <p data-testid="component-boundary-original">{(error as { original?: string }).original ?? 'no original field'}</p>
  {/snippet}
</svelte:boundary>
