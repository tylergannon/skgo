<script lang="ts">
  import { enhance } from "$app/forms";
  import { greet } from "./greeting.remote";
  import type { PageProps } from "./$types";

  let { form }: PageProps = $props();
</script>

<svelte:head><title>Migrating | skgo example</title></svelte:head>

<p>{await greet("Ada")}</p>

<form method="POST" action="?/save" use:enhance>
  <label>
    Email
    <input name="email" value={form && "email" in form ? form.email : ""} />
  </label>
  {#if form && "emailError" in form}
    <p>{form.emailError}</p>
  {/if}
  <button>Save</button>
</form>

{#if form && "receipt" in form}
  <p>{form.receipt}</p>
{/if}
