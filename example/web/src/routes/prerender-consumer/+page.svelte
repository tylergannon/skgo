<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Prerender consumer</h1>
<p data-testid="prerender-consumer-receipt">{data.receipt}</p>
