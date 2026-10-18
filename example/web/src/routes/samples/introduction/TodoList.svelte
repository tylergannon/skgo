<script lang="ts">
  import { getTodos } from "./todos.remote";
</script>

<ul>
  {#each await getTodos() as todo (todo.id)}
    <li>{todo.text}</li>
  {/each}
</ul>
