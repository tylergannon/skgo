<script lang="ts">
  import { enhance } from "$app/forms";
  import type { PageProps } from "./$types";

  let { form }: PageProps = $props();
</script>

<form method="POST" action="?/save" use:enhance>
  <label>Name <input name="name" /></label>
  <button>Save</button>
</form>

{#if form}<p>{form.receipt}</p>{/if}
