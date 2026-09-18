# Remove the custom project generator

correction: The user rejected the skgo-maintained SvelteKit scaffold itself, not just its missing add-ons. The immediate authorized task is demolition of skgo new, followed by root's own PR review and merge. Do not preserve that scaffold as the foundation of an add-on fix or implement a replacement in this removal PR.

decision: Work starts from fresh main in a separate worktree. The stopped add-on implementation is preserved and is not part of this PR. Preserve the runtime, adapter, Go binding generator, example application, and their coverage while removing generator-exclusive code and CI plumbing.
