<script>
  import { receipt } from '#lib/data/data.remote.js';

  /** @type {import('./$types').PageProps} */
  let { data, form } = $props();
  let remote = $state('Not requested');

  async function requestReceipt() {
    const value = await receipt('browser');
    remote = `${value.message} (${value.count})`;
  }
</script>

<h1>Go receipt</h1>
<p id="load">{data.message} ({data.count})</p>
<button onclick={requestReceipt}>Request Go remote</button>
<p id="remote">{remote}</p>
<form method="POST" action="?/save">
  <button>Save with Go</button>
</form>
{#if form}<p id="action">{form.message}</p>{/if}
<a href="/">Home</a>
