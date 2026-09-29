<script lang="ts">
	import { invalidate, refreshAll } from '$app/navigation';

	let { data } = $props();
	let localCount = $state(0);
</script>

<h1 data-testid="title">Load reruns</h1>
<p data-testid="rerun-slug">{data.slug}</p>
<p data-testid="rerun-filter">{data.filter}</p>
<p data-testid="rerun-serial">{data.serial}</p>
<p data-testid="rerun-local-count">{localCount}</p>
<button data-testid="rerun-local-increment" onclick={() => localCount++}>Increment local count</button>
<button data-testid="rerun-refresh-all" onclick={() => refreshAll()}>Refresh all loads</button>
<button
	data-testid="rerun-invalidate-predicate"
	onclick={() => invalidate((url) => url.protocol === 'app:' && url.pathname === 'reruns')}
>Invalidate matching dependency</button>
<nav>
	<a href="/account/reruns/two?x=1">Slug two</a>
	<a href="/account/reruns/two?x=2">Tracked x</a>
	<a href="/account/reruns/two?x=2&y=1">Untracked y</a>
</nav>
