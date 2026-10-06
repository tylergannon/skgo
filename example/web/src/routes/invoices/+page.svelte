<script lang="ts">
  let { data } = $props();
</script>

<h1>Invoices</h1>
<nav>
  <a href="/invoices?overdue=1">Overdue</a>
  <a href="/invoices">All invoices</a>
</nav>
<p data-testid="invoice-filter">
  {data.overdueOnly ? "Overdue" : "All invoices"}
</p>
<ul data-testid="invoices">
  {#each data.invoices as invoice (invoice.number)}
    <li data-testid="invoice">
      {invoice.number}: {invoice.dueDate} — {invoice.outstandingCents} cents{invoice.overdue
        ? " (overdue)"
        : ""}
    </li>
  {/each}
</ul>
<p data-testid="invoice-total">Total: {data.totalCents} cents</p>
