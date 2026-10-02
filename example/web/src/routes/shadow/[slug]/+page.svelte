<script lang="ts">
	import { page } from '$app/state';
</script>

<h1 data-testid="title">Prerendered shadow</h1>
<p data-testid="shadow-slug">prerendered:{page.params.slug}</p>
