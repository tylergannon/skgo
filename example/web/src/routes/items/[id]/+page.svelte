<script lang="ts">
  import ContactForms from "../../contact/ContactForms.svelte";
  import type { PageProps } from "./$types";
  import { getItem } from "./item.remote";

  let { data, params }: PageProps = $props();
</script>

<h1 data-testid="title">Item {params.id}</h1>
<p data-testid="load-item-name">{data.loadedItem.name}</p>

<svelte:boundary>
  {#if params.id}
    {@const item = await getItem(params.id)}
    <p data-testid="item-name">{item.name}</p>
    <p data-testid="colocated">{item.colocated}</p>
  {/if}
  {#snippet failed(error)}
    <p data-testid="item-failed">{(error as Error).message}</p>
  {/snippet}
</svelte:boundary>

<ContactForms showInbox={false} />
