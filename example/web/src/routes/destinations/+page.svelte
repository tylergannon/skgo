<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Destinations</h1>
<p>One Go load, one fetch of each kind of destination the app has.</p>
<ul>
  {#each data.answers as answer (answer.path)}
    <li data-path={answer.path}>{answer.status} {answer.contentType}</li>
  {/each}
</ul>
