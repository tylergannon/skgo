<script lang="ts">
  import { page } from "$app/state";
  let { data } = $props();
</script>

<h1 data-testid="title">Server fetch</h1>
<p>
  This universal load calls the Go todos API. Its first response is included in
  the document and reused during hydration. Sign in above and reload to see
  private todos; the request with credentials omitted stays public.
</p>
<p data-testid="fetch-hook">Fetch hook: {data.hook}</p>
<h2>Your todos</h2>
<ul data-testid="fetch-todos">
  {#each data.todos as todo (todo.id)}
    <li data-id={todo.id}>{todo.text}</li>
  {/each}
</ul>
<h2>With credentials omitted</h2>
<ul data-testid="fetch-public-todos">
  {#each data.publicTodos as todo (todo.id)}
    <li data-id={todo.id}>{todo.text}</li>
  {/each}
</ul>
<a
  href={`/fetch?refresh=${Number(page.url.searchParams.get("refresh") ?? 0) + 1}`}
  data-testid="fetch-navigate">Refresh through client navigation</a
>
