<script lang="ts">
  import AddNote from "./AddNote.svelte";
  import EditNote from "./EditNote.svelte";
  import { watchCount } from "./count.remote";
  import { getNotes } from "./notes.remote";
</script>

<p>Notes: {await watchCount()}</p>

<ul>
  {#each await getNotes() as note (note.id)}
    <li><EditNote id={note.id} /></li>
  {/each}
</ul>

<AddNote />
