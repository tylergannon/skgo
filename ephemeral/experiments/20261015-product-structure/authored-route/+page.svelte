<script lang="ts">
  import { readProbe } from "./probe.remote";
</script>
<h1>STRUCTURAL_7719</h1>
<svelte:boundary>
  <p>REMOTE_{await readProbe()}</p>
</svelte:boundary>
<a href="/items/42">Existing item after node shift</a>
