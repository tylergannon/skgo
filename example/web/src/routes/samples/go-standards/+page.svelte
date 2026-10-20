<script lang="ts">
  import type { PageProps } from "./$types";
  import { sendNote } from "./note.remote";
  import { countTodos } from "./session.remote";

  let { form }: PageProps = $props();
</script>

<h1>Go and web standards</h1>

<svelte:boundary>
  <p>{await countTodos()} todos</p>
  {#snippet failed(error)}
    <p>{(error as Error).message}</p>
  {/snippet}
</svelte:boundary>

<form method="POST" action="?/upload" enctype="multipart/form-data">
  <label>File <input type="file" name="upload" required /></label>
  <label><input type="checkbox" name="tag" value="draft" /> Draft</label>
  <button>Upload</button>
</form>

{#if form}
  <p>{form.filename}: {form.bytes} bytes, SHA-256 {form.sha256}</p>
{/if}

<form enctype="multipart/form-data" {...sendNote}>
  <label>Title <input {...sendNote.fields.title.as("text")} /></label>
  {#each sendNote.fields.title.issues() ?? [] as issue (issue.message)}
    <p>{issue.message}</p>
  {/each}
  <label>Attachment <input {...sendNote.fields.attachment.as("file")} /></label>
  <button>Send</button>
</form>

{#if sendNote.result}
  <p>
    {sendNote.result.title}: {sendNote.result.file}, {sendNote.result.bytes} bytes
  </p>
{/if}
