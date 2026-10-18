<script lang="ts">
  let { receipt }: { receipt: string } = $props();
</script>

<h1>Go route selection</h1>
<p data-testid="routing-receipt">{receipt}</p>
<nav aria-label="Routing examples">
  <a href="/routing/optional">Omitted order</a>
  <a href="/routing/optional/42">Optional order 42</a>
  <a href="/routing/fallback/7">Matched order 7</a>
  <a href="/routing/fallback/no-order">Rejected order fallback</a>
  <a href="/routing/slash/always/">Always slash</a>
  <a href="/routing/slash/never">Never slash</a>
  <a href="/routing/slash/ignore/">Ignore slash</a>
</nav>
