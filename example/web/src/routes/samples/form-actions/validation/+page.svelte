<script lang="ts">
  import type { PageProps } from "./$types";

  let { data, form }: PageProps = $props();
  let rejected = $derived(form && "emailError" in form ? form : null);
</script>

<form method="POST" action="?/save">
  <label>
    Name
    <input name="name" value={rejected ? rejected.name : data.profile.name} />
  </label>
  <label>
    Email
    <input
      name="email"
      value={rejected ? rejected.email : data.profile.email}
    />
  </label>
  {#if rejected}<p class="error">{rejected.emailError}</p>{/if}
  <button>Save</button>
</form>
