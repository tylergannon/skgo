correction: After handing site CI off, continue with the next substantial gap; do not mistake the handoff rule for a stop instruction.
friction: A Svelte `{#await}` block did not make Kit await the remote during prerender, so no remote asset was written. Invoke the fixed-argument remote from a universal load and check the built asset.
friction: A fresh worktree has no Node dependencies. Install the example app's pinned lockfile before treating a Vite import failure as a product defect.
decision: The live status regression is being investigated separately at the user's request; keep this source feature isolated and do not claim its result on the status board until a tagged measurement records it.
friction: `hydrated(page)` returns quietly when there is no boot script; a no-refetch assertion using it can pass without a client. Use `booted(page)` for a client-consumption claim.
