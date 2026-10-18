<script lang="ts">
  import { double, shout, signUp } from "./testing.remote";

  let doubled = $state<number>();
</script>

<svelte:head><title>Testing | skgo example</title></svelte:head>

<p>{await shout("hello")}</p>

<button onclick={async () => (doubled = await double(21))}>Double 21</button>
{#if doubled !== undefined}
  <p>{doubled}</p>
{/if}

<form {...signUp}>
  <label>Email <input {...signUp.fields.email.as("email")} /></label>
  {#each signUp.fields.email.issues() ?? [] as issue (issue.message)}
    <p>{issue.message}</p>
  {/each}
  <button>Sign up</button>
</form>
{#if signUp.result}
  <p>Welcome, {signUp.result.email}</p>
{/if}
