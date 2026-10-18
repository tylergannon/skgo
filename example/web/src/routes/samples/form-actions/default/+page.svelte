<script lang="ts">
  import type { PageProps } from "./$types";

  let { form }: PageProps = $props();
</script>

<form method="POST">
  <label>Name <input name="name" /></label>
  <label>Email <input name="email" /></label>
  <label>Biography <textarea name="biography"></textarea></label>
  <button>Save</button>
</form>

{#if form}<p>{form.receipt}</p>{/if}
