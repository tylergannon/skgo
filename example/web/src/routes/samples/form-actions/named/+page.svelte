<script lang="ts">
  import type { PageProps } from "./$types";

  let { form }: PageProps = $props();
</script>

<form method="POST" action="?/save">
  <label>Name <input name="name" /></label>
  <button>Save</button>
  <button formaction="?/archive">Archive</button>
</form>

{#if form}<p>{form.receipt}</p>{/if}
