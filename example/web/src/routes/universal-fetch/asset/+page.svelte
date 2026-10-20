<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Fetched assets</h1>
<p>
  This page's universal load fetched two assets it imports: one Vite emitted as
  a file, and one small enough that Vite inlined it as a data: URL.
</p>
<p data-testid="asset-emitted">
  {data.emitted.url}
  {data.emitted.status}
  {data.emitted.lamp}
  {data.emitted.bytes}
</p>
<p data-testid="asset-inlined">
  {data.inlined.url}
  {data.inlined.status}
  {data.inlined.lamp}
  {data.inlined.bytes}
</p>
