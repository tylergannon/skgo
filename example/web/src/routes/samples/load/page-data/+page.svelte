<script lang="ts">
  import type { PageProps } from "./$types";

  let { data }: PageProps = $props();
</script>

<p>As of {data.asOf}</p>
<ul>
  {#each data.invoices as invoice (invoice.number)}
    <li>{invoice.number} due {invoice.dueDate}</li>
  {/each}
</ul>
