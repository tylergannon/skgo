<script lang="ts">
  import { addNote } from "./add.remote";
  import { getNotes } from "./notes.remote";

  let text = $state("");
</script>

<form
  onsubmit={async (event) => {
    event.preventDefault();
    await addNote(text).updates(getNotes());
    text = "";
  }}
>
  <input bind:value={text} />
  <button>Add</button>
</form>
