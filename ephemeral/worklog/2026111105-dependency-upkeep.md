# Dependency upkeep

decision: Begin with the live npm SvelteKit 3.2.0 upgrade as the release-defining compatibility lane. It is the exact freshness contract; other direct dependency updates remain separately assessed so an incompatible major does not masquerade as a routine lock refresh.
finding: npm live latest is SvelteKit 3.2.0, while SKGO v0.46.0 pins 3.1.1. The target's published peers retain TypeScript ^6.0.0 and Svelte ^5.57.1, so TypeScript 7 is excluded from this Kit adoption.
friction: The prescribed external independent-review runner failed before review with its configured provider returning HTTP 401. Preserve its session ID 767dff89-1a11-4cfb-bb78-92ac18286bd1; do not retry or alter credentials. A separate available reviewer was requested.
finding: Kit 3.2.0's `src/core/postbuild/queue.js` has the same stock SHA-256 as 3.1.1. The shipped correction therefore applies without source drift; metadata, exact patch keys, consumer declarations and locks must still advance together so VitePlus will install it.
