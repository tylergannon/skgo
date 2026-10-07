<script lang="ts">
  import { enhance } from "$app/forms";
  import type { PageProps } from "./$types";
  let { data, form }: PageProps = $props();
  function onlyEnhance(node: HTMLFormElement, mode: string) {
    if (mode === "enhanced") return enhance(node);
  }
</script>

<svelte:head><title>Middleware | skgo example</title></svelte:head>
<main class="card">
  <h1 data-testid="title">Middleware</h1>
  <p>Go middleware authenticated this visit before the load below ran.</p>
  <p data-testid="mw-token">{data.token}</p>
  <p data-testid="mw-route">{data.route}</p>
  <p data-testid="mw-cookie">{data.cookie}</p>
  <p><a data-testid="mw-alpha" href="/middleware/alpha">Alpha</a></p>
  <noscript
    ><p data-testid="mw-noscript">
      JavaScript is disabled. The form still works.
    </p></noscript
  >
  {#if form && "receipt" in form}
    <h2 data-testid="mw-receipt">{form.receipt}</h2>
  {/if}
  {#each ["native", "enhanced"] as mode}
    <form
      method="POST"
      data-testid={mode + "-note-form"}
      use:onlyEnhance={mode}
    >
      <h2>{mode === "native" ? "Native submission" : "Enhanced submission"}</h2>
      <label>Note <input name="note" value="remember-this" /></label>
      <button type="submit">Save note</button>
    </form>
  {/each}
</main>

<style>
  .card {
    max-width: 45rem;
    margin: 1rem auto;
    padding: 1.2rem;
    border: 1px solid var(--line);
    background: white;
  }
  form {
    border-top: 1px solid var(--line);
    padding: 0.7rem 0;
  }
  label {
    display: grid;
    margin: 0.4rem 0;
  }
  input {
    padding: 0.5rem;
    font: inherit;
  }
  button {
    background: var(--green);
    color: white;
    border: 0;
    padding: 0.6rem;
    cursor: pointer;
  }
</style>
