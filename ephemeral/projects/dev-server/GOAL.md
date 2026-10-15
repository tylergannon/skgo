# SKGO development experience: goal and architectural responsibilities

**We do not yet know the solution. Research multiple plausible strategies before choosing one.** These responsibilities define what we are trying to accomplish; they do not prescribe a particular implementation, integration mechanism, library, transport, or sequence of small tasks.

Run `skgo dev`, open the application, edit Svelte or Go, and see the change quickly and automatically. Preserve browser drafts and healthy page state whenever the change permits. Show useful errors and recover when they are fixed.

1. **A Vite development server does Vite's job.** It watches frontend files, transforms them, manages their dependencies, performs frontend HMR and maintains its browser connection.
2. **A Go application server serves the application.** It contains SKGO and application code, answers application HTTP requests, and uses the existing embedded renderer.
3. **Our Go development machinery supervises the Go application.** It observes Go changes, prepares updates and patches the running application quickly. It rebuilds/restarts when necessary. This is a responsibility distinction; it does not prescribe additional services.
4. **The two servers cooperate.** Frontend edits follow Vite's existing development flow. Go edits use our native patching work, and successful changes reach the browser through integration with Vite. The work is making that cooperation fast and reliable.

**Live patching is essential. Preserving arbitrary initialized Go state is not required.** Restart is fallback. The user's reported six-to-ten-second rebuild/link delay is the reason ordinary edits need the fast live-patching path. Long-running workflows do not have to survive inside the replaceable HTTP server.

## Research mission

Gather comprehensive documentation, source and prior art in this token cache. Use Luna researchers to discover and investigate a broad set of plausible strategies and combinations. Preserve competing hypotheses and their evidence, costs, uncertainties and possible discriminating experiments. Follow promising new directions found during research. No current candidate list is exhaustive, and no favorite mechanism or single research question defines the research boundary.

After the research rounds, use Sol 6.1 to index the material so alternatives and their source evidence can be found. Then compare the maybes and choose inexpensive proof-of-concept experiments. Source support and existing partial proofs are not proof that an entire proposed integration works. Research breadth does not turn candidate tactics into architectural requirements.

This document records the user's explicit four-part architecture and correction on 2026-10-15. The source plan, issues, older index routes and experiment notes are context; they must not override this goal or silently choose a solution.
