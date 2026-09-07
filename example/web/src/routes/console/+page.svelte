<script lang="ts">
	/**
	 * A page that reports a failure the way kit and Svelte report one: through
	 * the console. Kit's `log_handle_error_hook_failure` and Svelte's
	 * `unresolved_hydratable` both do exactly this, in a production build, on
	 * paths a render reaches — so before the engine had a console at all, every
	 * such report was a ReferenceError thrown in the middle of the render and
	 * the reason went nowhere.
	 *
	 * The line is written while the page renders, which in production is inside
	 * the Go process. It has to come out of Go's log.
	 */
	console.error('skgo-console-probe: this page reported a failure while rendering');
</script>

<h1 data-testid="title">Console</h1>
<p data-testid="console-note">This page reported a failure to the console while it rendered.</p>
