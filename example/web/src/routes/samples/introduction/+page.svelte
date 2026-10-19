<script lang="ts">
  import { addTodo, getTodos } from "./todos.remote";
  import TodoList from "./TodoList.svelte";

  let text = $state("");
</script>

<h1>Todos</h1>

<svelte:boundary>
  <TodoList />
  {#snippet failed(error)}
    <p>{(error as Error).message}</p>
  {/snippet}
</svelte:boundary>

<form
  onsubmit={async (event) => {
    event.preventDefault();
    await addTodo(text).updates(getTodos());
    text = "";
  }}
>
  <input bind:value={text} />
  <button>Add</button>
</form>
