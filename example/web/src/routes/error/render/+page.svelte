<h1 data-testid="title">Never seen</h1>

<!--
	Nothing here renders. The root layout throws for this route, and the root
	layout is the one node no `+error.svelte` can guard — kit wraps the root
	error page *in* it rather than the other way round — so the render fails,
	the retry through `respond_with_error` fails the same way, and the visitor
	gets kit's static error.html.
-->
