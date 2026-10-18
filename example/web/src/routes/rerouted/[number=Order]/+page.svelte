<script lang="ts">
 let {data}=$props();
</script>
<h1>Go reroute</h1>
<p data-testid="reroute-receipt">{data.receipt}</p>
<nav aria-label="Reroute examples">
 <a href="/go-alias/page?receipt=alias">Go alias</a>
 <a href="/rerouted/42?receipt=direct">Direct route</a>
</nav>
