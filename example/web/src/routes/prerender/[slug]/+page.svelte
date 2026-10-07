<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Prerendered {data.slug}</h1>
<p data-testid="entry-receipt">{data.receipt}</p>
