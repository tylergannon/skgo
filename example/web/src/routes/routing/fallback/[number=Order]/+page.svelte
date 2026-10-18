<script lang="ts">
  import RoutingReceipt from "#lib/RoutingReceipt.svelte";
  let { data } = $props();
</script>

<RoutingReceipt receipt={data.receipt} />
