<script lang="ts">
	import Greeting from '#lib/Greeting.svelte';
	import { getSite } from '#lib/site-api.ts';

	/**
	 * What this app is for.
	 *
	 * Every capability skgo has is a page in this app, and a visitor who has
	 * never seen skgo should be able to start here and reach all of them. Each
	 * entry says what to look for once it is open, because a page that renders
	 * is not the same as a page that proves anything: "the list is there" and
	 * "the list was answered by a Go function while Go rendered the document"
	 * look identical until somebody says which one they meant.
	 */
	const capabilities: Array<{ name: string; href: string; look: string; reload?: boolean }> = [
		{
			name: 'Pages rendered in the Go process',
			href: '/items/42',
			look: 'The name "Widget 42" is in the bytes Go sent, put there by a Go function called while the document was being built.'
		},
		{
			name: 'Route parameters',
			href: '/items/7',
			look: 'The id in the URL is the argument the Go function was given: /items/7 is a different widget from /items/42.'
		},
		{
			name: 'Rest parameters',
			href: '/docs/guide/getting-started',
			look: 'The title is the path after /docs, and the depth is how many segments it had.'
		},
		{
			name: 'Remote functions: queries, commands and forms',
			href: '/todos',
			look: 'Adding, completing and deleting a todo are Go functions; the count above the list is a live query that follows along.'
		},
		{
			name: 'Refreshing a query after a command',
			href: '/todos/gate',
			look: 'The gate opens and closes from Go, and the query behind it re-runs because the command said which query to refresh.'
		},
		{
			name: 'One query, one answer per argument',
			href: '/todos/pair',
			look: 'Both panels call the same Go function, told apart only by the id they pass. Retitle p1 and ask for p1 back: p2 does not move, because each argument has its own cached answer.'
		},
		{
			name: 'A query that goes on answering',
			href: '/live',
			look: 'The number was already in the document before any script ran, and Go pushes every later value down the same open stream.'
		},
		{
			name: 'Many calls answered by one',
			href: '/batch',
			look: 'Four rows each asked for one symbol on their own. One Go function was called once holding all four, and every row says how many were in the call that answered it.'
		},
		{
			name: 'Server loads, section-wide',
			href: '/account',
			look: 'The layout and the page were both loaded in Go. Move between Overview and Orders: the serial does not change, because kit did not re-run the layout.'
		},
		{
			name: 'A nested error page',
			href: '/account/statement',
			look: 'A load refuses with 402, and the error page renders inside the account layout rather than replacing it.'
		},
		{
			name: 'Values a load promises but does not have yet',
			href: '/stream',
			look: 'The total is in the document; the rows arrive after it, in the order they settle rather than the order they were asked for.'
		},
		{
			name: 'Custom types that keep their methods',
			href: '/pricing',
			look: 'Every price is a method call on a Go type. The wire carried whole cents — "$7.50" only exists because Money.format() ran.'
		},
		{
			name: 'A form that works with JavaScript switched off',
			href: '/contact',
			look: 'The form posts to Go and the page comes back with the answer in it. Disable JavaScript and it still works.'
		},
		{
			name: 'HTTP endpoints written in Go',
			href: '/api',
			look: 'Kit 3 identifies the route through $app/manifest and sends its new HTTP QUERY method to an ordinary net/http function.'
		},
		{
			name: 'Links and asset URLs worked out while the page renders',
			href: '/render-paths',
			look: "Every href on the page was computed during the render by kit's own $app/paths, and match() asked Go's route table which route /items/77 belongs to."
		},
		{
			name: 'A page with no client-side JavaScript',
			href: '/plain',
			look: 'csr = false, so the document carries no script at all. Everything on it had to be rendered on the server to be there.'
		},
		{
			name: 'A page rendered only in the browser',
			href: '/spa',
			look: "ssr = false, so Go answers with kit's SPA fallback and the page appears once the client has booted."
		},
		{
			name: 'An empty list is still a list',
			href: '/empty',
			look: "Go's nil slice reaches the browser as [], so the page renders an empty list instead of failing on null."
		},
		{
			name: "What the renderer writes reaches Go's log",
			href: '/console',
			look: 'The page calls console.error while it renders, and the line comes out of the Go process.'
		},
		{
			name: 'A load that refuses',
			href: '/error/expected',
			look: 'The load throws 418 and the visitor gets an error page at that status, rendered before it was sent.'
		},
		{
			name: 'A failure the page catches itself',
			href: '/error/boundary',
			look: 'The query refuses, the boundary beside it renders what it said, and the rest of the page is untouched — at the status the error carried.'
		},
		{
			name: 'An error that is a bug says nothing about itself',
			href: '/error/unexpected',
			look: 'The Go load fails with a database connection string in its message. The visitor gets the message and support id chosen by handleError, and no part of the original leaves the process.'
		},
		{
			name: 'A failure no error page can catch',
			href: '/?boom=root-layout',
			// kit's own attribute for a link that must be a document request.
			// The page it promises only exists in a document: a client-side
			// navigation would fetch `__data.json`, find the root layout's
			// error in it and render something in the app — which is the one
			// thing this entry says cannot happen.
			reload: true,
			look: "The root layout's load refuses. Nothing above it can render an error page, so kit's static error.html answers instead — no app, no script, nothing that tries again."
		}
	];
