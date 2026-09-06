<script lang="ts">
	import { onMount } from 'svelte';

	import { browser } from '$app/environment';
	import { getSession, recordVisit } from './session.remote';

	let { children } = $props();
	let hydrated = $state(false);
	const session = getSession();

	$effect(() => {
		hydrated = browser;
	});

	// The junkyard app counted visits in `+layout.server.ts`, which kit runs on
	// every navigation and which may write cookies. skgo has no server load,
	// and a query may not write cookies, so the count becomes a command fired
	// after the page is already on screen. It cannot live in an `$effect`: the
	// refresh it triggers re-runs the effect, and the counter runs away.
	onMount(() => {
		void recordVisit().updates(getSession());
	});
</script>

<header>
	<nav>
		<a href="/">Home</a>
		<a href="/messages">Messages</a>
		<a href="/slow">Slow</a>
		<a href="/about">About</a>
		<a href="/prices">Prices</a>
		<a href="/whoami">Who am I</a>
		<a href="/login">Login</a>
	</nav>
	<p>
		<span data-testid="user">{(await session).user || 'anonymous'}</span>
		· visits <span data-testid="visits">{(await session).visits}</span>
		· hydrated <span data-testid="hydrated">{hydrated ? 'yes' : 'no'}</span>
	</p>
</header>

<main>
	{@render children()}
</main>
