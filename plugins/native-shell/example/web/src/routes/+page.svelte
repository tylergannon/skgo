<script>
  import { readValue } from './value.remote';
</script>
<h1>Tiny native application</h1>
<p>The same Go value is available to generated Swift queries and commands.</p>
<button onclick={() => readValue()}>Read from Go</button>