</script>

<header class="showcase-hero">
	<p class="eyebrow">The working reference</p>
	<h1 data-testid="title">Home</h1>
	<Greeting name="skgo" />
	<p class="intro">
		Every server answer in this ordinary SvelteKit app comes from Go. Pick a capability below,
		try it, then follow the colocated <code>.go</code> and <code>.svelte</code> files.
	</p>

	<svelte:boundary>
		{@const site = await getSite()}
		<div class="provenance">
			<p data-testid="site-name">{site.name}</p>
			<p data-testid="colocated">{site.colocated}</p>
		</div>
		{#snippet failed(error)}
			<p data-testid="site-failed">{(error as Error).message}</p>
		{/snippet}
	</svelte:boundary>
</header>

<div class="capability-heading">
	<div>
		<p class="eyebrow">The complete tour</p>
		<h2>What this app demonstrates</h2>
	</div>
	<a href="https://github.com/tylergannon/skgo/tree/main/example" target="_blank">Read the source ↗</a>
</div>

<ul data-testid="capabilities">
	{#each capabilities as capability (capability.href)}
		<li data-testid="capability">
			<a
				data-testid="capability-link"
				href={capability.href}
				data-sveltekit-reload={capability.reload ? '' : undefined}>{capability.name}</a
			>
			<p data-testid="capability-look">{capability.look}</p>
		</li>
	{/each}
</ul>

<style>
	.showcase-hero {
		padding: clamp(2rem, 7vw, 5rem);
		border: 1px solid var(--line);
		background: linear-gradient(135deg, #eff9f4, #fff6ee);
	}
	.showcase-hero :global(h1[data-testid="title"]) { margin: 0; font-size: clamp(3rem, 9vw, 6.5rem); letter-spacing: -0.07em; }
	.showcase-hero :global(h2) { margin-top: 0.4rem; color: var(--green); font-family: Georgia, serif; font-weight: 400; }
	.eyebrow { margin: 0 0 0.6rem; color: var(--green); font-size: 0.72rem; font-weight: 800; letter-spacing: 0.14em; text-transform: uppercase; }
	.intro { max-width: 46rem; margin: 1.5rem 0; font-size: 1.08rem; line-height: 1.6; color: #52615b; }
	.provenance { display: inline-flex; gap: 0.7rem; align-items: center; padding: 0.5rem 0.7rem; background: var(--ink); color: white; font-size: 0.78rem; }
	.provenance :global(p) { margin: 0; }
	.provenance :global(p:first-child) { color: var(--lime); font-weight: 800; }
	.provenance :global(p:last-child) { font-family: ui-monospace, monospace; }
	.capability-heading { display: flex; justify-content: space-between; gap: 2rem; align-items: end; margin: 5rem 0 1.5rem; }
	.capability-heading h2 { margin: 0; font-size: clamp(2rem, 5vw, 3.4rem); letter-spacing: -0.05em; }
	.capability-heading a { margin-bottom: 0.35rem; font-size: 0.86rem; font-weight: 800; }
	ul { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 1px; margin: 0; padding: 1px; list-style: none; background: var(--line); }
	li { min-height: 12rem; padding: 1.5rem; background: var(--paper); }
	li:last-child:nth-child(odd) { grid-column: 1 / -1; }
	li a { color: var(--green); font-size: 1.05rem; font-weight: 800; text-decoration-thickness: 1px; text-underline-offset: 0.25rem; }
	li p { margin: 1rem 0 0; color: #62706b; line-height: 1.55; }
	@media (max-width: 700px) {
		ul { grid-template-columns: 1fr; }
		li:last-child:nth-child(odd) { grid-column: auto; }
		.capability-heading { display: block; }
	}
</style>
