<script lang="ts">
  import { getNote } from "./notes.remote";
  import { renameNote } from "./rename.remote";

  let { id }: { id: string } = $props();
  let text = $state("");
</script>

<p>{(await getNote(id)).text}</p>

<form
  onsubmit={async (event) => {
    event.preventDefault();
    await renameNote({ id, text });
    text = "";
  }}
>
  <input bind:value={text} />
  <button>Rename</button>
</form>
