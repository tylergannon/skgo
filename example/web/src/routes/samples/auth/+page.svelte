<script lang="ts">
  import { profile } from "./account.remote";
</script>

<svelte:head><title>Auth | skgo example</title></svelte:head>

<svelte:boundary>
  <p>Signed in as {(await profile()).user}.</p>
  {#snippet failed()}
    <p>Signed out.</p>
  {/snippet}
</svelte:boundary>

<form method="POST" action="?/signIn">
  <label>Name <input name="username" required /></label>
  <button>Sign in</button>
</form>

<form method="POST" action="?/publish">
  <button>Publish</button>
</form>
