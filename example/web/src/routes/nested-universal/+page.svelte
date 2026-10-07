<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Nested universal fetch</h1>
<p>This page's universal load fetched another rendered page of the app.</p>
<p data-testid="nested-fact">{data.fact}</p>
<p data-testid="nested-visitor">{data.visitor}</p>
<p data-testid="nested-status">{data.status}</p>
