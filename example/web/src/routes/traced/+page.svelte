<script lang="ts">
  import { receipt } from "./traced.remote";

  let { data } = $props();
</script>

<h1 data-testid="title">Traced load</h1>
<p data-testid="trace-id">{data.traceId}</p>

<svelte:boundary>
  {@const result = await receipt()}
  <p data-testid="receipt">{result.kind}</p>
  {#snippet pending()}
    <p>loading…</p>
  {/snippet}
</svelte:boundary>
