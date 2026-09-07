<script lang="ts">
	import { whereTo } from './redirect.remote';
</script>

<h1 data-testid="title">Where to</h1>

<!--
	Nothing here reaches the browser. The query redirects instead of answering,
	and a redirect thrown during a render is the whole document's answer in
	kit — a bare 3xx with a Location and no body.
-->
<p data-testid="where">{(await whereTo()).where}</p>
