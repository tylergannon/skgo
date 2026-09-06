<script lang="ts">
	import type { PageProps } from './$types';
	let { params }: PageProps = $props();
</script>

<h1 data-testid="title">Item {params.id}</h1>
