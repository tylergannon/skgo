<script lang="ts">
  import { listTasks, addTask } from './tasks.remote';
  let title = $state('');
  let message = $state('');
  const tasks = listTasks();

  async function submit(event: SubmitEvent) {
    event.preventDefault();
    try {
      await addTask({ title });
      title = '';
      message = '';
      await tasks.refresh();
    } catch (error) {
      message = error instanceof Error ? error.message : 'Could not add the task';
    }
  }
</script>

<svelte:head><title>Task list with MCP</title></svelte:head>
<main>
  <h1>Task list with MCP</h1>
  <p>The web page and MCP tools share this Go application’s in-memory task list.</p>
  <form onsubmit={submit}>
    <label>Task <input bind:value={title} required maxlength="120" /></label>
    <button type="submit">Add task</button>
  </form>
  {#if message}<p role="alert">{message}</p>{/if}
  <button onclick={() => tasks.refresh()}>Refresh tasks</button>
  {#await tasks}
    <p>Loading tasks…</p>
  {:then items}
    <ul>{#each items as task (task.id)}<li>{task.id}: {task.title}</li>{/each}</ul>
    {#if items.length === 0}<p>No tasks yet.</p>{/if}
  {:catch}<p role="alert">Could not load tasks.</p>{/await}
</main>

<style>
  main { max-width: 42rem; margin: 4rem auto; padding: 0 1.5rem; font-family: system-ui; }
  form { display: flex; gap: 1rem; margin: 2rem 0; }
  input, button { padding: .6rem; font: inherit; }
  li { margin: .75rem 0; }
</style>
