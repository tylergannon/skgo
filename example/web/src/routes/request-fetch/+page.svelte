<script lang="ts">
  let { data } = $props();
</script>

<h1 data-testid="title">Request fetch</h1>
<p>
  A Go load fetched this page's data from a Go endpoint of the same app,
  in-process, with the visitor's own credentials.
</p>
<p data-testid="request-fetch-fact">{data.fact}</p>
<p data-testid="request-fetch-visitor">Fetched as {data.visitor}</p>
